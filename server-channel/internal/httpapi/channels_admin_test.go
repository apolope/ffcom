package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/storage"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// Teste de integração contra um Postgres de verdade: só roda com
// FFCOM_TEST_DATABASE_URL apontando para um banco descartável (as
// migrations são aplicadas nele). Sem a variável é pulado, já que o CI não
// sobe Postgres.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("FFCOM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FFCOM_TEST_DATABASE_URL não definida")
	}
	db, err := store.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

const missingID = "00000000-0000-0000-0000-000000000000"

func TestManageChannelsEndToEnd(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	files, err := storage.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	newMember := func(subject string) store.Member {
		t.Helper()
		m, err := db.Members.GetOrCreateByOIDCSubject(ctx, subject+"-"+t.Name())
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	owner := newMember("owner")
	owner.IsOwner = true
	plain := newMember("plain")
	manager := newMember("manager")
	role, err := db.Roles.Create(ctx, "gerente-"+t.Name(), nil, permissions.ManageChannels, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Roles.Delete(context.Background(), role.ID) })
	if err := db.Roles.AssignToMember(ctx, manager.ID, role.ID); err != nil {
		t.Fatal(err)
	}

	do := func(h http.Handler, member store.Member, method, id string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, "/", &buf)
		if id != "" {
			req.SetPathValue("id", id)
		}
		req = req.WithContext(auth.WithMember(req.Context(), member))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	createCategory := handleCreateCategory(db.Categories, db.Roles)
	updateCategory := handleUpdateCategory(db.Categories, db.Roles)
	deleteCategory := handleDeleteCategory(db.Categories, db.Roles)
	listCategories := handleListCategories(db.Categories, db.Channels, db.Roles, db.ChannelOverwrites)
	createChannel := handleCreateChannel(db.Categories, db.Channels, db.Roles)
	updateChannel := handleUpdateChannel(db.Categories, db.Channels, db.Roles)
	deleteChannel := handleDeleteChannel(db.Channels, db.Attachments, files, db.Roles)

	// Membro sem o bit leva 403 em todas as rotas de escrita.
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"criar categoria":  do(createCategory, plain, "POST", "", map[string]any{"name": "x"}),
		"editar categoria": do(updateCategory, plain, "PATCH", missingID, map[string]any{"name": "x"}),
		"apagar categoria": do(deleteCategory, plain, "DELETE", missingID, nil),
		"criar canal":      do(createChannel, plain, "POST", "", map[string]any{"name": "x", "type": "text"}),
		"editar canal":     do(updateChannel, plain, "PATCH", missingID, map[string]any{"name": "x"}),
		"apagar canal":     do(deleteChannel, plain, "DELETE", missingID, nil),
	} {
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s sem ManageChannels: status %d, esperado 403", name, rec.Code)
		}
	}

	// Gerente (role com ManageChannels) monta a estrutura padrão do zero.
	rec := do(createCategory, manager, "POST", "", map[string]any{"name": "  Geral  "})
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar categoria: %d %s", rec.Code, rec.Body)
	}
	var cat categoryView
	json.NewDecoder(rec.Body).Decode(&cat)
	t.Cleanup(func() { db.Categories.Delete(context.Background(), cat.ID) })
	if cat.Name != "Geral" {
		t.Errorf("nome não foi normalizado: %q", cat.Name)
	}

	// Categoria vazia aparece para quem gerencia e não para quem não gerencia.
	listHas := func(member store.Member) bool {
		rec := do(listCategories, member, "GET", "", nil)
		var body listCategoriesResponse
		json.NewDecoder(rec.Body).Decode(&body)
		for _, c := range body.Categories {
			if c.ID == cat.ID {
				return true
			}
		}
		return false
	}
	if !listHas(manager) || !listHas(owner) {
		t.Error("categoria vazia deveria aparecer para gerente e dono")
	}
	if listHas(plain) {
		t.Error("categoria vazia não deveria aparecer para membro comum")
	}

	var created []channelView
	for _, spec := range []struct{ name, typ string }{{"geral", "text"}, {"Voz", "voice"}, {"forum", "forum"}} {
		rec := do(createChannel, manager, "POST", "", map[string]any{"categoryId": cat.ID, "name": spec.name, "type": spec.typ})
		if rec.Code != http.StatusCreated {
			t.Fatalf("criar canal %s: %d %s", spec.name, rec.Code, rec.Body)
		}
		var ch channelView
		json.NewDecoder(rec.Body).Decode(&ch)
		created = append(created, ch)
	}
	t.Cleanup(func() {
		for _, ch := range created {
			db.Channels.Delete(context.Background(), ch.ID)
		}
	})
	for i, ch := range created {
		if ch.Position != i {
			t.Errorf("canal %s com position %d, esperado %d (fim da categoria)", ch.Name, ch.Position, i)
		}
	}

	for name, body := range map[string]map[string]any{
		"tipo inválido":         {"name": "x", "type": "video"},
		"categoria inexistente": {"categoryId": missingID, "name": "x", "type": "text"},
		"nome vazio":            {"name": "   ", "type": "text"},
		"nome longo":            {"name": strings.Repeat("a", 101), "type": "text"},
	} {
		if rec := do(createChannel, manager, "POST", "", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, esperado 400", name, rec.Code)
		}
	}

	// Renomear sem mandar categoryId mantém a categoria; tipo não muda.
	text := created[0]
	rec = do(updateChannel, manager, "PATCH", text.ID, map[string]any{"name": "bate-papo"})
	var renamed channelView
	json.NewDecoder(rec.Body).Decode(&renamed)
	if rec.Code != http.StatusOK || renamed.Name != "bate-papo" || renamed.CategoryID == nil || *renamed.CategoryID != cat.ID {
		t.Errorf("renomear canal: %d %+v", rec.Code, renamed)
	}
	if rec := do(updateChannel, manager, "PATCH", text.ID, map[string]any{"type": "voice"}); rec.Code != http.StatusBadRequest {
		t.Errorf("trocar tipo: status %d, esperado 400", rec.Code)
	}
	// "categoryId": null tira da categoria; mandar de volta recoloca.
	rec = do(updateChannel, manager, "PATCH", created[2].ID, map[string]any{"categoryId": nil})
	var moved channelView
	json.NewDecoder(rec.Body).Decode(&moved)
	if rec.Code != http.StatusOK || moved.CategoryID != nil {
		t.Errorf("tirar canal da categoria: %d %+v", rec.Code, moved)
	}
	if rec := do(updateChannel, manager, "PATCH", created[2].ID, map[string]any{"categoryId": cat.ID}); rec.Code != http.StatusOK {
		t.Errorf("devolver canal à categoria: status %d", rec.Code)
	}
	if rec := do(updateCategory, owner, "PATCH", cat.ID, map[string]any{"name": "Principal"}); rec.Code != http.StatusOK {
		t.Errorf("renomear categoria como dono: status %d", rec.Code)
	}
	if rec := do(updateChannel, manager, "PATCH", missingID, map[string]any{"name": "x"}); rec.Code != http.StatusNotFound {
		t.Errorf("editar canal inexistente: status %d, esperado 404", rec.Code)
	}

	// Apagar canal leva mensagens e arquivos de anexo junto.
	msg, err := db.Messages.Create(ctx, text.ID, nil, manager.ID, "olá")
	if err != nil {
		t.Fatal(err)
	}
	key, err := files.Save(strings.NewReader("conteúdo"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Attachments.Create(ctx, msg.ID, "a.txt", "text/plain", 8, key); err != nil {
		t.Fatal(err)
	}
	if rec := do(deleteChannel, manager, "DELETE", text.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("apagar canal: %d %s", rec.Code, rec.Body)
	}
	if _, err := db.Channels.GetByID(ctx, text.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("canal ainda existe: %v", err)
	}
	if _, err := db.Messages.GetByID(ctx, msg.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("mensagem do canal apagado ainda existe: %v", err)
	}
	if f, err := files.Open(key); !errors.Is(err, os.ErrNotExist) {
		if f != nil {
			f.Close()
		}
		t.Errorf("arquivo de anexo continua no disco (err=%v)", err)
	}

	// Apagar categoria deixa os canais restantes sem categoria.
	if rec := do(deleteCategory, manager, "DELETE", cat.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("apagar categoria: %d", rec.Code)
	}
	voice, err := db.Channels.GetByID(ctx, created[1].ID)
	if err != nil {
		t.Fatalf("canal de voz sumiu junto com a categoria: %v", err)
	}
	if voice.CategoryID != nil {
		t.Errorf("canal deveria ter ficado sem categoria, está em %s", *voice.CategoryID)
	}
}
