package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"a3sitsolutions.com/ffcom/server-central/internal/authentik"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
	"a3sitsolutions.com/ffcom/server-central/internal/telegram"
)

// Pedidos de cadastro feitos pela home page. A pessoa preenche o
// formulário (rota pública, sem login, porque ela ainda não tem conta), o
// pedido vai para o tópico do FFCom no grupo "Rede" do Telegram com botões
// de aprovar e reprovar, e o clique volta pelo a3s-network-monitor (dono do
// webhook do bot) até o listener interno. Aprovado, o usuário é criado no
// Authentik, entra no grupo ffcom-users e recebe o e-mail de definir senha.
// Reprovado, nada é enviado à pessoa. Quem já tem conta no Authentik (a
// instância é compartilhada com outros projetos) marca isso no formulário
// e não informa nome de usuário nem apelido: a aprovação acha a conta pelo
// e-mail e só a põe no grupo. Ver docs/architecture.md, "Decisão:
// cadastro com aprovação pelo Telegram".

// SignupConfig liga os pedidos de cadastro. Sem Telegram ou Authentik a
// rota pública responde 503.
type SignupConfig struct {
	Telegram  *telegram.Client
	Authentik *authentik.Client
	// UsersGroup é o grupo que dá acesso ao FFCom (ffcom-users).
	UsersGroup string
	// RecoveryEmailStage é o uuid do stage de e-mail do fluxo de
	// recuperação do Authentik, usado para mandar o link de definir senha.
	RecoveryEmailStage string
	// RecoveryTokenDuration é a validade do link (formato do Authentik,
	// ex. "days=3").
	RecoveryTokenDuration string
	// DecisionSecret é conferido no header X-FFCom-Secret das decisões que
	// o monitor repassa.
	DecisionSecret string
	// MaxPerIPPerDay e MaxPerHour seguram flood no formulário público, que
	// não tem login: por IP em 24 h e no total por hora (o grupo aceita ~20
	// mensagens por minuto e é dividido com o monitor).
	MaxPerIPPerDay int
	MaxPerHour     int
}

func (c SignupConfig) enabled() bool {
	return c.Telegram != nil && c.Authentik != nil
}

// Prefixos do callback_data dos botões. O monitor despacha por prefixo e
// repassa ao FFCom tudo que começa com signupCallbackPrefix.
const (
	signupCallbackPrefix = "ffcom-signup:"
	signupApproveData    = signupCallbackPrefix + "approve:"
	signupRejectData     = signupCallbackPrefix + "reject:"
)

var (
	signupUsernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,29}$`)
	uuidPattern           = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// reservedUsernames são nomes que ninguém pede pelo formulário, por
// parecerem da equipe, do sistema ou de outro serviço da infra. A
// comparação ignora ponto, hífen e sublinhado ("ad.min" também cai). O
// site/cadastro.js tem uma cópia só para avisar antes do envio; vale a
// daqui.
var reservedUsernames = map[string]bool{
	"a3s": true, "a3sitsolutions": true, "abuse": true, "admin": true, "administrador": true,
	"administrator": true, "ajuda": true, "akadmin": true, "api": true, "authentik": true,
	"bot": true, "contato": true, "dono": true, "equipe": true, "ffcom": true, "help": true,
	"info": true, "mod": true, "moderacao": true, "moderador": true, "moderator": true,
	"noreply": true, "null": true, "oficial": true, "official": true, "owner": true,
	"postmaster": true, "root": true, "security": true, "seguranca": true, "sistema": true,
	"staff": true, "suporte": true, "support": true, "system": true, "telegram": true,
	"undefined": true, "webmaster": true, "www": true,
}

// usernameProblem devolve por que username (já em minúsculas) não pode ser
// pedido, ou "" se o formato é aceito. Não confere se está em uso.
func usernameProblem(username string) string {
	if !signupUsernamePattern.MatchString(username) {
		return "o nome de usuário precisa ter de 3 a 30 caracteres: letras minúsculas sem acento, números, ponto, hífen ou sublinhado, começando por letra ou número"
	}
	bare := strings.NewReplacer(".", "", "-", "", "_", "").Replace(username)
	if reservedUsernames[bare] {
		return "esse nome de usuário é reservado; escolha outro"
	}
	return ""
}

// cleanLine tira espaços das pontas, junta espaços repetidos e recusa
// caracteres de controle (quebra de linha inclusive), devolvendo o texto e
// se ele tem entre min e max caracteres.
func cleanLine(raw string, min, max int) (string, bool) {
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	text := strings.Join(strings.Fields(raw), " ")
	n := utf8.RuneCountInString(text)
	return text, n >= min && n <= max
}

// cleanText é cleanLine para texto de várias linhas (o motivo): aceita
// quebras de linha, limitadas a linhas não vazias.
func cleanText(raw string, min, max int) (string, bool) {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		clean, _ := cleanLine(line, 0, max)
		if clean == "" {
			if strings.TrimSpace(line) != "" {
				return "", false
			}
			continue
		}
		lines = append(lines, clean)
	}
	text := strings.Join(lines, "\n")
	n := utf8.RuneCountInString(text)
	return text, n >= min && n <= max
}

// validEmail normaliza e confere o e-mail: um endereço só, sem nome, com
// domínio que tenha ponto.
func validEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) < 3 || len(email) > 254 {
		return "", false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" {
		return "", false
	}
	at := strings.LastIndex(email, "@")
	domain := email[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", false
	}
	return email, true
}

type signupBody struct {
	FullName string `json:"fullName"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
	Reason   string `json:"reason"`
	// ExistingAccount é a caixa "já tenho conta": nome de usuário e apelido
	// são ignorados, porque a conta já tem os dela.
	ExistingAccount bool `json:"existingAccount"`
	// Website é um campo escondido no formulário: gente não vê e não
	// preenche, robô de formulário preenche.
	Website string `json:"website"`
}

// validate devolve o pedido normalizado ou a mensagem do primeiro campo
// inválido.
func (b signupBody) validate() (store.NewSignup, string) {
	var n store.NewSignup
	var ok bool
	if n.FullName, ok = cleanLine(b.FullName, 2, 80); !ok {
		return n, "o nome precisa ter entre 2 e 80 caracteres"
	}
	n.ExistingAccount = b.ExistingAccount
	if !n.ExistingAccount {
		n.Username = strings.ToLower(strings.TrimSpace(b.Username))
		if problem := usernameProblem(n.Username); problem != "" {
			return n, problem
		}
	}
	if n.Email, ok = validEmail(b.Email); !ok {
		return n, "e-mail inválido"
	}
	if !n.ExistingAccount {
		if n.Nickname, ok = cleanLine(b.Nickname, 2, 32); !ok {
			return n, "o apelido precisa ter entre 2 e 32 caracteres"
		}
	}
	if n.Reason, ok = cleanText(b.Reason, 10, 500); !ok {
		return n, "conte em 10 a 500 caracteres por que quer entrar no FFCom"
	}
	return n, ""
}

type signupService struct {
	cfg     SignupConfig
	signups *store.SignupStore
	now     func() time.Time
}

func newSignupService(db *store.Store, cfg SignupConfig) *signupService {
	return &signupService{cfg: cfg, signups: db.Signups, now: time.Now}
}

// POST /api/signup-requests — público. Grava o pedido e manda ao Telegram.
func handleCreateSignup(db *store.Store, cfg SignupConfig) http.Handler {
	s := newSignupService(db, cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.enabled() {
			http.Error(w, "cadastro indisponível no momento", http.StatusServiceUnavailable)
			return
		}
		var body signupBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		if body.Website != "" {
			// Robô: responde como se tivesse dado certo, sem gravar nada.
			writeJSON(w, http.StatusCreated, map[string]string{"status": store.SignupPending})
			return
		}
		n, problem := body.validate()
		if problem != "" {
			http.Error(w, problem, http.StatusBadRequest)
			return
		}
		n.ClientIP = clientIP(r)

		ctx := r.Context()
		now := s.now()
		if count, err := s.signups.CountSince(ctx, n.ClientIP, now.Add(-24*time.Hour)); err != nil {
			log.Printf("cadastro: %v", err)
		} else if count >= cfg.MaxPerIPPerDay {
			http.Error(w, "muitos pedidos saíram desta rede hoje; tente de novo amanhã", http.StatusTooManyRequests)
			return
		}
		if count, err := s.signups.CountSince(ctx, "", now.Add(-time.Hour)); err != nil {
			log.Printf("cadastro: %v", err)
		} else if count >= cfg.MaxPerHour {
			http.Error(w, "muitos pedidos agora; tente de novo daqui a uma hora", http.StatusTooManyRequests)
			return
		}

		if existing, err := s.signups.OpenOrApprovedFor(ctx, n.Email, n.Username); err == nil {
			http.Error(w, conflictMessage(existing, n), http.StatusConflict)
			return
		} else if !errors.Is(err, store.ErrNotFound) {
			log.Printf("cadastro: %v", err)
			http.Error(w, "erro ao conferir pedidos anteriores", http.StatusInternalServerError)
			return
		}
		// O nome de usuário é público nos apps da instância, então conferir
		// aqui não expõe nada e poupa a pessoa de esperar uma aprovação que
		// falharia. Authentik fora do ar não bloqueia o pedido: a aprovação
		// confere de novo.
		if !n.ExistingAccount {
			if u, err := cfg.Authentik.FindUsername(ctx, n.Username); err != nil {
				log.Printf("cadastro: conferir usuário no authentik: %v", err)
			} else if u != nil {
				http.Error(w, "esse nome de usuário já está em uso; escolha outro", http.StatusConflict)
				return
			}
		}
		// O e-mail também é conferido, mas o resultado só vai para a
		// mensagem de quem aprova: a resposta é a mesma havendo conta ou
		// não, para o formulário não servir de consulta de quem tem conta.
		n.EmailAccounts = s.emailAccounts(ctx, n.Email)

		req, err := s.signups.Create(ctx, n)
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "já existe um pedido em análise com este e-mail ou nome de usuário", http.StatusConflict)
			return
		}
		if err != nil {
			log.Printf("cadastro: %v", err)
			http.Error(w, "erro ao gravar o pedido", http.StatusInternalServerError)
			return
		}
		// Falha no Telegram não perde o pedido: o notificador tenta de novo.
		s.notify(ctx, req)
		writeJSON(w, http.StatusCreated, map[string]string{"status": store.SignupPending})
	})
}

// emailAccounts lista, para a mensagem do Telegram, as contas do Authentik
// com o e-mail email ("" se nenhuma, nil se não deu para conferir).
func (s *signupService) emailAccounts(ctx context.Context, email string) *string {
	users, err := s.cfg.Authentik.FindByEmail(ctx, email)
	if err != nil {
		log.Printf("cadastro: conferir e-mail no authentik: %v", err)
		return nil
	}
	names := make([]string, 0, len(users))
	for _, u := range users {
		names = append(names, u.Username)
	}
	joined := strings.Join(names, ", ")
	return &joined
}

// handleUsernameAvailable responde se um nome de usuário pode ser pedido,
// para o formulário avisar enquanto a pessoa digita:
//
//	GET /api/signup-requests/username-available?username=maria
//	{"available": false, "message": "esse nome de usuário já está em uso; escolha outro"}
//
// Público como o envio, e não revela nada que o envio não revele (o nome
// de usuário é público nos apps da instância). Além do limite geral por
// IP, tem um próprio, para a rota não virar varredura de nomes. O envio
// confere tudo de novo, então esta rota é só conforto.
func handleUsernameAvailable(db *store.Store, cfg SignupConfig) http.Handler {
	s := newSignupService(db, cfg)
	limiter := newRateLimiter(30, 10)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cfg.enabled() {
			http.Error(w, "cadastro indisponível no momento", http.StatusServiceUnavailable)
			return
		}
		if !limiter.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "muitas consultas; espere um pouco", http.StatusTooManyRequests)
			return
		}
		username := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("username")))
		answer := func(available bool, message string) {
			writeJSON(w, http.StatusOK, map[string]any{"available": available, "message": message})
		}
		if problem := usernameProblem(username); problem != "" {
			answer(false, problem)
			return
		}
		ctx := r.Context()
		if existing, err := s.signups.OpenOrApprovedFor(ctx, "", username); err == nil {
			answer(false, conflictMessage(existing, store.NewSignup{Username: username}))
			return
		} else if !errors.Is(err, store.ErrNotFound) {
			log.Printf("cadastro: %v", err)
			http.Error(w, "erro ao conferir o nome", http.StatusInternalServerError)
			return
		}
		u, err := cfg.Authentik.FindUsername(ctx, username)
		if err != nil {
			log.Printf("cadastro: conferir usuário no authentik: %v", err)
			http.Error(w, "não deu para conferir agora", http.StatusServiceUnavailable)
			return
		}
		if u != nil {
			answer(false, "esse nome de usuário já está em uso; escolha outro")
			return
		}
		answer(true, "")
	})
}

// conflictMessage explica por que um pedido novo esbarra em existing.
func conflictMessage(existing store.SignupRequest, n store.NewSignup) string {
	field := "este e-mail"
	if existing.Email != n.Email {
		field = "este nome de usuário"
	}
	if existing.Status == store.SignupApproved {
		return "já existe uma conta aprovada com " + field + "; confira o e-mail de definir senha ou entre no app"
	}
	return "já existe um pedido em análise com " + field
}

// notify manda o pedido ao Telegram e guarda o id da mensagem.
func (s *signupService) notify(ctx context.Context, req store.SignupRequest) {
	messageID, err := s.cfg.Telegram.SendMessage(ctx, signupMessage(req), signupButtons(req.ID))
	if err != nil {
		log.Printf("cadastro: enviar pedido %s ao telegram: %v", req.ID, err)
		return
	}
	if err := s.signups.SetTelegramMessage(ctx, req.ID, messageID); err != nil {
		log.Printf("cadastro: %v", err)
	}
}

func signupButtons(id string) []telegram.Button {
	return []telegram.Button{
		{Text: "✅ Aprovar", Data: signupApproveData + id},
		{Text: "❌ Reprovar", Data: signupRejectData + id},
	}
}

// signupMessage é o texto do pedido no Telegram, em HTML.
func signupMessage(req store.SignupRequest) string {
	e := telegram.Escape
	var b strings.Builder
	if req.ExistingAccount {
		b.WriteString("<b>FFCom · pedido de acesso (já tem conta)</b>\n\n")
	} else {
		b.WriteString("<b>FFCom · pedido de cadastro</b>\n\n")
	}
	fmt.Fprintf(&b, "<b>Nome:</b> %s\n", e(req.FullName))
	if !req.ExistingAccount {
		fmt.Fprintf(&b, "<b>Usuário:</b> <code>%s</code>\n", e(req.Username))
	}
	fmt.Fprintf(&b, "<b>E-mail:</b> %s\n", e(req.Email))
	if !req.ExistingAccount {
		fmt.Fprintf(&b, "<b>Apelido:</b> %s\n", e(req.Nickname))
	}
	fmt.Fprintf(&b, "<b>Motivo:</b>\n<i>%s</i>\n\n", e(req.Reason))
	if note := emailAccountsNote(req); note != "" {
		b.WriteString(note + "\n\n")
	}
	fmt.Fprintf(&b, "Recebido em %s", req.CreatedAt.In(brasilia).Format("02/01/2006 15:04"))
	return b.String()
}

// emailAccountsNote conta a quem aprova o que a aprovação vai fazer com as
// contas do Authentik que já usam o e-mail do pedido, conferidas no envio.
// Importa sobretudo quando a pessoa não marcou "já tenho conta": aprovar
// põe no grupo a conta de quem é dono do e-mail, sem criar a pedida.
func emailAccountsNote(req store.SignupRequest) string {
	if req.EmailAccounts == nil {
		return ""
	}
	var accounts []string
	if *req.EmailAccounts != "" {
		accounts = strings.Split(*req.EmailAccounts, ", ")
	}
	e := telegram.Escape
	switch {
	case len(accounts) == 0 && req.ExistingAccount:
		return "⚠️ Diz já ter conta, mas nenhuma conta do Authentik usa este e-mail: aprovar vai falhar."
	case len(accounts) == 0:
		return ""
	case len(accounts) > 1:
		return fmt.Sprintf("⚠️ Este e-mail está em %d contas do Authentik (<code>%s</code>): aprovar vai parar; ponha a certa em ffcom-users pela UI.", len(accounts), e(*req.EmailAccounts))
	case req.ExistingAccount:
		return fmt.Sprintf("🔗 Conta existente: <code>%s</code>. Aprovar só adiciona ao grupo ffcom-users.", e(accounts[0]))
	}
	return fmt.Sprintf("🔗 Este e-mail já é da conta <code>%s</code> no Authentik. Aprovar adiciona essa conta ao grupo ffcom-users, sem criar o usuário pedido.", e(accounts[0]))
}

// decidedMessage é o pedido com a decisão no fim, sem botões.
func decidedMessage(req store.SignupRequest, at time.Time) string {
	by := "alguém"
	if req.DecidedBy != nil && *req.DecidedBy != "" {
		by = *req.DecidedBy
	}
	when := at.In(brasilia).Format("02/01 15:04")
	var b strings.Builder
	b.WriteString(signupMessage(req))
	b.WriteString("\n\n")
	if req.Status == store.SignupApproved && req.LinkedExisting {
		account := ""
		if req.AuthentikUsername != nil {
			account = " <code>" + telegram.Escape(*req.AuthentikUsername) + "</code>"
		}
		fmt.Fprintf(&b, "✅ <b>Aprovado</b> por %s em %s. Conta existente%s adicionada ao grupo ffcom-users; ela já tem senha, então nenhum e-mail foi enviado.", telegram.Escape(by), when, account)
	} else if req.Status == store.SignupApproved {
		fmt.Fprintf(&b, "✅ <b>Aprovado</b> por %s em %s. Conta criada no Authentik, no grupo ffcom-users.", telegram.Escape(by), when)
		if req.Failure != nil {
			fmt.Fprintf(&b, "\n⚠️ %s", telegram.Escape(*req.Failure))
		} else {
			b.WriteString(" E-mail de definir senha enviado.")
		}
	} else {
		fmt.Fprintf(&b, "❌ <b>Reprovado</b> por %s em %s.", telegram.Escape(by), when)
	}
	return b.String()
}

// updateMessage troca a mensagem do pedido pela versão com a decisão.
func (s *signupService) updateMessage(ctx context.Context, req store.SignupRequest) {
	if req.TelegramMessageID == nil {
		return
	}
	if err := s.cfg.Telegram.EditMessage(ctx, *req.TelegramMessageID, decidedMessage(req, s.now()), nil); err != nil {
		log.Printf("cadastro: editar mensagem do pedido %s: %v", req.ID, err)
	}
}

// showFailure mostra na mensagem por que a aprovação falhou, mantendo os
// botões para tentar de novo ou reprovar. O aviso do clique pode não
// chegar a quem clicou (o monitor responde o clique antes de a aprovação
// terminar), então a mensagem é onde o motivo fica.
func (s *signupService) showFailure(ctx context.Context, req store.SignupRequest, reason string) {
	if req.TelegramMessageID == nil {
		return
	}
	text := signupMessage(req) + "\n\n⚠️ Não aprovado: " + telegram.Escape(reason) + ". Clique de novo depois de resolver, ou reprove."
	if err := s.cfg.Telegram.EditMessage(ctx, *req.TelegramMessageID, text, signupButtons(req.ID)); err != nil {
		log.Printf("cadastro: editar mensagem do pedido %s: %v", req.ID, err)
	}
}

// describeDecided conta o estado de um pedido que não pôde ser decidido
// agora, para o aviso do clique no Telegram.
func describeDecided(req store.SignupRequest) string {
	by := ""
	if req.DecidedBy != nil && *req.DecidedBy != "" {
		by = " por " + *req.DecidedBy
	}
	switch req.Status {
	case store.SignupApproved:
		return "Pedido já aprovado" + by
	case store.SignupRejected:
		return "Pedido já reprovado" + by
	case store.SignupApproving:
		return "Aprovação em andamento" + by
	}
	return "Pedido em outro estado: " + req.Status
}

// approve cria a conta do pedido id no Authentik e devolve se aprovou e o
// aviso para quem clicou.
func (s *signupService) approve(ctx context.Context, id, by string) (bool, string) {
	req, err := s.signups.ClaimApproval(ctx, id, by)
	if errors.Is(err, store.ErrNotFound) {
		current, err := s.signups.Get(ctx, id)
		if err != nil {
			return false, "Pedido não encontrado"
		}
		return false, describeDecided(current)
	}
	if err != nil {
		log.Printf("cadastro: %v", err)
		return false, "Erro ao ler o pedido; tente de novo"
	}

	user, linked, err := s.resolveUser(ctx, req)
	if err == nil {
		err = s.addToGroup(ctx, user.PK)
	}
	if err != nil {
		log.Printf("cadastro: aprovar pedido %s: %v", id, err)
		reason := approvalFailure(err)
		if err := s.signups.ReleaseApproval(ctx, id, reason); err != nil {
			log.Printf("cadastro: %v", err)
		}
		s.showFailure(ctx, req, reason)
		return false, "Não aprovado: " + reason
	}

	if linked {
		// Conta que já existia: tem senha, então não há e-mail a mandar.
		done, err := s.signups.FinishApproval(ctx, id, user.PK, user.Username, true, nil)
		if err != nil {
			log.Printf("cadastro: concluir pedido %s: %v", id, err)
			return false, "Conta no grupo, mas o pedido não foi atualizado; confira os logs"
		}
		s.updateMessage(ctx, done)
		return true, "Aprovado; conta existente " + user.Username + " adicionada ao grupo"
	}

	var warning *string
	if err := s.cfg.Authentik.SendRecoveryEmail(ctx, user.PK, s.cfg.RecoveryEmailStage, s.cfg.RecoveryTokenDuration); err != nil {
		log.Printf("cadastro: e-mail de senha do pedido %s: %v", id, err)
		w := "O e-mail de definir senha não saiu; envie pela UI do Authentik (usuário > Enviar link de recuperação)."
		warning = &w
	}
	done, err := s.signups.FinishApproval(ctx, id, user.PK, user.Username, false, warning)
	if err != nil {
		log.Printf("cadastro: concluir pedido %s: %v", id, err)
		return false, "Conta criada, mas o pedido não foi atualizado; confira os logs"
	}
	s.updateMessage(ctx, done)
	if warning != nil {
		return true, "Aprovado, mas o e-mail de senha falhou"
	}
	return true, "Aprovado; e-mail de senha enviado"
}

// errApprovalBlocked marca as falhas de aprovação que dependem de alguém
// resolver no Authentik (conta desativada, e-mail em várias contas), cujo
// motivo vai inteiro para a mensagem.
var errApprovalBlocked = errors.New("aprovação bloqueada")

// resolveUser acha a conta do Authentik que o pedido deve pôr no grupo e
// diz se ela já existia (linked). A ordem importa:
//
//  1. Uma conta com o atributo deste pedido é reaproveitada: uma aprovação
//     anterior a criou e o processo caiu antes de concluir.
//  2. Havendo uma conta com o e-mail do pedido, é ela, marcada a caixa "já
//     tenho conta" ou não; criar outra com o mesmo e-mail duplicaria a
//     pessoa na instância. Quem aprova vê na mensagem, desde o envio, que
//     é isso que vai acontecer (ver emailAccountsNote).
//  3. Mais de uma conta com o e-mail: não há como escolher com segurança.
//  4. Nenhuma: cria a pedida, se a pessoa não disse que já tinha conta.
func (s *signupService) resolveUser(ctx context.Context, req store.SignupRequest) (authentik.User, bool, error) {
	ours := func(u authentik.User) bool {
		return u.Attributes["ffcom_signup_request"] == req.ID
	}
	byEmail, err := s.cfg.Authentik.FindByEmail(ctx, req.Email)
	if err != nil {
		return authentik.User{}, false, err
	}
	for _, u := range byEmail {
		if ours(u) {
			return u, false, nil
		}
	}
	switch {
	case len(byEmail) > 1:
		names := make([]string, 0, len(byEmail))
		for _, u := range byEmail {
			names = append(names, u.Username)
		}
		return authentik.User{}, false, fmt.Errorf("%w: o e-mail está em %d contas do Authentik (%s); ponha a certa em ffcom-users pela UI e reprove", errApprovalBlocked, len(byEmail), strings.Join(names, ", "))
	case len(byEmail) == 1:
		u := byEmail[0]
		if u.IsServiceAccount() {
			return authentik.User{}, false, fmt.Errorf("%w: o e-mail é da conta de serviço %s", errApprovalBlocked, u.Username)
		}
		if !u.IsActive {
			return authentik.User{}, false, fmt.Errorf("%w: a conta %s está desativada no Authentik", errApprovalBlocked, u.Username)
		}
		return u, true, nil
	case req.ExistingAccount:
		return authentik.User{}, false, fmt.Errorf("%w: nenhuma conta do Authentik usa este e-mail", errApprovalBlocked)
	}

	byUsername, err := s.cfg.Authentik.FindUsername(ctx, req.Username)
	if err != nil {
		return authentik.User{}, false, err
	}
	if byUsername != nil && ours(*byUsername) {
		return *byUsername, false, nil
	}
	if byUsername != nil {
		return authentik.User{}, false, fmt.Errorf("%w: nome de usuário %s já existe no Authentik", authentik.ErrUserExists, byUsername.Username)
	}
	user, err := s.cfg.Authentik.CreateUser(ctx, authentik.NewUser{
		Username: req.Username,
		// O nome do Authentik é o que o FFCom exibe em todo lugar (servidores,
		// amigos, ideias), então vai o apelido; o nome completo fica nos
		// atributos e no pedido.
		Name:  req.Nickname,
		Email: req.Email,
		Attributes: map[string]any{
			"ffcom_signup_request": req.ID,
			"ffcom_full_name":      req.FullName,
		},
	})
	return user, false, err
}

func (s *signupService) addToGroup(ctx context.Context, userPK int64) error {
	group, err := s.cfg.Authentik.GroupUUID(ctx, s.cfg.UsersGroup)
	if err != nil {
		return err
	}
	return s.cfg.Authentik.AddToGroup(ctx, group, userPK)
}

// approvalFailure resume o erro da aprovação para o Telegram (o aviso do
// clique aceita até 200 caracteres).
func approvalFailure(err error) string {
	for _, known := range []error{authentik.ErrUserExists, errApprovalBlocked} {
		if errors.Is(err, known) {
			msg := strings.TrimPrefix(err.Error(), known.Error()+": ")
			return truncateRunes(msg, 150)
		}
	}
	return "falha ao falar com o Authentik; tente de novo em instantes"
}

// reject reprova o pedido id e devolve se reprovou e o aviso para quem
// clicou.
func (s *signupService) reject(ctx context.Context, id, by string) (bool, string) {
	req, err := s.signups.Reject(ctx, id, by)
	if errors.Is(err, store.ErrNotFound) {
		current, err := s.signups.Get(ctx, id)
		if err != nil {
			return false, "Pedido não encontrado"
		}
		return false, describeDecided(current)
	}
	if err != nil {
		log.Printf("cadastro: %v", err)
		return false, "Erro ao reprovar; tente de novo"
	}
	s.updateMessage(ctx, req)
	return true, "Reprovado"
}

// registerSignupDecision monta no listener interno a rota pela qual o
// a3s-network-monitor repassa o clique nos botões:
//
//	POST /internal/signup-requests/{id}/decision
//	X-FFCom-Secret: <SIGNUP_DECISION_SECRET>
//	{"decision": "approve" | "reject", "by": "@quem_clicou"}
//
// Responde 200 com {"ok": bool, "message": "..."} sempre que o pedido foi
// tratado, inclusive quando a aprovação falhou; message cabe no aviso do
// answerCallbackQuery. A mensagem do Telegram é editada aqui mesmo.
func registerSignupDecision(mux *http.ServeMux, db *store.Store, cfg SignupConfig) {
	s := newSignupService(db, cfg)
	mux.HandleFunc("POST /internal/signup-requests/{id}/decision", func(w http.ResponseWriter, r *http.Request) {
		secret := r.Header.Get("X-FFCom-Secret")
		if cfg.DecisionSecret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(cfg.DecisionSecret)) != 1 {
			http.Error(w, "segredo inválido", http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		if !uuidPattern.MatchString(id) {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "message": "Pedido não encontrado"})
			return
		}
		var body struct {
			Decision string `json:"decision"`
			By       string `json:"by"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		by, _ := cleanLine(body.By, 0, 64)
		if by == "" {
			by = "Telegram"
		}
		// O aviso do clique precisa sair antes de o Telegram desistir; as
		// chamadas ao Authentik levam segundos.
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		var ok bool
		var message string
		switch body.Decision {
		case "approve":
			ok, message = s.approve(ctx, id, by)
		case "reject":
			ok, message = s.reject(ctx, id, by)
		default:
			http.Error(w, "decision deve ser approve ou reject", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "message": message})
	})
}

// RunSignupNotifier reenvia ao Telegram, a cada interval, os pedidos cujo
// envio falhou na hora do cadastro (Telegram fora do ar ou limite de
// mensagens).
func RunSignupNotifier(ctx context.Context, db *store.Store, cfg SignupConfig, interval time.Duration) {
	s := newSignupService(db, cfg)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pending, err := s.signups.ListUnsent(ctx, s.now().Add(-2*time.Minute), 5)
			if err != nil {
				log.Printf("cadastro: %v", err)
				continue
			}
			for _, req := range pending {
				s.notify(ctx, req)
			}
		}
	}
}
