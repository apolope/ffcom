package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/push"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/storage"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// fakeCentral faz o papel de POST /api/push/notify do server-central e
// guarda cada corpo recebido.
type fakeCentral struct {
	mu       sync.Mutex
	requests []map[string]any
}

func (f *fakeCentral) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/api/push/notify" {
		http.NotFound(w, r)
		return
	}
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.requests = append(f.requests, body)
	f.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}

// grantsIn junta os grants de todas as chamadas que estão em mine (o banco
// de teste é compartilhado com os outros testes do pacote).
func (f *fakeCentral) grantsIn(mine map[string]bool) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, req := range f.requests {
		grants, _ := req["grants"].([]any)
		for _, g := range grants {
			if s, _ := g.(string); mine[s] {
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Destinatários de uma mensagem: só quem deu grant, tem ViewChannels no
// canal, não é o autor, não está com o canal aberto e não foi expulso.
func TestPushRecipients(t *testing.T) {
	f := newRoleFixture(t)
	central := &fakeCentral{}
	srv := httptest.NewServer(central)
	t.Cleanup(srv.Close)
	notifier := push.New(srv.URL, NewPushResolver(f.db))
	hub := realtime.NewHub()

	channel := f.channel("geral")
	author := f.member("autor")
	reader := f.member("leitor")
	viewer := f.member("vendo")
	hidden := f.member("sem-view")
	kicked := f.member("expulso")
	noGrant := f.member("sem-grant")
	_ = noGrant

	// Overwrite que tira ViewChannels de uma role só neste canal.
	hiddenRole := f.role("escondido", 0)
	f.assign(hidden, hiddenRole)
	if _, err := f.db.ChannelOverwrites.Set(f.ctx, channel.ID, hiddenRole.ID, 0, permissions.ViewChannels); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.ChannelOverwrites.Delete(context.Background(), channel.ID, hiddenRole.ID) })

	setGrant := handleSetPushGrant(f.db.Members)
	address := "https://canal.example/" + t.Name()
	grants := map[string]string{}
	mine := map[string]bool{}
	for _, m := range []store.Member{author, reader, viewer, hidden, kicked} {
		g := fmt.Sprintf("grant-%s-%d", m.ID, time.Now().UnixNano())
		expectStatus(t, "PUT push-grant", f.do(setGrant, m, "PUT", map[string]any{"token": g, "serverAddress": address}), http.StatusNoContent)
		grants[m.ID] = g
		mine[g] = true
	}
	expectStatus(t, "PUT push-grant sem endereço", f.do(setGrant, reader, "PUT", map[string]any{"token": "x"}), http.StatusBadRequest)
	expectStatus(t, "PUT push-grant sem token", f.do(setGrant, reader, "PUT", map[string]any{"serverAddress": address}), http.StatusBadRequest)

	// Expulsar apaga o grant.
	if _, err := f.db.Members.Kick(f.ctx, kicked.ID); err != nil {
		t.Fatal(err)
	}

	// viewer está com o WebSocket do canal aberto.
	viewerClient := realtime.NewClient(nil)
	viewerClient.MemberID = viewer.ID
	hub.Register(channel.ID, viewerClient)

	msg, err := f.db.Messages.Create(f.ctx, channel.ID, nil, author.ID, "Ficaram lindas!")
	if err != nil {
		t.Fatal(err)
	}
	messagePusher(notifier, hub, author)(msg, "", "")
	notifier.Wait()

	got := central.grantsIn(mine)
	if len(got) != 1 || got[0] != grants[reader.ID] {
		t.Fatalf("grants enviados = %v, esperado só o do leitor (%s)", got, grants[reader.ID])
	}
	central.mu.Lock()
	var req map[string]any
	for _, r := range central.requests {
		if r["serverAddress"] == address {
			req = r
		}
	}
	central.mu.Unlock()
	want := map[string]any{"serverAddress": address, "channelId": channel.ID, "channelName": channel.Name, "author": author.DisplayName(), "text": "Ficaram lindas!", "messageId": msg.ID}
	for k, v := range want {
		if req[k] != v {
			t.Errorf("corpo[%q] = %v, esperado %v", k, req[k], v)
		}
	}

	// Quem fecha o canal volta a receber; DELETE com o token de outro
	// aparelho não apaga, sem token apaga.
	hub.Unregister(channel.ID, viewerClient)
	deleteGrant := handleDeletePushGrant(f.db.Members)
	expectStatus(t, "DELETE com outro token", f.do(deleteGrant, reader, "DELETE", map[string]any{"token": "outro"}), http.StatusNoContent)
	expectStatus(t, "DELETE sem corpo", f.do(deleteGrant, viewer, "DELETE", nil), http.StatusNoContent)
	central.mu.Lock()
	central.requests = nil
	central.mu.Unlock()
	messagePusher(notifier, hub, author)(msg, "", "")
	notifier.Wait()
	if got := central.grantsIn(mine); len(got) != 1 || got[0] != grants[reader.ID] {
		t.Fatalf("depois dos DELETE: grants = %v, esperado só o do leitor", got)
	}
}

// Sem push (FFCOM_CENTRAL_URL=off) ou com o central fora do ar, criar
// mensagem continua respondendo normalmente e na hora.
func TestPushDisabledOrCentralDown(t *testing.T) {
	if push.New("off", nil) != nil {
		t.Fatal(`FFCOM_CENTRAL_URL=off deveria desligar o push`)
	}
	messagePusher(nil, realtime.NewHub(), store.Member{})(store.Message{}, "", "")

	f := newRoleFixture(t)
	files, err := storage.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	channel := f.channel("geral")
	author := f.member("autor")
	reader := f.member("leitor")
	expectStatus(t, "PUT push-grant", f.do(handleSetPushGrant(f.db.Members), reader, "PUT", map[string]any{"token": "g-" + reader.ID, "serverAddress": "https://x"}), http.StatusNoContent)

	// Central que não responde a tempo: o envio é assíncrono, então a
	// mensagem sai antes.
	block := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	t.Cleanup(func() { close(block); slow.Close() })

	for name, notifier := range map[string]*push.Notifier{
		"desligado":     nil,
		"central lento": push.New(slow.URL, NewPushResolver(f.db)),
		"central fora":  push.New("http://127.0.0.1:1", NewPushResolver(f.db)),
	} {
		h := handleCreateMessageWithAttachment(realtime.NewHub(), f.db.Channels, f.db.Roles, f.db.ChannelOverwrites, f.db.Messages, f.db.Attachments, files, 1<<20, notifier)
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		mw.WriteField("content", "oi")
		mw.Close()
		req := httptest.NewRequest("POST", "/", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.SetPathValue("id", channel.ID)
		req = req.WithContext(auth.WithMember(req.Context(), author))
		rec := httptest.NewRecorder()
		start := time.Now()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Errorf("%s: status %d, esperado 201 (%s)", name, rec.Code, rec.Body)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Errorf("%s: criar mensagem levou %s", name, elapsed)
		}
	}
}
