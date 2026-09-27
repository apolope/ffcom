package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/relay"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// Teste de integração contra um Postgres de verdade: só roda com
// FFCOM_TEST_DATABASE_URL apontando para um banco descartável (as
// migrations são aplicadas nele), mesmo padrão de server-channel. O relay é
// falso (httptest): guarda o prompt e a URL de callback, e o teste faz o
// papel do relay chamando o listener de callback.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbURL := os.Getenv("FFCOM_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("FFCOM_TEST_DATABASE_URL não definida")
	}
	db, err := store.Open(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

type fakeRelay struct {
	mu       sync.Mutex
	prompts  []string
	webhooks []string
	fail     bool
}

func (f *fakeRelay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct{ Prompt, WebhookURL string }
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &struct {
		Prompt     *string `json:"prompt"`
		WebhookURL *string `json:"webhookUrl"`
	}{&body.Prompt, &body.WebhookURL})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail || r.Header.Get("X-API-KEY") != "chave" {
		http.Error(w, "fora", http.StatusServiceUnavailable)
		return
	}
	f.prompts = append(f.prompts, body.Prompt)
	f.webhooks = append(f.webhooks, body.WebhookURL)
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"jobId":"relay-%d"}`, len(f.prompts))
}

func (f *fakeRelay) last() (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prompts[len(f.prompts)-1], f.webhooks[len(f.webhooks)-1]
}

func TestIdeasEndToEnd(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()

	fake := &fakeRelay{}
	relaySrv := httptest.NewServer(fake)
	t.Cleanup(relaySrv.Close)
	cfg := IdeasConfig{
		Relay:            relay.New(relaySrv.URL, "chave"),
		CallbackBaseURL:  "http://ffcom-central-app:8090",
		CallbackSecret:   "segredo",
		WandPerDay:       3,
		OffensivePenalty: 2,
		AdminGroup:       "ffcom-admins",
		ImproveTimeout:   time.Hour,
		CheckTimeout:     time.Hour,
	}
	callback := NewCallbackHandler(db, cfg)

	mux := http.NewServeMux()
	mux.Handle("GET /api/ideas", handleListIdeas(db.Ideas, cfg))
	mux.Handle("GET /api/ideas/me", handleIdeasMe(db, cfg))
	mux.Handle("POST /api/ideas", handleCreateIdea(db, cfg))
	mux.Handle("POST /api/ideas/assist", handleCreateAssist(db, cfg))
	mux.Handle("GET /api/ideas/assist/{id}", handleGetAssist(db, cfg))
	mux.Handle("PUT /api/ideas/{id}/vote", handleVoteIdea(db, cfg))
	mux.Handle("PATCH /api/ideas/{id}", handleModerateIdea(db, cfg))

	suffix := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	newAccount := func(subject, name string) store.Account {
		t.Helper()
		a, err := db.Accounts.GetOrCreateBySubject(ctx, subject+"-"+suffix, &name)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	alice := newAccount("alice", "Alice Souza")
	bob := newAccount("bob", "Bob Lima")
	carol := newAccount("carol", "Carol")
	admin := newAccount("admin", "Admin")

	call := func(account *store.Account, method, path string, body any, groups ...string) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(method, path, reader)
		if account != nil {
			req = req.WithContext(auth.WithAccount(req.Context(), *account, groups...))
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder, v any) {
		t.Helper()
		if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
			t.Fatalf("resposta %d não é JSON: %s", rec.Code, rec.Body.String())
		}
	}
	// relayResponde faz o papel do relay: chama o callback da última URL
	// recebida com o status e o texto do Claude.
	relayResponde := func(status, result string) {
		t.Helper()
		_, webhook := fake.last()
		u, err := url.Parse(webhook)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(relay.Callback{JobID: "x", Status: status, Result: result, Error: "falhou"})
		req := httptest.NewRequest(http.MethodPost, u.Path+"?"+u.RawQuery, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		callback.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("callback respondeu %d", rec.Code)
		}
	}
	jobID := func() string {
		_, webhook := fake.last()
		u, _ := url.Parse(webhook)
		return u.Query().Get("job")
	}
	// esperaPedido espera o callback (processado em goroutine) fechar o pedido.
	esperaPedido := func(id string) store.AssistJob {
		t.Helper()
		for range 100 {
			job, err := db.Ideas.GetJob(ctx, id)
			if err == nil && job.Status != store.AssistPending {
				return job
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("pedido %s não fechou", id)
		return store.AssistJob{}
	}
	esperaIdeia := func(id string) store.Idea {
		t.Helper()
		for range 100 {
			idea, err := db.Ideas.Get(ctx, id, "")
			if err == nil && idea.Status != store.IdeaChecking {
				return idea
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("ideia %s não saiu de checking", id)
		return store.Idea{}
	}
	var me ideasMeResponse
	// saldoVolta espera a devolução do uso, que acontece logo depois de o
	// pedido fechar, na mesma goroutine do callback.
	saldoVolta := func(account store.Account, want int) ideasMeResponse {
		t.Helper()
		var m ideasMeResponse
		for range 100 {
			decode(call(&account, "GET", "/api/ideas/me", nil), &m)
			if m.WandLeft == want {
				return m
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("saldo não chegou a %d: %+v", want, m)
		return m
	}
	getMe := func(account store.Account) ideasMeResponse {
		t.Helper()
		var m ideasMeResponse
		decode(call(&account, "GET", "/api/ideas/me", nil), &m)
		return m
	}

	// Callback sem o segredo certo é recusado.
	rec := httptest.NewRecorder()
	callback.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/claude-callback?secret=errado&job=x", strings.NewReader(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("callback com segredo errado: %d", rec.Code)
	}

	// --- Varinha: sucesso gasta 1 uso.
	var assist assistResponse
	r := call(&alice, "POST", "/api/ideas/assist", map[string]string{"text": "queria um tema escuro no app, o branco cansa"})
	if r.Code != http.StatusAccepted {
		t.Fatalf("varinha: %d %s", r.Code, r.Body.String())
	}
	decode(r, &assist)
	if assist.WandLeft != 2 {
		t.Fatalf("saldo depois do primeiro uso = %d", assist.WandLeft)
	}
	if prompt, _ := fake.last(); !strings.Contains(prompt, "queria um tema escuro") {
		t.Fatal("prompt não levou o texto")
	}
	// Um pedido por vez.
	if r := call(&alice, "POST", "/api/ideas/assist", map[string]string{"text": "outro texto qualquer aqui"}); r.Code != http.StatusConflict {
		t.Fatalf("segundo pedido simultâneo: %d", r.Code)
	}
	relayResponde("success", `{"ofensiva": false, "sugestao": true, "titulo": "Tema escuro", "texto": "Adicionar um tema escuro ao app.", "parecidas": []}`)
	esperaPedido(assist.ID)
	decode(call(&alice, "GET", "/api/ideas/assist/"+assist.ID, nil), &assist)
	if assist.Status != store.AssistDone || assist.Result == nil || assist.Result.Text != "Adicionar um tema escuro ao app." || assist.WandLeft != 2 {
		t.Fatalf("resultado da varinha: %+v", assist)
	}
	// Pedido de outra pessoa não aparece.
	if r := call(&bob, "GET", "/api/ideas/assist/"+assist.ID, nil); r.Code != http.StatusNotFound {
		t.Fatalf("pedido alheio: %d", r.Code)
	}

	// --- Varinha: ideia ofensiva gasta o uso e mais a penalidade.
	decode(call(&alice, "POST", "/api/ideas/assist", map[string]string{"text": "texto ofensivo de teste aqui"}), &assist)
	relayResponde("success", `{"ofensiva": true, "titulo": "", "texto": "", "parecidas": []}`)
	esperaPedido(assist.ID)
	saldoVolta(alice, 0)
	decode(call(&alice, "GET", "/api/ideas/assist/"+assist.ID, nil), &assist)
	if !assist.Result.Offensive || assist.WandLeft != 0 {
		t.Fatalf("depois da ofensiva: %+v", assist)
	}
	if me = getMe(alice); me.WandLeft != 0 || me.WandUsed != 2 || me.WandPenalty != 2 {
		t.Fatalf("saldo da alice: %+v", me)
	}
	if r := call(&alice, "POST", "/api/ideas/assist", map[string]string{"text": "mais uma tentativa de texto"}); r.Code != http.StatusTooManyRequests {
		t.Fatalf("varinha sem saldo: %d", r.Code)
	}

	// --- Varinha: falha do relay devolve o uso.
	decode(call(&bob, "POST", "/api/ideas/assist", map[string]string{"text": "reações com emoji nas mensagens"}), &assist)
	relayResponde("error", "")
	if job := esperaPedido(assist.ID); job.Status != store.AssistFailed {
		t.Fatalf("pedido com erro do relay: %s", job.Status)
	}
	saldoVolta(bob, 3)
	// Resposta fora do formato também devolve.
	decode(call(&bob, "POST", "/api/ideas/assist", map[string]string{"text": "reações com emoji nas mensagens"}), &assist)
	relayResponde("success", "Desculpe, não posso ajudar.")
	esperaPedido(assist.ID)
	saldoVolta(bob, 3)
	// Relay fora do ar na hora de enviar: 502 e uso devolvido.
	fake.mu.Lock()
	fake.fail = true
	fake.mu.Unlock()
	if r := call(&bob, "POST", "/api/ideas/assist", map[string]string{"text": "reações com emoji nas mensagens"}); r.Code != http.StatusBadGateway {
		t.Fatalf("relay fora: %d", r.Code)
	}
	fake.mu.Lock()
	fake.fail = false
	fake.mu.Unlock()
	if me = getMe(bob); me.WandLeft != 3 || me.PendingAssist != nil {
		t.Fatalf("depois do relay fora: %+v", me)
	}

	// --- Sugestão: checagem ok publica com o texto limpo e o título.
	var bobIdea ideaResponse
	r = call(&bob, "POST", "/api/ideas", map[string]string{"text": "Reações com emoji nas mensagens, porra"})
	if r.Code != http.StatusAccepted {
		t.Fatalf("enviar ideia: %d %s", r.Code, r.Body.String())
	}
	decode(r, &bobIdea)
	if bobIdea.Status != store.IdeaChecking {
		t.Fatalf("ideia nova deveria estar em checking: %s", bobIdea.Status)
	}
	if prompt, _ := fake.last(); !strings.Contains(prompt, "Não reescreva") {
		t.Fatal("checagem deveria usar o prompt de checagem")
	}
	relayResponde("success", `{"ofensiva": false, "sugestao": true, "titulo": "Reações com emoji", "texto": "Reações com emoji nas mensagens, por favor"}`)
	if idea := esperaIdeia(bobIdea.ID); idea.Status != store.IdeaOpen || idea.Title != "Reações com emoji" || !strings.HasSuffix(idea.Body, "por favor") {
		t.Fatalf("ideia depois da checagem: %+v", idea)
	}
	// Uma por dia.
	if r := call(&bob, "POST", "/api/ideas", map[string]string{"text": "Mais uma ideia no mesmo dia"}); r.Code != http.StatusConflict {
		t.Fatalf("segunda ideia no dia: %d", r.Code)
	}
	if me = getMe(bob); !me.SuggestedToday || me.TodayIdea == nil || me.TodayIdea.ID != bobIdea.ID {
		t.Fatalf("me do bob: %+v", me)
	}
	// A checagem não gasta varinha.
	if me.WandLeft != 3 {
		t.Fatalf("checagem gastou varinha: %+v", me)
	}

	// --- Texto que não é sugestão: a varinha só devolve a dica, gastando o
	// uso normal e sem penalidade.
	daniel := newAccount("daniel", "Daniel Rocha")
	decode(call(&daniel, "POST", "/api/ideas/assist", map[string]string{"text": "aplicativo muito ruim"}), &assist)
	relayResponde("success", `{"ofensiva": false, "sugestao": false, "motivo": "Diga o que você gostaria que mudasse.", "titulo": "", "texto": "", "parecidas": []}`)
	esperaPedido(assist.ID)
	decode(call(&daniel, "GET", "/api/ideas/assist/"+assist.ID, nil), &assist)
	if !assist.Result.NotSuggestion || assist.Result.Offensive || assist.Result.Hint != "Diga o que você gostaria que mudasse." || assist.WandLeft != 2 {
		t.Fatalf("varinha com texto que não é sugestão: %+v", assist.Result)
	}
	if me = getMe(daniel); me.WandPenalty != 0 {
		t.Fatalf("não-sugestão não deveria penalizar: %+v", me)
	}

	// --- No envio, é descartada, não gasta o dia e devolve a dica.
	var danielIdea ideaResponse
	decode(call(&daniel, "POST", "/api/ideas", map[string]string{"text": "aplicativo muito ruim"}), &danielIdea)
	relayResponde("success", `{"ofensiva": false, "sugestao": false, "motivo": "Conte o que te incomoda para virar uma sugestão.", "titulo": "", "texto": ""}`)
	if idea := esperaIdeia(danielIdea.ID); idea.Status != store.IdeaDiscarded || idea.Feedback == nil {
		t.Fatalf("ideia que não é sugestão: %+v", idea)
	}
	me = getMe(daniel)
	if me.SuggestedToday || me.LastDiscarded == nil || me.LastDiscarded.ID != danielIdea.ID ||
		me.LastDiscarded.Feedback == nil || *me.LastDiscarded.Feedback != "Conte o que te incomoda para virar uma sugestão." || me.DiscardsLeft != 2 {
		t.Fatalf("me depois do descarte: %+v", me)
	}
	var publicas []ideaResponse
	decode(call(nil, "GET", "/api/ideas", nil), &publicas)
	for _, i := range publicas {
		if i.ID == danielIdea.ID {
			t.Fatal("descartada não pode aparecer no ranking")
		}
	}
	// Mais dois descartes esgotam a cota; o próximo envio é recusado.
	for i := range 2 {
		var d ideaResponse
		decode(call(&daniel, "POST", "/api/ideas", map[string]string{"text": fmt.Sprintf("teste de texto %d", i)}), &d)
		relayResponde("success", `{"ofensiva": false, "sugestao": false, "motivo": "", "titulo": "", "texto": ""}`)
		esperaIdeia(d.ID)
	}
	if r := call(&daniel, "POST", "/api/ideas", map[string]string{"text": "agora uma sugestão de verdade"}); r.Code != http.StatusTooManyRequests {
		t.Fatalf("depois de 3 descartes: %d", r.Code)
	}
	if me = getMe(daniel); me.DiscardsLeft != 0 || me.LastDiscarded == nil || me.LastDiscarded.Feedback == nil || *me.LastDiscarded.Feedback != defaultHint {
		t.Fatalf("cota de descartes: %+v", me)
	}

	// --- Sugestão ofensiva na checagem final: vai para moderação.
	var aliceIdea ideaResponse
	decode(call(&alice, "POST", "/api/ideas", map[string]string{"text": "Uma ideia ofensiva de teste para moderar"}), &aliceIdea)
	relayResponde("success", `{"ofensiva": true, "titulo": "", "texto": ""}`)
	if idea := esperaIdeia(aliceIdea.ID); idea.Status != store.IdeaReview || idea.ReviewReason == nil || *idea.ReviewReason != reasonOffensive {
		t.Fatalf("ideia ofensiva: %+v", idea)
	}

	// --- Relay fora do ar no envio: vai para moderação sem checagem.
	fake.mu.Lock()
	fake.fail = true
	fake.mu.Unlock()
	var carolIdea ideaResponse
	decode(call(&carol, "POST", "/api/ideas", map[string]string{"text": "Busca dentro das mensagens do canal"}), &carolIdea)
	fake.mu.Lock()
	fake.fail = false
	fake.mu.Unlock()
	if idea := esperaIdeia(carolIdea.ID); idea.Status != store.IdeaReview || *idea.ReviewReason != reasonNoCheck {
		t.Fatalf("ideia sem checagem: %+v", idea)
	}

	// --- Ranking público só mostra as publicadas; moderação só para admin.
	contains := func(list []ideaResponse, id string) *ideaResponse {
		for i := range list {
			if list[i].ID == id {
				return &list[i]
			}
		}
		return nil
	}
	var ranking []ideaResponse
	decode(call(nil, "GET", "/api/ideas", nil), &ranking)
	if contains(ranking, bobIdea.ID) == nil || contains(ranking, aliceIdea.ID) != nil || contains(ranking, carolIdea.ID) != nil {
		t.Fatal("ranking público mostrando o que não devia")
	}
	if got := contains(ranking, bobIdea.ID); got.Author != "Bob" || got.ReviewReason != nil {
		t.Fatalf("autor deveria ser só o primeiro nome: %+v", got)
	}
	if r := call(&bob, "GET", "/api/ideas?view=review", nil); r.Code != http.StatusForbidden {
		t.Fatalf("moderação para não admin: %d", r.Code)
	}
	var review []ideaResponse
	decode(call(&admin, "GET", "/api/ideas?view=review", nil, "ffcom-admins"), &review)
	if contains(review, aliceIdea.ID) == nil || contains(review, carolIdea.ID) == nil {
		t.Fatal("fila de moderação sem as ideias retidas")
	}

	// --- Votos: like +2, dislike -1, sem votar na própria.
	vote := func(account store.Account, id string, value int) ideaResponse {
		t.Helper()
		r := call(&account, "PUT", "/api/ideas/"+id+"/vote", map[string]int{"value": value})
		if r.Code != http.StatusOK {
			t.Fatalf("voto: %d %s", r.Code, r.Body.String())
		}
		var idea ideaResponse
		decode(r, &idea)
		return idea
	}
	vote(alice, bobIdea.ID, 1)
	if got := vote(carol, bobIdea.ID, -1); got.Score != 1 || got.Likes != 1 || got.Dislikes != 1 || got.MyVote != -1 {
		t.Fatalf("pontuação: %+v", got)
	}
	if got := vote(carol, bobIdea.ID, 1); got.Score != 4 {
		t.Fatalf("trocar voto: %+v", got)
	}
	if got := vote(carol, bobIdea.ID, 0); got.Score != 2 || got.MyVote != 0 {
		t.Fatalf("tirar voto: %+v", got)
	}
	if r := call(&bob, "PUT", "/api/ideas/"+bobIdea.ID+"/vote", map[string]int{"value": 1}); r.Code != http.StatusForbidden {
		t.Fatalf("votar na própria: %d", r.Code)
	}
	if r := call(&bob, "PUT", "/api/ideas/"+aliceIdea.ID+"/vote", map[string]int{"value": 1}); r.Code != http.StatusConflict {
		t.Fatalf("votar em ideia retida: %d", r.Code)
	}

	// --- Admin aprova a ideia retida e o ranking ordena por pontuação.
	if r := call(&bob, "PATCH", "/api/ideas/"+aliceIdea.ID, map[string]string{"status": "open"}); r.Code != http.StatusForbidden {
		t.Fatalf("moderar sem ser admin: %d", r.Code)
	}
	if r := call(&admin, "PATCH", "/api/ideas/"+aliceIdea.ID, map[string]string{"status": "open"}, "ffcom-admins"); r.Code != http.StatusOK {
		t.Fatalf("aprovar: %d %s", r.Code, r.Body.String())
	}
	vote(bob, aliceIdea.ID, -1)
	decode(call(&carol, "GET", "/api/ideas", nil), &ranking)
	posBob, posAlice := -1, -1
	for i, idea := range ranking {
		switch idea.ID {
		case bobIdea.ID:
			posBob = i
		case aliceIdea.ID:
			posAlice = i
			if idea.Title == "" {
				t.Fatal("ideia aprovada sem título deveria ganhar um")
			}
		}
	}
	if posBob < 0 || posAlice < 0 || posBob > posAlice {
		t.Fatalf("ordem do ranking: bob=%d alice=%d", posBob, posAlice)
	}

	// --- Varinha aponta ideias parecidas entre as do topo.
	decode(call(&carol, "POST", "/api/ideas/assist", map[string]string{"text": "dava pra ter emoji nas reações?"}), &assist)
	prompt, _ := fake.last()
	number := 0
	for i, line := range strings.Split(prompt, "\n") {
		_ = i
		if strings.Contains(line, "Reações com emoji:") {
			fmt.Sscanf(line, "%d.", &number)
		}
	}
	if number == 0 {
		t.Fatalf("a ideia do bob deveria estar no prompt:\n%s", prompt)
	}
	relayResponde("success", fmt.Sprintf(`{"ofensiva": false, "sugestao": true, "titulo": "Emoji", "texto": "Reações com emoji.", "parecidas": [%d]}`, number))
	esperaPedido(assist.ID)
	decode(call(&carol, "GET", "/api/ideas/assist/"+assist.ID, nil), &assist)
	if len(assist.Result.Similar) != 1 || assist.Result.Similar[0].ID != bobIdea.ID || assist.Result.Similar[0].MyVote != 0 {
		t.Fatalf("parecidas: %+v", assist.Result.Similar)
	}

	// --- Implementada com a versão do CHANGELOG.
	if r := call(&admin, "PATCH", "/api/ideas/"+bobIdea.ID, map[string]any{"status": "implemented", "implementedVersion": "v0.15.0"}, "ffcom-admins"); r.Code != http.StatusBadRequest {
		t.Fatalf("versão fora do formato: %d", r.Code)
	}
	if r := call(&admin, "PATCH", "/api/ideas/"+bobIdea.ID, map[string]any{"status": "implemented", "implementedVersion": "client v0.15.0"}, "ffcom-admins"); r.Code != http.StatusOK {
		t.Fatalf("implementar: %d %s", r.Code, r.Body.String())
	}
	var implemented []ideaResponse
	decode(call(nil, "GET", "/api/ideas?view=implemented", nil), &implemented)
	if got := contains(implemented, bobIdea.ID); got == nil || got.ImplementedVersion == nil || *got.ImplementedVersion != "client v0.15.0" {
		t.Fatalf("implementadas: %+v", got)
	}
	decode(call(nil, "GET", "/api/ideas", nil), &ranking)
	if contains(ranking, bobIdea.ID) != nil {
		t.Fatal("implementada não deveria ficar no ranking")
	}

	// --- Prazo: o sweeper fecha o pedido sem callback e devolve o uso; o
	// callback atrasado não muda mais nada.
	cfgExpira := cfg
	cfgExpira.ImproveTimeout = 0
	sweeper := &ideaAssistant{cfg: cfgExpira, ideas: db.Ideas}
	before := getMe(carol).WandLeft
	decode(call(&carol, "POST", "/api/ideas/assist", map[string]string{"text": "um texto que vai expirar"}), &assist)
	late := jobID()
	time.Sleep(10 * time.Millisecond)
	sweeper.sweep(ctx)
	if job, _ := db.Ideas.GetJob(ctx, late); job.Status != store.AssistFailed {
		t.Fatalf("pedido vencido: %s", job.Status)
	}
	if got := getMe(carol).WandLeft; got != before {
		t.Fatalf("uso do pedido vencido não voltou: antes %d, depois %d", before, got)
	}
	relayResponde("success", `{"ofensiva": true, "titulo": "", "texto": "", "parecidas": []}`)
	time.Sleep(100 * time.Millisecond)
	if got := getMe(carol); got.WandPenalty != 0 {
		t.Fatalf("callback atrasado aplicou penalidade: %+v", got)
	}
}
