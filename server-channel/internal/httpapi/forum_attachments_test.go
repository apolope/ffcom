package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/storage"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// Anexo em canal forum: abrir thread e responder com arquivo pelas rotas
// REST, ler de volta pelo histórico da thread e baixar pelo mesmo
// GET /api/attachments/{id} do canal de texto.
func TestForumAttachmentsEndToEnd(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	files, err := storage.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hub := realtime.NewHub()
	const maxBytes = 1 << 20

	member, err := db.Members.GetOrCreateByOIDCSubject(ctx, "forum-author-"+t.Name())
	if err != nil {
		t.Fatal(err)
	}
	forum, err := db.Channels.Create(ctx, nil, "forum-"+t.Name(), store.ChannelForum, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Channels.Delete(context.Background(), forum.ID) })
	text, err := db.Channels.Create(ctx, nil, "texto-"+t.Name(), store.ChannelText, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Channels.Delete(context.Background(), text.ID) })

	createThread := handleCreateThreadWithAttachment(hub, db.Channels, db.Roles, db.ChannelOverwrites, db.Messages, db.Attachments, files, maxBytes, nil)
	createPost := handleCreatePostWithAttachment(hub, db.Roles, db.ChannelOverwrites, db.Messages, db.Attachments, files, maxBytes, nil)
	listPosts := handleListThreadMessages(db.Roles, db.ChannelOverwrites, db.Messages, db.Attachments)
	getAttachment := handleGetAttachment(db.Attachments, db.Messages, db.Roles, db.ChannelOverwrites, files)

	do := func(h http.Handler, id string, fields map[string]string, file []byte) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			mw.WriteField(k, v)
		}
		if file != nil {
			fw, _ := mw.CreateFormFile("file", "../../planta.txt")
			fw.Write(file)
		}
		mw.Close()
		req := httptest.NewRequest("POST", "/", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.SetPathValue("id", id)
		req = req.WithContext(auth.WithMember(req.Context(), member))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	get := func(h http.Handler, id string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "/", nil)
		req.SetPathValue("id", id)
		req = req.WithContext(auth.WithMember(req.Context(), member))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Validações: canal de texto, sem título, sem conteúdo nem arquivo.
	if rec := do(createThread, text.ID, map[string]string{"title": "x", "content": "y"}, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("thread em canal de texto: status %d, esperado 400", rec.Code)
	}
	if rec := do(createThread, forum.ID, map[string]string{"content": "y"}, []byte("a")); rec.Code != http.StatusBadRequest {
		t.Errorf("thread sem título: status %d, esperado 400", rec.Code)
	}
	if rec := do(createThread, forum.ID, map[string]string{"title": "x"}, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("thread sem conteúdo nem anexo: status %d, esperado 400", rec.Code)
	}
	if rec := do(createThread, forum.ID, map[string]string{"title": "x"}, make([]byte, maxBytes+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("anexo acima do limite: status %d, esperado 413", rec.Code)
	}
	if threads, _ := db.Messages.ListThreads(ctx, forum.ID); len(threads) != 0 {
		t.Fatalf("validação falha não deveria criar thread, há %d", len(threads))
	}

	// Thread só com anexo (sem texto) no post inicial.
	rec := do(createThread, forum.ID, map[string]string{"title": "  Planta da casa  "}, []byte("conteúdo da planta"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("criar thread com anexo: %d %s", rec.Code, rec.Body)
	}
	var created createThreadResponse
	json.NewDecoder(rec.Body).Decode(&created)
	if created.Thread.Title != "Planta da casa" {
		t.Errorf("título não foi normalizado: %q", created.Thread.Title)
	}
	if created.Message.ThreadID == nil || *created.Message.ThreadID != created.Thread.ID {
		t.Errorf("post inicial sem threadId da thread criada: %+v", created.Message.ThreadID)
	}
	if len(created.Message.Attachments) != 1 || created.Message.Attachments[0].Filename != "planta.txt" {
		t.Fatalf("anexo do post inicial: %+v", created.Message.Attachments)
	}

	// Resposta com texto e anexo.
	rec = do(createPost, created.Thread.ID, map[string]string{"content": "segue a versão 2"}, []byte("v2"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("responder com anexo: %d %s", rec.Code, rec.Body)
	}
	if rec := do(createPost, missingID, map[string]string{"content": "x"}, []byte("x")); rec.Code != http.StatusNotFound {
		t.Errorf("responder em thread inexistente: status %d, esperado 404", rec.Code)
	}

	// Histórico da thread traz os anexos dos dois posts.
	rec = get(listPosts, created.Thread.ID)
	var history listMessagesResponse
	json.NewDecoder(rec.Body).Decode(&history)
	if len(history.Messages) != 2 {
		t.Fatalf("histórico da thread: %d posts, esperado 2", len(history.Messages))
	}
	for _, m := range history.Messages {
		if len(m.Attachments) != 1 {
			t.Errorf("post %s sem anexo no histórico", m.ID)
		}
	}

	// Download do anexo do fórum pela rota comum.
	rec = get(getAttachment, created.Message.Attachments[0].ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("baixar anexo do fórum: %d %s", rec.Code, rec.Body)
	}
	if body, _ := io.ReadAll(rec.Body); string(body) != "conteúdo da planta" {
		t.Errorf("conteúdo baixado: %q", body)
	}

	// Sem SendMessages no fórum (overwrite negando na role padrão): 403 nas
	// duas rotas.
	def, err := db.Roles.GetDefault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ChannelOverwrites.Set(ctx, forum.ID, def.ID, 0, permissions.SendMessages); err != nil {
		t.Fatal(err)
	}
	if rec := do(createThread, forum.ID, map[string]string{"title": "x", "content": "y"}, nil); rec.Code != http.StatusForbidden {
		t.Errorf("thread sem SendMessages: status %d, esperado 403", rec.Code)
	}
	if rec := do(createPost, created.Thread.ID, map[string]string{"content": "y"}, nil); rec.Code != http.StatusForbidden {
		t.Errorf("resposta sem SendMessages: status %d, esperado 403", rec.Code)
	}
}
