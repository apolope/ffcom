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

// Bits granulares (CreateCategories, ReorderChannels etc.) liberam só a ação
// deles; renomear continua exigindo ManageChannels. Também cobre
// PUT /api/categories/order.
func TestGranularStructurePermissions(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()

	member, err := db.Members.GetOrCreateByOIDCSubject(ctx, "organizer-"+t.Name())
	if err != nil {
		t.Fatal(err)
	}
	bits := permissions.CreateCategories | permissions.ReorderCategories | permissions.CreateChannels | permissions.ReorderChannels
	role, err := db.Roles.Create(ctx, "organizador-"+t.Name(), nil, bits, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Roles.Delete(context.Background(), role.ID) })
	if err := db.Roles.AssignToMember(ctx, member.ID, role.ID); err != nil {
		t.Fatal(err)
	}

	do := func(h http.Handler, method, id string, body any) *httptest.ResponseRecorder {
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

	var cats []categoryView
	for _, name := range []string{"A", "B"} {
		rec := do(handleCreateCategory(db.Categories, db.Roles), "POST", "", map[string]any{"name": name})
		if rec.Code != http.StatusCreated {
			t.Fatalf("criar categoria com CreateCategories: %d %s", rec.Code, rec.Body)
		}
		var c categoryView
		json.NewDecoder(rec.Body).Decode(&c)
		t.Cleanup(func() { db.Categories.Delete(context.Background(), c.ID) })
		cats = append(cats, c)
	}

	if rec := do(handleUpdateCategory(db.Categories, db.Roles), "PATCH", cats[0].ID, map[string]any{"name": "X"}); rec.Code != http.StatusForbidden {
		t.Errorf("renomear categoria sem ManageChannels: status %d, esperado 403", rec.Code)
	}
	if rec := do(handleDeleteCategory(db.Categories, db.Roles), "DELETE", cats[0].ID, nil); rec.Code != http.StatusForbidden {
		t.Errorf("apagar categoria sem DeleteCategories: status %d, esperado 403", rec.Code)
	}

	rec := do(handleCreateChannel(db.Categories, db.Channels, db.Roles), "POST", "", map[string]any{"categoryId": cats[0].ID, "name": "c", "type": "text"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar canal com CreateChannels: %d %s", rec.Code, rec.Body)
	}
	var ch channelView
	json.NewDecoder(rec.Body).Decode(&ch)
	t.Cleanup(func() { db.Channels.Delete(context.Background(), ch.ID) })
	updateChannel := handleUpdateChannel(db.Categories, db.Channels, db.Roles)
	if rec := do(updateChannel, "PATCH", ch.ID, map[string]any{"categoryId": cats[1].ID}); rec.Code != http.StatusOK {
		t.Errorf("mover canal com ReorderChannels: status %d", rec.Code)
	}
	if rec := do(updateChannel, "PATCH", ch.ID, map[string]any{"name": "d"}); rec.Code != http.StatusForbidden {
		t.Errorf("renomear canal sem ManageChannels: status %d, esperado 403", rec.Code)
	}
	if rec := do(handleDeleteChannel(db.Channels, db.Attachments, nil, db.Roles), "DELETE", ch.ID, nil); rec.Code != http.StatusForbidden {
		t.Errorf("apagar canal sem DeleteChannels: status %d, esperado 403", rec.Code)
	}

	// Reordenar canais: mover para B no topo, antes de um canal que o grupo
	// não lista (fica depois). Canal inexistente dá 409.
	rec = do(handleCreateChannel(db.Categories, db.Channels, db.Roles), "POST", "", map[string]any{"categoryId": cats[1].ID, "name": "e", "type": "text"})
	var other channelView
	json.NewDecoder(rec.Body).Decode(&other)
	t.Cleanup(func() { db.Channels.Delete(context.Background(), other.ID) })
	reorderChannels := handleReorderChannels(db.Channels, db.Roles)
	if rec := do(reorderChannels, "PUT", "", map[string]any{"groups": []map[string]any{{"categoryId": cats[1].ID, "ids": []string{missingID}}}}); rec.Code != http.StatusConflict {
		t.Errorf("reordenar canal inexistente: status %d, esperado 409", rec.Code)
	}
	if rec := do(reorderChannels, "PUT", "", map[string]any{"groups": []map[string]any{
		{"categoryId": nil, "ids": []string{}},
		{"categoryId": cats[1].ID, "ids": []string{ch.ID}},
	}}); rec.Code != http.StatusNoContent {
		t.Fatalf("reordenar canais: %d %s", rec.Code, rec.Body)
	}
	for id, want := range map[string]int{ch.ID: 0, other.ID: 1} {
		got, err := db.Channels.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.CategoryID == nil || *got.CategoryID != cats[1].ID || got.Position != want {
			t.Errorf("canal %s: categoria %v posição %d, esperado %s posição %d", id, got.CategoryID, got.Position, cats[1].ID, want)
		}
	}

	// Reordenar: a lista precisa ser o conjunto atual inteiro. Inverte a
	// ordem de tudo que existe no banco (outros testes podem ter deixado
	// categorias).
	reorder := handleReorderCategories(db.Categories, db.Roles)
	all, err := db.Categories.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(all))
	for i, c := range all {
		ids[len(all)-1-i] = c.ID
	}
	if rec := do(reorder, "PUT", "", map[string]any{"ids": ids[1:]}); rec.Code != http.StatusConflict {
		t.Errorf("reordenar com lista incompleta: status %d, esperado 409", rec.Code)
	}
	if rec := do(reorder, "PUT", "", map[string]any{"ids": ids}); rec.Code != http.StatusNoContent {
		t.Fatalf("reordenar: %d %s", rec.Code, rec.Body)
	}
	after, err := db.Categories.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range after {
		if c.ID != ids[i] || c.Position != i {
			t.Fatalf("posição %d: %s (%d), esperado %s", i, c.ID, c.Position, ids[i])
		}
	}
}
