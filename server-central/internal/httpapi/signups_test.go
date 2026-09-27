package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/authentik"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
	"a3sitsolutions.com/ffcom/server-central/internal/telegram"
)

func TestSignupValidate(t *testing.T) {
	ok := signupBody{FullName: "  Maria   da Silva ", Username: "Maria.Silva", Email: " Maria@Exemplo.com.br ", Nickname: "Mari", Reason: "Um amigo me indicou\r\n\r\npara jogar"}
	n, problem := ok.validate()
	if problem != "" {
		t.Fatalf("pedido válido recusado: %s", problem)
	}
	if n.FullName != "Maria da Silva" || n.Username != "maria.silva" || n.Email != "maria@exemplo.com.br" || n.Reason != "Um amigo me indicou\npara jogar" {
		t.Fatalf("normalização errada: %+v", n)
	}

	bad := []signupBody{
		{FullName: "M", Username: "maria", Email: "m@x.com", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria", Username: "ma", Email: "m@x.com", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria", Username: "maria silva", Email: "m@x.com", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria", Username: "maria", Email: "Maria <m@x.com>", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria", Username: "maria", Email: "m@localhost", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria\nX", Username: "maria", Email: "m@x.com", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria", Username: "maria", Email: "m@x.com", Nickname: "M", Reason: "motivo suficiente"},
		{FullName: "Maria", Username: "maria", Email: "m@x.com", Nickname: "Mari", Reason: "curto"},
		{FullName: "Maria", Username: "admin", Email: "m@x.com", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria", Username: "Ad.Min", Email: "m@x.com", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria", Username: "ak_admin", Email: "m@x.com", Nickname: "Mari", Reason: "motivo suficiente"},
		{FullName: "Maria", Email: "m@x.com", Reason: "motivo suficiente"},
	}
	for i, b := range bad {
		if _, problem := b.validate(); problem == "" {
			t.Errorf("caso %d deveria ser recusado: %+v", i, b)
		}
	}

	// Quem já tem conta não informa usuário nem apelido; se vierem, são
	// descartados.
	existing := signupBody{FullName: "Maria", Username: "admin", Email: "m@x.com", Nickname: "x", Reason: "motivo suficiente", ExistingAccount: true}
	n, problem = existing.validate()
	if problem != "" || n.Username != "" || n.Nickname != "" || !n.ExistingAccount {
		t.Fatalf("pedido de quem já tem conta: %q %+v", problem, n)
	}
}

func TestSignupMessageEscapesHTML(t *testing.T) {
	req := store.SignupRequest{ID: "x", FullName: "<b>Ana</b>", Username: "ana", Email: "a@x.com", Nickname: "A&B", Reason: "quero <script>", CreatedAt: time.Now()}
	msg := signupMessage(req)
	if strings.Contains(msg, "<b>Ana</b>") || strings.Contains(msg, "<script>") || !strings.Contains(msg, "A&amp;B") {
		t.Fatalf("mensagem sem escape: %s", msg)
	}
}

// fakeTelegram faz o papel da Bot API: guarda as mensagens enviadas e
// editadas.
type fakeTelegram struct {
	mu     sync.Mutex
	sent   []map[string]any
	edited []map[string]any
	fail   bool
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	if !strings.HasPrefix(r.URL.Path, "/botTOKEN/") {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"description":"Unauthorized"}`)
		return
	}
	if f.fail {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `{"ok":false,"description":"fora"}`)
		return
	}
	switch strings.TrimPrefix(r.URL.Path, "/botTOKEN/") {
	case "sendMessage":
		f.sent = append(f.sent, body)
		fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d}}`, 100+len(f.sent))
	case "editMessageText":
		f.edited = append(f.edited, body)
		fmt.Fprint(w, `{"ok":true,"result":{}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"ok":false,"description":"Not Found"}`)
	}
}

// fakeAuthentik faz o papel da API do Authentik com um grupo e os usuários
// criados.
type fakeAuthentik struct {
	mu        sync.Mutex
	users     []authentik.User
	members   map[int64]bool
	recovery  []int64
	failEmail bool
}

func (f *fakeAuthentik) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer ak-token" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v3")
	switch {
	case r.Method == http.MethodGet && path == "/core/users/":
		// Como a busca livre do Authentik: trecho, sem diferenciar
		// maiúsculas, em nome de usuário, nome ou e-mail.
		term := strings.ToLower(r.URL.Query().Get("search"))
		var results []authentik.User
		for _, u := range f.users {
			if term != "" && (strings.Contains(strings.ToLower(u.Username), term) || strings.Contains(strings.ToLower(u.Email), term)) {
				results = append(results, u)
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"results": results})
	case r.Method == http.MethodGet && path == "/core/groups/":
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]string{{"pk": "11111111-1111-1111-1111-111111111111", "name": r.URL.Query().Get("name")}}})
	case r.Method == http.MethodPost && path == "/core/users/":
		var body struct {
			Username   string         `json:"username"`
			Name       string         `json:"name"`
			Email      string         `json:"email"`
			Attributes map[string]any `json:"attributes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		u := authentik.User{PK: int64(len(f.users) + 1), Username: body.Username, Email: body.Email, IsActive: true, Type: "internal", Attributes: body.Attributes}
		u.Attributes["name"] = body.Name
		f.users = append(f.users, u)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(u)
	case r.Method == http.MethodPost && path == "/core/groups/11111111-1111-1111-1111-111111111111/add_user/":
		var body struct {
			PK int64 `json:"pk"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.members[body.PK] = true
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/recovery_email/"):
		var body struct {
			EmailStage    string `json:"email_stage"`
			TokenDuration string `json:"token_duration"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if f.failEmail || body.EmailStage != "stage-uuid" || body.TokenDuration != "days=3" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"non_field_errors":["sem fluxo de recuperação"]}`)
			return
		}
		var pk int64
		fmt.Sscanf(strings.TrimPrefix(path, "/core/users/"), "%d", &pk)
		f.recovery = append(f.recovery, pk)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSignupEndToEnd(t *testing.T) {
	db := openTestStore(t)

	tg := &fakeTelegram{}
	tgSrv := httptest.NewServer(tg)
	t.Cleanup(tgSrv.Close)
	ak := &fakeAuthentik{members: map[int64]bool{}}
	akSrv := httptest.NewServer(ak)
	t.Cleanup(akSrv.Close)

	cfg := SignupConfig{
		Telegram:              telegram.New("TOKEN", "-100123", 42).WithBaseURL(tgSrv.URL),
		Authentik:             authentik.New(akSrv.URL, "ak-token"),
		UsersGroup:            "ffcom-users",
		RecoveryEmailStage:    "stage-uuid",
		RecoveryTokenDuration: "days=3",
		DecisionSecret:        "segredo",
		MaxPerIPPerDay:        3,
		MaxPerHour:            1000,
	}
	public := http.NewServeMux()
	public.Handle("POST /api/signup-requests", handleCreateSignup(db, cfg))
	public.Handle("GET /api/signup-requests/username-available", handleUsernameAvailable(db, cfg))
	internal := NewInternalHandler(db, IdeasConfig{}, cfg)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	// IP único por execução, para o limite por IP não contar pedidos de
	// execuções anteriores no mesmo banco.
	ip := "a-" + suffix
	post := func(body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/signup-requests", bytes.NewReader(raw))
		req.Header.Set("X-Forwarded-For", ip)
		rec := httptest.NewRecorder()
		public.ServeHTTP(rec, req)
		return rec
	}
	signup := func(username, email, website string) *httptest.ResponseRecorder {
		t.Helper()
		return post(map[string]any{
			"fullName": "Maria da Silva", "username": username, "email": email,
			"nickname": "Mari", "reason": "Um amigo me chamou para o servidor", "website": website,
		})
	}
	signupExisting := func(email string) *httptest.ResponseRecorder {
		t.Helper()
		return post(map[string]any{
			"fullName": "Bia Souza", "email": email, "existingAccount": true,
			"reason": "Já uso outro serviço da infra e quero o FFCom",
		})
	}
	decide := func(id, decision, secret string) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"decision": decision, "by": "@apolonio"})
		req := httptest.NewRequest(http.MethodPost, "/internal/signup-requests/"+id+"/decision", bytes.NewReader(raw))
		req.Header.Set("X-FFCom-Secret", secret)
		rec := httptest.NewRecorder()
		internal.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	lastSent := func() (string, []string) {
		tg.mu.Lock()
		defer tg.mu.Unlock()
		msg := tg.sent[len(tg.sent)-1]
		var data []string
		markup := msg["reply_markup"].(map[string]any)["inline_keyboard"].([]any)[0].([]any)
		for _, b := range markup {
			data = append(data, b.(map[string]any)["callback_data"].(string))
		}
		if msg["message_thread_id"].(float64) != 42 {
			t.Fatalf("mensagem fora do tópico: %v", msg)
		}
		return msg["text"].(string), data
	}
	idFrom := func(data string) string {
		return data[strings.LastIndex(data, ":")+1:]
	}

	userA := "maria" + suffix
	emailA := "maria" + suffix + "@exemplo.com"

	// Robô (honeypot preenchido): parece sucesso, nada vai ao Telegram.
	if rec := signup("robo"+suffix, "robo"+suffix+"@exemplo.com", "http://spam"); rec.Code != http.StatusCreated || len(tg.sent) != 0 {
		t.Fatalf("honeypot: %d, %d mensagens", rec.Code, len(tg.sent))
	}

	// Pedido válido vai ao Telegram com os dois botões.
	if rec := signup(userA, emailA, ""); rec.Code != http.StatusCreated {
		t.Fatalf("cadastro: %d %s", rec.Code, rec.Body.String())
	}
	text, data := lastSent()
	if !strings.Contains(text, userA) || len(data) != 2 || !strings.HasPrefix(data[0], "ffcom-signup:approve:") || !strings.HasPrefix(data[1], "ffcom-signup:reject:") {
		t.Fatalf("mensagem inesperada: %s %v", text, data)
	}
	for _, d := range data {
		if len(d) > 64 {
			t.Fatalf("callback_data passa de 64 bytes: %s", d)
		}
	}
	idA := idFrom(data[0])

	// Mesmo e-mail com pedido em aberto: conflito.
	if rec := signup("outro"+suffix, emailA, ""); rec.Code != http.StatusConflict {
		t.Fatalf("pedido duplicado: %d", rec.Code)
	}

	// Segredo errado não decide nada.
	if code, _ := decide(idA, "approve", "errado"); code != http.StatusUnauthorized {
		t.Fatalf("segredo errado: %d", code)
	}

	// Aprovar cria o usuário com o apelido como nome, põe no grupo e manda o
	// e-mail de senha; a mensagem perde os botões.
	code, out := decide(idA, "approve", "segredo")
	if code != http.StatusOK || out["ok"] != true {
		t.Fatalf("aprovar: %d %v", code, out)
	}
	if len(ak.users) != 1 || ak.users[0].Username != userA || ak.users[0].Attributes["name"] != "Mari" || !ak.members[1] || len(ak.recovery) != 1 {
		t.Fatalf("authentik depois de aprovar: %+v %v %v", ak.users, ak.members, ak.recovery)
	}
	if len(tg.edited) != 1 || tg.edited[0]["reply_markup"] != nil || !strings.Contains(tg.edited[0]["text"].(string), "Aprovado") {
		t.Fatalf("edição da mensagem: %v", tg.edited)
	}

	// Clicar de novo não cria outra conta.
	if _, out := decide(idA, "approve", "segredo"); out["ok"] != false || !strings.Contains(out["message"].(string), "já aprovado") {
		t.Fatalf("segundo clique: %v", out)
	}
	if _, out := decide(idA, "reject", "segredo"); out["ok"] != false {
		t.Fatalf("reprovar aprovado: %v", out)
	}
	if len(ak.users) != 1 {
		t.Fatalf("conta duplicada: %d", len(ak.users))
	}

	// Pedido de novo com o e-mail aprovado: conflito explicando.
	if rec := signup("maria2"+suffix, emailA, ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "aprovada") {
		t.Fatalf("pedido com e-mail aprovado: %d %s", rec.Code, rec.Body.String())
	}
	// Nome de usuário que já existe no Authentik: recusado na hora.
	if rec := signup(userA, "novo"+suffix+"@exemplo.com", ""); rec.Code != http.StatusConflict {
		t.Fatalf("usuário existente: %d %s", rec.Code, rec.Body.String())
	}

	// Reprovar: nada no Authentik, mensagem editada.
	userB := "joao" + suffix
	if rec := signup(userB, "joao"+suffix+"@exemplo.com", ""); rec.Code != http.StatusCreated {
		t.Fatalf("cadastro B: %d %s", rec.Code, rec.Body.String())
	}
	_, data = lastSent()
	idB := idFrom(data[1])
	if _, out := decide(idB, "reject", "segredo"); out["ok"] != true {
		t.Fatalf("reprovar: %v", out)
	}
	if len(ak.users) != 1 || !strings.Contains(tg.edited[len(tg.edited)-1]["text"].(string), "Reprovado") {
		t.Fatalf("depois de reprovar: %d usuários, %v", len(ak.users), tg.edited)
	}
	// Reprovado pode pedir de novo.
	if rec := signup(userB, "joao"+suffix+"@exemplo.com", ""); rec.Code != http.StatusCreated {
		t.Fatalf("novo pedido depois de reprovado: %d %s", rec.Code, rec.Body.String())
	}

	// Limite por IP (3 por dia; o honeypot não grava).
	if rec := signup("ze"+suffix, "ze"+suffix+"@exemplo.com", ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("limite por IP: %d %s", rec.Code, rec.Body.String())
	}

	// E-mail de senha falhando não desfaz a aprovação, e o aviso aparece.
	_, data = lastSent()
	idC := idFrom(data[0])
	ak.failEmail = true
	if _, out := decide(idC, "approve", "segredo"); out["ok"] != true || !strings.Contains(out["message"].(string), "falhou") {
		t.Fatalf("aprovar com e-mail falhando: %v", out)
	}
	req, err := db.Signups.Get(t.Context(), idC)
	if err != nil || req.Status != store.SignupApproved || req.Failure == nil {
		t.Fatalf("pedido C: %+v %v", req, err)
	}

	// E-mail que já é de outra conta no Authentik (instância compartilhada),
	// sem a pessoa marcar "já tenho conta": quem aprova é avisado desde o
	// envio, e a aprovação põe essa conta no grupo em vez de criar a pedida,
	// sem e-mail de senha (a conta já tem uma).
	ip = "c-" + suffix
	ak.failEmail = false
	ak.users = append(ak.users, authentik.User{PK: 99, Username: "Outro-Projeto" + suffix, Email: "Bia" + suffix + "@Exemplo.com", IsActive: true, Type: "internal"})
	if rec := signup("bia"+suffix, "bia"+suffix+"@exemplo.com", ""); rec.Code != http.StatusCreated {
		t.Fatalf("cadastro D: %d %s", rec.Code, rec.Body.String())
	}
	text, data = lastSent()
	if !strings.Contains(text, "Outro-Projeto"+suffix) || !strings.Contains(text, "sem criar o usuário pedido") {
		t.Fatalf("mensagem D não avisa da conta existente: %s", text)
	}
	idD := idFrom(data[0])
	usersBefore, recoveryBefore := len(ak.users), len(ak.recovery)
	if _, out := decide(idD, "approve", "segredo"); out["ok"] != true || !strings.Contains(out["message"].(string), "Outro-Projeto") {
		t.Fatalf("aprovar com e-mail de outra conta: %v", out)
	}
	if len(ak.users) != usersBefore || !ak.members[99] || len(ak.recovery) != recoveryBefore {
		t.Fatalf("vincular conta existente: %d usuários, grupo %v, %d e-mails", len(ak.users), ak.members, len(ak.recovery))
	}
	last := tg.edited[len(tg.edited)-1]
	if last["reply_markup"] != nil || !strings.Contains(last["text"].(string), "Conta existente") {
		t.Fatalf("mensagem de conta vinculada: %v", last)
	}
	if req, _ := db.Signups.Get(t.Context(), idD); req.Status != store.SignupApproved || !req.LinkedExisting || req.AuthentikUsername == nil || *req.AuthentikUsername != "Outro-Projeto"+suffix {
		t.Fatalf("pedido D: %+v", req)
	}

	// Nome de usuário que só difere em maiúsculas de um que existe no
	// Authentik também é recusado.
	if rec := signup("outro-projeto"+suffix, "op"+suffix+"@exemplo.com", ""); rec.Code != http.StatusConflict {
		t.Fatalf("usuário existente com outra caixa: %d %s", rec.Code, rec.Body.String())
	}

	// "Já tenho conta": sem usuário nem apelido; a aprovação só põe no grupo.
	ip = "d-" + suffix
	ak.users = append(ak.users, authentik.User{PK: 98, Username: "caio" + suffix, Email: "caio" + suffix + "@exemplo.com", IsActive: true, Type: "internal"})
	if rec := signupExisting("caio" + suffix + "@exemplo.com"); rec.Code != http.StatusCreated {
		t.Fatalf("pedido de quem tem conta: %d %s", rec.Code, rec.Body.String())
	}
	text, data = lastSent()
	if !strings.Contains(text, "já tem conta") || !strings.Contains(text, "Conta existente: <code>caio"+suffix) || strings.Contains(text, "Usuário:") {
		t.Fatalf("mensagem de quem tem conta: %s", text)
	}
	if _, out := decide(idFrom(data[0]), "approve", "segredo"); out["ok"] != true || !ak.members[98] {
		t.Fatalf("aprovar quem tem conta: %v %v", out, ak.members)
	}

	// Diz ter conta, mas o e-mail não é de nenhuma: avisado no envio, e a
	// aprovação para sem criar nada.
	if rec := signupExisting("ninguem" + suffix + "@exemplo.com"); rec.Code != http.StatusCreated {
		t.Fatalf("pedido sem conta: %d %s", rec.Code, rec.Body.String())
	}
	text, data = lastSent()
	if !strings.Contains(text, "nenhuma conta do Authentik usa este e-mail") {
		t.Fatalf("mensagem sem conta: %s", text)
	}
	usersBefore = len(ak.users)
	idE := idFrom(data[0])
	if _, out := decide(idE, "approve", "segredo"); out["ok"] != false || !strings.Contains(out["message"].(string), "nenhuma conta") || len(ak.users) != usersBefore {
		t.Fatalf("aprovar sem conta: %v", out)
	}
	last = tg.edited[len(tg.edited)-1]
	if last["reply_markup"] == nil || !strings.Contains(last["text"].(string), "Não aprovado") {
		t.Fatalf("mensagem de falha: %v", last)
	}
	if req, _ := db.Signups.Get(t.Context(), idE); req.Status != store.SignupPending {
		t.Fatalf("pedido E deveria voltar a pendente: %s", req.Status)
	}
	if _, out := decide(idE, "reject", "segredo"); out["ok"] != true {
		t.Fatalf("reprovar depois da falha: %v", out)
	}

	// E-mail em duas contas: não dá para escolher, a aprovação para.
	ak.users = append(ak.users,
		authentik.User{PK: 96, Username: "dup1" + suffix, Email: "dup" + suffix + "@exemplo.com", IsActive: true, Type: "internal"},
		authentik.User{PK: 97, Username: "dup2" + suffix, Email: "dup" + suffix + "@exemplo.com", IsActive: true, Type: "internal"})
	if rec := signupExisting("dup" + suffix + "@exemplo.com"); rec.Code != http.StatusCreated {
		t.Fatalf("pedido com e-mail duplicado: %d %s", rec.Code, rec.Body.String())
	}
	_, data = lastSent()
	if _, out := decide(idFrom(data[0]), "approve", "segredo"); out["ok"] != false || !strings.Contains(out["message"].(string), "2 contas") || ak.members[96] || ak.members[97] {
		t.Fatalf("aprovar com e-mail em duas contas: %v", out)
	}

	// Telegram fora do ar: o pedido fica gravado sem mensagem, para o
	// notificador reenviar.
	tg.fail = true
	ip = "b-" + suffix
	if rec := signup("ana"+suffix, "ana"+suffix+"@exemplo.com", ""); rec.Code != http.StatusCreated {
		t.Fatalf("cadastro com telegram fora: %d", rec.Code)
	}
	unsent, err := db.Signups.ListUnsent(t.Context(), time.Now().Add(time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range unsent {
		found = found || u.Username == "ana"+suffix
	}
	if !found {
		t.Fatal("pedido sem mensagem não aparece para o notificador")
	}

	// Disponibilidade, para o formulário avisar enquanto a pessoa digita.
	available := func(username string) (bool, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/signup-requests/username-available?username="+username, nil)
		req.Header.Set("X-Forwarded-For", "e-"+suffix)
		rec := httptest.NewRecorder()
		public.ServeHTTP(rec, req)
		var out struct {
			Available bool   `json:"available"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("disponibilidade de %s: %d %s", username, rec.Code, rec.Body.String())
		}
		return out.Available, out.Message
	}
	for username, want := range map[string]string{
		"livre" + suffix:                       "",
		"Livre" + suffix:                       "",
		"ad-min":                               "reservado",
		"x":                                    "de 3 a 30",
		"outro-projeto" + suffix:               "em uso",
		"ana" + suffix:                         "em análise",
		strings.ToUpper(userA[:1]) + userA[1:]: "aprovada",
	} {
		ok, message := available(username)
		if ok != (want == "") || !strings.Contains(message, want) {
			t.Errorf("disponibilidade de %s: %v %q, esperava %q", username, ok, message, want)
		}
	}
}
