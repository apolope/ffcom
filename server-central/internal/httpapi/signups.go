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
// Reprovado, nada é enviado à pessoa. Ver docs/architecture.md, "Decisão:
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
	n.Username = strings.ToLower(strings.TrimSpace(b.Username))
	if !signupUsernamePattern.MatchString(n.Username) {
		return n, "o nome de usuário precisa ter de 3 a 30 caracteres: letras minúsculas sem acento, números, ponto, hífen ou sublinhado, começando por letra ou número"
	}
	if n.Email, ok = validEmail(b.Email); !ok {
		return n, "e-mail inválido"
	}
	if n.Nickname, ok = cleanLine(b.Nickname, 2, 32); !ok {
		return n, "o apelido precisa ter entre 2 e 32 caracteres"
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
		// falharia. O e-mail só é conferido na aprovação, para o formulário
		// não servir de consulta de quem tem conta. Authentik fora do ar não
		// bloqueia o pedido: a aprovação confere de novo.
		if u, err := cfg.Authentik.FindUser(ctx, "username", n.Username); err != nil {
			log.Printf("cadastro: conferir usuário no authentik: %v", err)
		} else if u != nil {
			http.Error(w, "esse nome de usuário já está em uso; escolha outro", http.StatusConflict)
			return
		}

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
	b.WriteString("<b>FFCom · pedido de cadastro</b>\n\n")
	fmt.Fprintf(&b, "<b>Nome:</b> %s\n", e(req.FullName))
	fmt.Fprintf(&b, "<b>Usuário:</b> <code>%s</code>\n", e(req.Username))
	fmt.Fprintf(&b, "<b>E-mail:</b> %s\n", e(req.Email))
	fmt.Fprintf(&b, "<b>Apelido:</b> %s\n", e(req.Nickname))
	fmt.Fprintf(&b, "<b>Motivo:</b>\n<i>%s</i>\n\n", e(req.Reason))
	fmt.Fprintf(&b, "Recebido em %s", req.CreatedAt.In(brasilia).Format("02/01/2006 15:04"))
	return b.String()
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
	if req.Status == store.SignupApproved {
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

	user, err := s.createUser(ctx, req)
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

	var warning *string
	if err := s.cfg.Authentik.SendRecoveryEmail(ctx, user.PK, s.cfg.RecoveryEmailStage, s.cfg.RecoveryTokenDuration); err != nil {
		log.Printf("cadastro: e-mail de senha do pedido %s: %v", id, err)
		w := "O e-mail de definir senha não saiu; envie pela UI do Authentik (usuário > Enviar link de recuperação)."
		warning = &w
	}
	done, err := s.signups.FinishApproval(ctx, id, user.PK, warning)
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

// createUser cria o usuário do pedido, ou reaproveita o que uma aprovação
// anterior do mesmo pedido já criou (o processo pode ter caído entre criar
// e concluir).
func (s *signupService) createUser(ctx context.Context, req store.SignupRequest) (authentik.User, error) {
	ours := func(u *authentik.User) bool {
		return u != nil && u.Attributes["ffcom_signup_request"] == req.ID
	}
	byUsername, err := s.cfg.Authentik.FindUser(ctx, "username", req.Username)
	if err != nil {
		return authentik.User{}, err
	}
	if ours(byUsername) {
		return *byUsername, nil
	}
	if byUsername != nil {
		return authentik.User{}, fmt.Errorf("%w: nome de usuário %s já existe no Authentik", authentik.ErrUserExists, req.Username)
	}
	byEmail, err := s.cfg.Authentik.FindUser(ctx, "email", req.Email)
	if err != nil {
		return authentik.User{}, err
	}
	if byEmail != nil {
		return authentik.User{}, fmt.Errorf("%w: o e-mail já é do usuário %s no Authentik", authentik.ErrUserExists, byEmail.Username)
	}
	return s.cfg.Authentik.CreateUser(ctx, authentik.NewUser{
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
	if errors.Is(err, authentik.ErrUserExists) {
		msg := strings.TrimPrefix(err.Error(), authentik.ErrUserExists.Error()+": ")
		return truncateRunes(msg, 150)
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
