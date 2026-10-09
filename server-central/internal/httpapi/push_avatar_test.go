package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/push"
	"a3sitsolutions.com/ffcom/server-central/internal/storage"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// pngBytes começa com a assinatura de PNG, o bastante para o
// DetectContentType.
var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{7}, 64)...)

type pushAvatarFixture struct {
	t      *testing.T
	ctx    context.Context
	db     *store.Store
	files  *storage.AvatarStore
	suffix string
}

func newPushAvatarFixture(t *testing.T) pushAvatarFixture {
	db := openTestStore(t)
	files, err := storage.NewAvatarStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return pushAvatarFixture{t: t, ctx: context.Background(), db: db, files: files, suffix: fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())}
}

// account cria uma conta; withAvatar grava um avatar para ela, como o
// POST /api/me/avatar faria.
func (f pushAvatarFixture) account(subject string, withAvatar bool) store.Account {
	f.t.Helper()
	name := subject
	a, err := f.db.Accounts.GetOrCreateBySubject(f.ctx, subject+"-"+f.suffix, &name)
	if err != nil {
		f.t.Fatal(err)
	}
	if withAvatar {
		f.setAvatar(a)
	}
	return a
}

func (f pushAvatarFixture) setAvatar(a store.Account) {
	f.t.Helper()
	if err := f.files.Save(a.ID, bytes.NewReader(pngBytes)); err != nil {
		f.t.Fatal(err)
	}
	url := "/api/avatars/" + a.ID
	if _, err := f.db.Profiles.Upsert(f.ctx, a.ID, a.OIDCSubject, &url); err != nil {
		f.t.Fatal(err)
	}
}

func (f pushAvatarFixture) get(token string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.Handle("GET "+pushAvatarsPathPrefix+"{token}", handleGetPushAvatar(f.db.Push, f.files))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", pushAvatarsPathPrefix+token, nil))
	return rec
}

func tokenOf(t *testing.T, url string) string {
	t.Helper()
	const base = "https://central.test" + pushAvatarsPathPrefix
	if !strings.HasPrefix(url, base) {
		t.Fatalf("link %q fora de %s", url, base)
	}
	return strings.TrimPrefix(url, base)
}

// Emissão, hash no banco, reaproveitamento, troca com menos de 24 h,
// vencimento e limpeza.
func TestPushAvatarLinks(t *testing.T) {
	f := newPushAvatarFixture(t)
	links := NewPushAvatarLinks(f.db.Push, "https://central.test/")
	now := time.Now()
	links.now = func() time.Time { return now }

	without := f.account("sem-avatar", false)
	if url, key := links.ForAccount(f.ctx, without.ID); url != "" || key != "" {
		t.Fatalf("conta sem avatar devolveu link: %q %q", url, key)
	}
	if url, _ := links.ForSubject(f.ctx, "nao-existe-"+f.suffix); url != "" {
		t.Fatalf("subject desconhecido devolveu link: %q", url)
	}

	ana := f.account("ana", true)
	url, key := links.ForAccount(f.ctx, ana.ID)
	token := tokenOf(t, url)
	if len(token) != 43 {
		t.Fatalf("token com %d caracteres, esperado 43 (256 bits em base64url)", len(token))
	}
	if key == "" || strings.Contains(key, ana.ID) {
		t.Fatalf("authorKey %q vazio ou com o id da conta", key)
	}

	// O banco só tem o hash: o token em claro não acha nada.
	if _, _, err := f.db.Push.AvatarLinkAccount(f.ctx, []byte(token)); err != store.ErrNotFound {
		t.Fatalf("token em claro achou linha: %v", err)
	}
	accountID, expiresAt, err := f.db.Push.AvatarLinkAccount(f.ctx, hashGrant(token))
	if err != nil || accountID != ana.ID {
		t.Fatalf("hash do token: conta %q, erro %v", accountID, err)
	}
	if d := expiresAt.Sub(now); d < pushAvatarLinkTTL-time.Second || d > pushAvatarLinkTTL+time.Second {
		t.Fatalf("validade %v, esperado %v", d, pushAvatarLinkTTL)
	}

	// Pelo subject (o que o server-channel manda) sai o mesmo link.
	if url2, key2 := links.ForSubject(f.ctx, ana.OIDCSubject); url2 != url || key2 != key {
		t.Fatalf("pelo subject: %q %q, esperado %q %q", url2, key2, url, key)
	}

	// Com mais de 24 h pela frente, reaproveita.
	now = now.Add(23 * time.Hour)
	if url2, _ := links.ForAccount(f.ctx, ana.ID); url2 != url {
		t.Fatalf("link não reaproveitado com 25 h pela frente: %q", url2)
	}
	// Com menos, emite outro; o antigo continua valendo até vencer.
	now = now.Add(2 * time.Hour)
	url3, _ := links.ForAccount(f.ctx, ana.ID)
	if url3 == url {
		t.Fatal("link com 23 h pela frente foi reaproveitado")
	}
	if rec := f.get(token); rec.Code != http.StatusOK {
		t.Fatalf("link antigo ainda válido: status %d", rec.Code)
	}

	// Trocar o avatar muda o authorKey (o app baixa de novo).
	time.Sleep(2 * time.Millisecond)
	f.setAvatar(ana)
	if _, key4 := links.ForAccount(f.ctx, ana.ID); key4 == key {
		t.Fatal("authorKey não mudou com o avatar novo")
	}

	// Vencido some na limpeza.
	expiredToken, _ := newPushAvatarToken()
	if err := f.db.Push.CreateAvatarLink(f.ctx, hashGrant(expiredToken), ana.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	links.Purge(f.ctx)
	if n, err := f.db.Push.PurgeExpiredAvatarLinks(f.ctx); err != nil || n != 0 {
		t.Fatalf("link vencido continuou no banco depois da limpeza (%d, %v)", n, err)
	}
	if rec := f.get(token); rec.Code != http.StatusOK {
		t.Fatalf("limpeza apagou link válido: status %d", rec.Code)
	}
}

// A rota serve os bytes sem login, várias vezes, com o tipo e o cache
// certos; vencido, desconhecido e avatar removido dão o mesmo 404.
func TestGetPushAvatar(t *testing.T) {
	f := newPushAvatarFixture(t)
	links := NewPushAvatarLinks(f.db.Push, "https://central.test")
	ana := f.account("ana", true)
	url, _ := links.ForAccount(f.ctx, ana.ID)
	token := tokenOf(t, url)

	for i := range 2 {
		rec := f.get(token)
		if rec.Code != http.StatusOK {
			t.Fatalf("leitura %d: status %d (%s)", i+1, rec.Code, rec.Body.String())
		}
		if !bytes.Equal(rec.Body.Bytes(), pngBytes) {
			t.Fatalf("leitura %d: bytes diferentes do avatar", i+1)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("Content-Type %q", ct)
		}
		cc := rec.Header().Get("Cache-Control")
		age, err := strconv.Atoi(strings.TrimPrefix(cc, "private, max-age="))
		if err != nil || age <= int((pushAvatarLinkTTL-time.Minute).Seconds()) || age > int(pushAvatarLinkTTL.Seconds()) {
			t.Errorf("Cache-Control %q", cc)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("rota de avatar do push não deveria ter CORS")
		}
	}

	unknown := f.get("nao-existe")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("token desconhecido: status %d", unknown.Code)
	}
	expiredToken, _ := newPushAvatarToken()
	if err := f.db.Push.CreateAvatarLink(f.ctx, hashGrant(expiredToken), ana.ID, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	expired := f.get(expiredToken)
	if expired.Code != http.StatusNotFound || expired.Body.String() != unknown.Body.String() {
		t.Fatalf("vencido: %d %q, desconhecido: %q", expired.Code, expired.Body.String(), unknown.Body.String())
	}
	if err := f.files.Delete(ana.ID); err != nil {
		t.Fatal(err)
	}
	if removed := f.get(token); removed.Code != http.StatusNotFound || removed.Body.String() != unknown.Body.String() {
		t.Fatalf("avatar removido: %d %q", removed.Code, removed.Body.String())
	}
}

// notify leva authorAvatar quando o autor (pelo authorSubject) tem avatar e
// omite quando não tem ou quando o server-channel não manda o campo; DM e
// amizade levam o de quem mandou.
func TestPushNotifyAuthorAvatar(t *testing.T) {
	f := newPushAvatarFixture(t)
	sender := &fakeSender{}
	dispatcher := push.NewDispatcher(sender, f.db.Push, 2, 100)
	dispatcher.SetAuthorAvatars(NewPushAvatarLinks(f.db.Push, "https://central.test"))

	marina := f.account("marina", true)
	semFoto := f.account("sem-foto", false)
	leitor := f.account("leitor", false)
	address := "https://srv-" + f.suffix + ".example"
	if _, err := f.db.KnownServers.Add(f.ctx, leitor.ID, address, "Servidor", nil); err != nil {
		t.Fatal(err)
	}
	grant := "grant-" + f.suffix
	if err := f.db.Push.CreateGrant(f.ctx, leitor.ID, address, hashGrant(grant)); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Push.UpsertDevice(f.ctx, leitor.ID, "fcm-leitor-"+f.suffix, "android"); err != nil {
		t.Fatal(err)
	}

	h := handlePushNotify(f.db.Push, dispatcher, newPushLimits())
	notify := func(subject string) map[string]string {
		t.Helper()
		body := map[string]any{"serverAddress": address, "channelId": "c1", "channelName": "geral", "author": "X", "text": "oi", "grants": []string{grant}}
		if subject != "" {
			body["authorSubject"] = subject
		}
		raw, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/push/notify", bytes.NewReader(raw)))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("notify: status %d", rec.Code)
		}
		dispatcher.Wait()
		sent := sender.take()
		if len(sent) != 1 {
			t.Fatalf("esperava 1 push, veio %d", len(sent))
		}
		return sent[0].data
	}

	d := notify(marina.OIDCSubject)
	marinaLink := d["authorAvatar"]
	token := tokenOf(t, marinaLink)
	if d["authorKey"] == "" {
		t.Error("authorKey vazio com authorAvatar")
	}
	if rec := f.get(token); rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Fatalf("link do push não serve o avatar: %d", rec.Code)
	}
	for _, subject := range []string{"", semFoto.OIDCSubject, "desconhecido-" + f.suffix} {
		if d := notify(subject); d["authorAvatar"] != "" || d["authorKey"] != "" {
			t.Errorf("subject %q: authorAvatar %q authorKey %q, esperado sem", subject, d["authorAvatar"], d["authorKey"])
		}
	}

	pushFromAccount(dispatcher, f.db.Profiles, push.TypeDM, leitor.ID, marina.ID, "")
	pushFromAccount(dispatcher, f.db.Profiles, push.TypeFriendRequest, leitor.ID, semFoto.ID, "req")
	dispatcher.Wait()
	byType := map[string]map[string]string{}
	for _, s := range sender.take() {
		byType[s.data["type"]] = s.data
	}
	// A DM reaproveita o link que já saiu para a marina.
	if got := byType[push.TypeDM]["authorAvatar"]; got != marinaLink {
		t.Errorf("dm: authorAvatar %q, esperado %q", got, marinaLink)
	}
	if got := byType[push.TypeFriendRequest]["authorAvatar"]; got != "" {
		t.Errorf("friend_request de quem não tem avatar: authorAvatar %q", got)
	}
}
