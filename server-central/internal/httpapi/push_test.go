package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/push"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// fakeSender faz o papel do FCM nos testes: guarda o que seria enviado e
// responde UNREGISTERED para os tokens em dead.
type fakeSender struct {
	mu   sync.Mutex
	sent []sentPush
	dead map[string]bool
}

type sentPush struct {
	token string
	data  map[string]string
}

func (f *fakeSender) Send(_ context.Context, token string, data map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dead[token] {
		return push.ErrUnregistered
	}
	f.sent = append(f.sent, sentPush{token: token, data: data})
	return nil
}

// take devolve e limpa o que foi enviado.
func (f *fakeSender) take() []sentPush {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.sent
	f.sent = nil
	return out
}

// Teste de integração (Postgres, ver openTestStore em ideas_test.go) do
// caminho inteiro: grant só para servidor da lista, notify entregando com
// autor e texto, grant de outro servidor e desconhecido sem efeito e com a
// mesma resposta, silêncio, revogação ao tirar o servidor da lista e token
// UNREGISTERED apagado.
func TestPushEndToEnd(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	sender := &fakeSender{dead: map[string]bool{}}
	dispatcher := push.NewDispatcher(sender, db.Push, 2, 100)

	mux := http.NewServeMux()
	mux.Handle("POST /api/servers", handleAddServer(db.KnownServers))
	mux.Handle("DELETE /api/servers/{id}", handleRemoveServer(db.KnownServers))
	mux.Handle("PUT /api/push/devices", handleRegisterPushDevice(db.Push))
	mux.Handle("DELETE /api/push/devices", handleDeletePushDevice(db.Push))
	mux.Handle("POST /api/push/grants", handleCreatePushGrant(db.Push))
	mux.Handle("GET /api/push/mutes", handleListPushMutes(db.Push))
	mux.Handle("PUT /api/push/mutes", handleSetPushMute(db.Push))
	mux.Handle("POST /api/push/notify", handlePushNotify(db.Push, dispatcher, newPushLimits()))

	suffix := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	newAccount := func(subject string) store.Account {
		t.Helper()
		name := subject
		a, err := db.Accounts.GetOrCreateBySubject(ctx, subject+"-"+suffix, &name)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	alice := newAccount("alice")
	bob := newAccount("bob")

	call := func(account *store.Account, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(method, path, reader)
		if account != nil {
			req = req.WithContext(auth.WithAccount(req.Context(), *account))
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	expect := func(what string, rec *httptest.ResponseRecorder, want int) {
		t.Helper()
		if rec.Code != want {
			t.Fatalf("%s: status %d, esperado %d (%s)", what, rec.Code, want, strings.TrimSpace(rec.Body.String()))
		}
	}

	addrA := "https://a-" + suffix + ".example"
	addrB := "https://b-" + suffix + ".example"
	var serverA knownServerView
	rec := call(&alice, "POST", "/api/servers", map[string]any{"address": addrA, "name": "Família da Ana"})
	expect("alice adiciona A", rec, http.StatusCreated)
	json.Unmarshal(rec.Body.Bytes(), &serverA)
	expect("bob adiciona B", call(&bob, "POST", "/api/servers", map[string]any{"address": addrB, "name": "B"}), http.StatusCreated)

	// Grant só para servidor da lista.
	expect("grant de servidor fora da lista", call(&alice, "POST", "/api/push/grants", map[string]any{"serverAddress": addrB}), http.StatusNotFound)
	expect("grant sem endereço", call(&alice, "POST", "/api/push/grants", map[string]any{}), http.StatusBadRequest)
	grant := func(account *store.Account, address string) string {
		t.Helper()
		rec := call(account, "POST", "/api/push/grants", map[string]any{"serverAddress": address})
		expect("grant", rec, http.StatusCreated)
		var out pushGrantResponse
		json.Unmarshal(rec.Body.Bytes(), &out)
		if out.Token == "" || out.ServerAddress != address {
			t.Fatalf("resposta do grant: %+v", out)
		}
		return out.Token
	}
	grantA := grant(&alice, addrA)
	grantB := grant(&bob, addrB)

	expect("plataforma inválida", call(&alice, "PUT", "/api/push/devices", map[string]any{"token": "x", "platform": "ios"}), http.StatusBadRequest)
	aliceDevice := "fcm-alice-" + suffix
	bobDevice := "fcm-bob-" + suffix
	expect("aparelho da alice", call(&alice, "PUT", "/api/push/devices", map[string]any{"token": aliceDevice, "platform": "android"}), http.StatusNoContent)
	expect("aparelho do bob", call(&bob, "PUT", "/api/push/devices", map[string]any{"token": bobDevice}), http.StatusNoContent)

	notify := func(address, channelID, text string, grants ...string) string {
		t.Helper()
		rec := call(nil, "POST", "/api/push/notify", map[string]any{
			"serverAddress": address,
			"serverName":    "nome que o servidor manda",
			"channelId":     channelID,
			"channelName":   "geral",
			"author":        "Marina",
			"text":          text,
			"grants":        grants,
		})
		expect("notify", rec, http.StatusAccepted)
		dispatcher.Wait()
		return rec.Body.String()
	}

	// Entrega com autor e texto, só para os aparelhos da conta do grant.
	okBody := notify(addrA, "c1", "Ficaram lindas!", grantA)
	sent := sender.take()
	if len(sent) != 1 || sent[0].token != aliceDevice {
		t.Fatalf("esperava 1 push para o aparelho da alice, veio %+v", sent)
	}
	d := sent[0].data
	want := map[string]string{"v": "1", "type": "channel_message", "serverAddress": addrA, "serverName": "Família da Ana", "channelId": "c1", "channelName": "geral", "author": "Marina", "text": "Ficaram lindas!"}
	for k, v := range want {
		if d[k] != v {
			t.Errorf("data[%q] = %q, esperado %q", k, d[k], v)
		}
	}
	if d["sentAt"] == "" {
		t.Error("sentAt vazio")
	}

	// Grant de outro servidor, grant desconhecido e grant repetido: nada a
	// mais, e a mesma resposta.
	if body := notify(addrA, "c1", "oi", grantB, "nao-existe"); body != okBody {
		t.Errorf("resposta diferente para grant de outro servidor: %q vs %q", body, okBody)
	}
	if sent := sender.take(); len(sent) != 0 {
		t.Fatalf("grant de outro servidor entregou: %+v", sent)
	}
	notify(addrA, "c1", "oi", grantA, grantA)
	if sent := sender.take(); len(sent) != 1 {
		t.Fatalf("grant repetido: esperava 1 push, veio %d", len(sent))
	}

	// Texto longo é truncado.
	notify(addrA, "c1", strings.Repeat("a", 1000), grantA)
	if sent := sender.take(); len(sent) != 1 || utf8.RuneCountInString(sent[0].data["text"]) != push.MaxTextRunes {
		t.Fatalf("texto não truncado em %d: %+v", push.MaxTextRunes, sent)
	}

	// Silêncio de canal e de servidor.
	expect("mute sem servidor na lista", call(&alice, "PUT", "/api/push/mutes", map[string]any{"serverAddress": addrB, "muted": true}), http.StatusNotFound)
	expect("mute sem muted", call(&alice, "PUT", "/api/push/mutes", map[string]any{"serverAddress": addrA}), http.StatusBadRequest)
	expect("silenciar c1", call(&alice, "PUT", "/api/push/mutes", map[string]any{"serverAddress": addrA, "channelId": "c1", "muted": true}), http.StatusNoContent)
	notify(addrA, "c1", "oi", grantA)
	if sent := sender.take(); len(sent) != 0 {
		t.Fatalf("canal silenciado notificou: %+v", sent)
	}
	notify(addrA, "c2", "oi", grantA)
	if sent := sender.take(); len(sent) != 1 {
		t.Fatalf("outro canal deveria notificar, veio %d", len(sent))
	}
	expect("silenciar servidor", call(&alice, "PUT", "/api/push/mutes", map[string]any{"serverAddress": addrA, "muted": true}), http.StatusNoContent)
	rec = call(&alice, "GET", "/api/push/mutes", nil)
	expect("listar silêncios", rec, http.StatusOK)
	var mutes pushMutesResponse
	json.Unmarshal(rec.Body.Bytes(), &mutes)
	if len(mutes.Mutes) != 2 {
		t.Fatalf("esperava 2 silêncios, veio %+v", mutes)
	}
	notify(addrA, "c2", "oi", grantA)
	if sent := sender.take(); len(sent) != 0 {
		t.Fatalf("servidor silenciado notificou: %+v", sent)
	}
	expect("voltar a notificar servidor", call(&alice, "PUT", "/api/push/mutes", map[string]any{"serverAddress": addrA, "muted": false}), http.StatusNoContent)
	notify(addrA, "c2", "oi", grantA)
	if sent := sender.take(); len(sent) != 1 {
		t.Fatalf("depois de tirar o silêncio do servidor, esperava 1 push, veio %d", len(sent))
	}

	// Token que o FCM diz não existir mais é apagado.
	sender.dead[bobDevice] = true
	notify(addrB, "c1", "oi", grantB)
	if tokens, _ := db.Push.DeviceTokens(ctx, bob.ID); len(tokens) != 0 {
		t.Errorf("token UNREGISTERED continua: %v", tokens)
	}

	// Logout no aparelho: DELETE tira o token.
	expect("remover aparelho", call(&alice, "DELETE", "/api/push/devices", map[string]any{"token": aliceDevice}), http.StatusNoContent)
	notify(addrA, "c2", "oi", grantA)
	if sent := sender.take(); len(sent) != 0 {
		t.Fatalf("aparelho removido recebeu: %+v", sent)
	}
	expect("aparelho da alice de novo", call(&alice, "PUT", "/api/push/devices", map[string]any{"token": aliceDevice}), http.StatusNoContent)

	// Tirar o servidor da lista revoga os grants dele (e os silêncios).
	expect("remover A", call(&alice, "DELETE", "/api/servers/"+serverA.ID, nil), http.StatusNoContent)
	if body := notify(addrA, "c2", "oi", grantA); body != okBody {
		t.Errorf("resposta diferente para grant revogado: %q", body)
	}
	if sent := sender.take(); len(sent) != 0 {
		t.Fatalf("grant de servidor removido entregou: %+v", sent)
	}
	rec = call(&alice, "GET", "/api/push/mutes", nil)
	json.Unmarshal(rec.Body.Bytes(), &mutes)
	if len(mutes.Mutes) != 0 {
		t.Errorf("silêncios do servidor removido continuam: %+v", mutes)
	}

	// Corpo inválido é 400.
	expect("notify sem serverAddress", call(nil, "POST", "/api/push/notify", map[string]any{"channelId": "c1", "grants": []string{grantA}}), http.StatusBadRequest)
}

// Sem FCM_SERVICE_ACCOUNT_JSON o Dispatcher é nil: notify responde igual e
// não toca no banco, e as notificações de DM e amizade viram no-op.
func TestPushNotifyDisabled(t *testing.T) {
	h := handlePushNotify(nil, nil, newPushLimits())
	raw, _ := json.Marshal(map[string]any{"serverAddress": "https://x", "channelId": "c1", "grants": []string{"g"}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/push/notify", bytes.NewReader(raw)))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, esperado 202", rec.Code)
	}
	pushFromAccount(nil, nil, push.TypeDM, "a", "b", "")
}

// DM e pedido de amizade: o worker busca o nome de quem mandou; a DM não
// leva conteúdo nenhum.
func TestPushFromAccount(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	sender := &fakeSender{}
	dispatcher := push.NewDispatcher(sender, db.Push, 1, 10)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	name := "Ana Souza"
	ana, err := db.Accounts.GetOrCreateBySubject(ctx, "ana-"+suffix, &name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Profiles.Upsert(ctx, ana.ID, name, nil); err != nil {
		t.Fatal(err)
	}
	bia, err := db.Accounts.GetOrCreateBySubject(ctx, "bia-"+suffix, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Push.UpsertDevice(ctx, bia.ID, "fcm-bia-"+suffix, "android"); err != nil {
		t.Fatal(err)
	}

	pushFromAccount(dispatcher, db.Profiles, push.TypeDM, bia.ID, ana.ID, "")
	pushFromAccount(dispatcher, db.Profiles, push.TypeFriendRequest, bia.ID, ana.ID, "req-1")
	dispatcher.Wait()
	sent := sender.take()
	if len(sent) != 2 {
		t.Fatalf("esperava 2 pushes, veio %d", len(sent))
	}
	byType := map[string]map[string]string{}
	for _, s := range sent {
		byType[s.data["type"]] = s.data
	}
	dm := byType[push.TypeDM]
	if dm["author"] != name || dm["accountId"] != ana.ID || dm["text"] != "" {
		t.Errorf("dm: %+v", dm)
	}
	if fr := byType[push.TypeFriendRequest]; fr["requestId"] != "req-1" || fr["author"] != name {
		t.Errorf("friend_request: %+v", fr)
	}
}
