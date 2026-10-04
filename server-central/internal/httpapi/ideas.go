package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// Rotas das sugestões de melhoria exibidas na home page. Lista pública
// (qualquer visitante vê o ranking, com o primeiro nome de quem sugeriu);
// sugerir, usar a varinha e votar exigem login no Authentik (só o grupo
// ffcom-users consegue logar no FFCom); moderar exige o grupo
// cfg.AdminGroup. Ver docs/architecture.md, "Decisão: sugestões de melhoria
// com varinha do Claude".

// brasilia é o fuso do "por dia" das regras (uma ideia e N usos da varinha
// por dia). Horário fixo em vez de time.LoadLocation porque a imagem é
// alpine sem tzdata, e o Brasil não tem horário de verão desde 2019.
var brasilia = time.FixedZone("BRT", -3*60*60)

// today devolve o dia corrente em Brasília, como data (meia-noite UTC) para
// as colunas DATE.
func today() time.Time {
	now := time.Now().In(brasilia)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// versionPattern é o formato das versões no CHANGELOG.md, para a ideia
// implementada apontar para a entrada do histórico.
var versionPattern = regexp.MustCompile(`^(client|central|channel|channel-image|site) v\d+\.\d+\.\d+$`)

type ideaResponse struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Body               string    `json:"body"`
	Author             string    `json:"author"`
	Status             string    `json:"status"`
	Score              int       `json:"score"`
	Likes              int       `json:"likes"`
	Dislikes           int       `json:"dislikes"`
	MyVote             int       `json:"myVote"`
	Mine               bool      `json:"mine"`
	ImplementedVersion *string   `json:"implementedVersion,omitempty"`
	ReviewReason       *string   `json:"reviewReason,omitempty"`
	Feedback           *string   `json:"feedback,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
}

// toIdeaResponse monta a resposta de uma ideia. O motivo da moderação só
// aparece para o admin e para quem escreveu.
func toIdeaResponse(i store.Idea, viewerID string, isAdmin bool) ideaResponse {
	resp := ideaResponse{
		ID:                 i.ID,
		Title:              i.Title,
		Body:               i.Body,
		Author:             i.AuthorFirstName,
		Status:             i.Status,
		Score:              i.Score(),
		Likes:              i.Likes,
		Dislikes:           i.Dislikes,
		MyVote:             i.MyVote,
		Mine:               viewerID != "" && i.AccountID == viewerID,
		ImplementedVersion: i.ImplementedVersion,
		CreatedAt:          i.CreatedAt,
	}
	if isAdmin || resp.Mine {
		resp.ReviewReason = i.ReviewReason
	}
	if resp.Mine {
		resp.Feedback = i.Feedback
	}
	return resp
}

func toIdeaResponses(ideas []store.Idea, viewerID string, isAdmin bool) []ideaResponse {
	out := make([]ideaResponse, 0, len(ideas))
	for _, i := range ideas {
		out = append(out, toIdeaResponse(i, viewerID, isAdmin))
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// viewer devolve a conta de quem pede (vazio para visitante) e se é admin.
func viewer(r *http.Request, cfg IdeasConfig) (string, bool) {
	account, ok := auth.AccountFromContext(r.Context())
	if !ok {
		return "", false
	}
	return account.ID, auth.InGroupFromContext(r.Context(), cfg.AdminGroup)
}

// validIdeaText limpa o texto e confere o tamanho.
func validIdeaText(raw string) (string, bool) {
	text := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(text)
	return text, n >= ideaMinChars && n <= ideaMaxChars
}

// GET /api/ideas?view=ranking|implemented|review — público, exceto review
// (fila de moderação, só admin).
func handleListIdeas(ideas *store.IdeaStore, cfg IdeasConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		viewerID, isAdmin := viewer(r, cfg)
		var (
			list []store.Idea
			err  error
		)
		switch r.URL.Query().Get("view") {
		case "", "ranking":
			list, err = ideas.List(r.Context(), []string{store.IdeaOpen, store.IdeaPlanned}, viewerID, store.IdeaOrderScore, 100)
		case "implemented":
			list, err = ideas.List(r.Context(), []string{store.IdeaImplemented}, viewerID, store.IdeaOrderRecent, 50)
		case "review":
			if !isAdmin {
				apierr.Write(w, http.StatusForbidden, "ideas.admin_only_review", "só administradores veem a moderação")
				return
			}
			list, err = ideas.List(r.Context(), []string{store.IdeaReview, store.IdeaChecking}, viewerID, store.IdeaOrderOldest, 100)
		default:
			apierr.Write(w, http.StatusBadRequest, "ideas.view_invalid", "view inválida")
			return
		}
		if err != nil {
			log.Printf("ideias: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "ideas.list_failed", "erro ao listar ideias")
			return
		}
		writeJSON(w, http.StatusOK, toIdeaResponses(list, viewerID, isAdmin))
	})
}

type ideasMeResponse struct {
	AssistEnabled  bool          `json:"assistEnabled"`
	WandLimit      int           `json:"wandLimit"`
	WandLeft       int           `json:"wandLeft"`
	WandUsed       int           `json:"wandUsed"`
	WandPenalty    int           `json:"wandPenalty"`
	PendingAssist  *string       `json:"pendingAssist,omitempty"`
	SuggestedToday bool          `json:"suggestedToday"`
	TodayIdea      *ideaResponse `json:"todayIdea,omitempty"`
	// LastDiscarded: a última ideia de hoje descartada por não ser
	// sugestão, com a dica em feedback (só enquanto não houver ideia do dia).
	LastDiscarded *ideaResponse `json:"lastDiscarded,omitempty"`
	DiscardsLeft  int           `json:"discardsLeft"`
	IsAdmin       bool          `json:"isAdmin"`
}

func wandLeft(limit, used, penalty int) int {
	return max(0, limit-used-penalty)
}

// GET /api/ideas/me — saldo da varinha, se já sugeriu hoje e se é admin.
func handleIdeasMe(db *store.Store, cfg IdeasConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, _ := auth.AccountFromContext(r.Context())
		isAdmin := auth.InGroupFromContext(r.Context(), cfg.AdminGroup)
		day := today()

		used, penalty, err := db.Ideas.WandUsage(r.Context(), account.ID, day)
		if err != nil {
			log.Printf("ideias: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "ideas.wand_balance_failed", "erro ao ler o saldo")
			return
		}
		resp := ideasMeResponse{
			AssistEnabled: cfg.Relay != nil,
			WandLimit:     cfg.WandPerDay,
			WandLeft:      wandLeft(cfg.WandPerDay, used, penalty),
			WandUsed:      used,
			WandPenalty:   penalty,
			IsAdmin:       isAdmin,
		}
		if job, err := db.Ideas.PendingJob(r.Context(), account.ID, store.AssistImprove); err == nil {
			resp.PendingAssist = &job.ID
		}
		idea, err := db.Ideas.ForDay(r.Context(), account.ID, day)
		switch {
		case err == nil:
			resp.SuggestedToday = true
			ir := toIdeaResponse(idea, account.ID, isAdmin)
			resp.TodayIdea = &ir
		case !errors.Is(err, store.ErrNotFound):
			log.Printf("ideias: %v", err)
		}
		discarded, err := db.Ideas.CountDiscarded(r.Context(), account.ID, day)
		if err != nil {
			log.Printf("ideias: %v", err)
		}
		resp.DiscardsLeft = max(0, ideaMaxDiscardsPerDay-discarded)
		if !resp.SuggestedToday {
			if last, err := db.Ideas.LastDiscarded(r.Context(), account.ID, day); err == nil {
				lr := toIdeaResponse(last, account.ID, isAdmin)
				resp.LastDiscarded = &lr
			}
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

type assistResponse struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	WandLeft int             `json:"wandLeft"`
	Result   *assistResultJS `json:"result,omitempty"`
}

type assistResultJS struct {
	Offensive     bool           `json:"offensive"`
	NotSuggestion bool           `json:"notSuggestion"`
	Hint          string         `json:"hint,omitempty"`
	Title         string         `json:"title,omitempty"`
	Text          string         `json:"text,omitempty"`
	Similar       []ideaResponse `json:"similar"`
}

// POST /api/ideas/assist {text} — usa a varinha: gasta um uso e manda o
// texto ao relay. Responde 202 com o id do pedido, que o client consulta em
// GET /api/ideas/assist/{id}.
func handleCreateAssist(db *store.Store, cfg IdeasConfig) http.Handler {
	a := &ideaAssistant{cfg: cfg, ideas: db.Ideas}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, _ := auth.AccountFromContext(r.Context())
		if !a.enabled() {
			apierr.Write(w, http.StatusServiceUnavailable, "ideas.wand_disabled", "a varinha não está disponível nesta instância")
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		text, ok := validIdeaText(body.Text)
		if !ok {
			apierr.WriteParams(w, http.StatusBadRequest, "ideas.text_length", "o texto precisa ter entre 10 e 1000 caracteres", apierr.Params{"min": ideaMinChars, "max": ideaMaxChars})
			return
		}
		if job, err := db.Ideas.PendingJob(r.Context(), account.ID, store.AssistImprove); err == nil {
			apierr.WriteParams(w, http.StatusConflict, "ideas.wand_busy", "a varinha ainda está trabalhando no pedido anterior", apierr.Params{"id": job.ID})
			return
		}

		day := today()
		reserved, err := db.Ideas.ReserveWand(r.Context(), account.ID, day, cfg.WandPerDay)
		if err != nil {
			log.Printf("ideias: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "ideas.wand_reserve_failed", "erro ao reservar o uso da varinha")
			return
		}
		if !reserved {
			apierr.Write(w, http.StatusTooManyRequests, "ideas.wand_daily_limit", "você já usou a varinha todas as vezes de hoje")
			return
		}

		top, err := db.Ideas.List(r.Context(), []string{store.IdeaOpen, store.IdeaPlanned}, "", store.IdeaOrderScore, ideaPromptTop)
		if err != nil {
			log.Printf("ideias: %v", err)
			top = nil
		}
		candidates := improveCandidates{Candidates: make([]string, 0, len(top))}
		for _, idea := range top {
			candidates.Candidates = append(candidates.Candidates, idea.ID)
		}
		pending, _ := json.Marshal(candidates)
		jobID, err := db.Ideas.CreateJob(r.Context(), store.AssistImprove, account.ID, day, nil, pending)
		if err != nil {
			log.Printf("ideias: %v", err)
			_ = db.Ideas.RefundWand(r.Context(), account.ID, day)
			apierr.Write(w, http.StatusInternalServerError, "ideas.job_create_failed", "erro ao criar o pedido")
			return
		}
		if err := a.submit(r.Context(), jobID, improvePrompt(text, top)); err != nil {
			log.Printf("ideias: %v", err)
			a.failImprove(r.Context(), jobID, "relay indisponível")
			apierr.Write(w, http.StatusBadGateway, "ideas.wand_unavailable", "a varinha está indisponível agora; tente de novo em instantes")
			return
		}

		used, penalty, _ := db.Ideas.WandUsage(r.Context(), account.ID, day)
		writeJSON(w, http.StatusAccepted, assistResponse{ID: jobID, Status: store.AssistPending, WandLeft: wandLeft(cfg.WandPerDay, used, penalty)})
	})
}

// GET /api/ideas/assist/{id} — estado de um pedido da varinha.
func handleGetAssist(db *store.Store, cfg IdeasConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, _ := auth.AccountFromContext(r.Context())
		job, err := db.Ideas.GetJob(r.Context(), r.PathValue("id"))
		if err != nil || job.AccountID != account.ID || job.Kind != store.AssistImprove {
			apierr.Write(w, http.StatusNotFound, "ideas.job_not_found", "pedido não encontrado")
			return
		}
		// Saldo de hoje (um pedido de ontem que fechou depois da meia-noite
		// não mexe no saldo novo).
		used, penalty, _ := db.Ideas.WandUsage(r.Context(), account.ID, today())
		resp := assistResponse{ID: job.ID, Status: job.Status, WandLeft: wandLeft(cfg.WandPerDay, used, penalty)}
		if job.Status == store.AssistDone {
			var v assistVerdict
			if err := json.Unmarshal(job.Result, &v); err == nil {
				result := &assistResultJS{Offensive: v.Offensive, NotSuggestion: v.NotSuggestion, Hint: v.Hint, Title: v.Title, Text: v.Text, Similar: []ideaResponse{}}
				if len(v.Similar) > 0 {
					similar, err := db.Ideas.GetMany(r.Context(), v.Similar, []string{store.IdeaOpen, store.IdeaPlanned}, account.ID)
					if err == nil {
						result.Similar = toIdeaResponses(similar, account.ID, false)
					}
				}
				resp.Result = result
			}
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// POST /api/ideas {text} — envia a ideia do dia. Ela nasce em checking e só
// aparece no ranking depois da checagem final (ou da aprovação de um admin).
func handleCreateIdea(db *store.Store, cfg IdeasConfig) http.Handler {
	a := &ideaAssistant{cfg: cfg, ideas: db.Ideas}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, _ := auth.AccountFromContext(r.Context())
		isAdmin := auth.InGroupFromContext(r.Context(), cfg.AdminGroup)
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		text, ok := validIdeaText(body.Text)
		if !ok {
			apierr.WriteParams(w, http.StatusBadRequest, "ideas.text_length", "o texto precisa ter entre 10 e 1000 caracteres", apierr.Params{"min": ideaMinChars, "max": ideaMaxChars})
			return
		}

		day := today()
		discarded, err := db.Ideas.CountDiscarded(r.Context(), account.ID, day)
		if err != nil {
			log.Printf("ideias: %v", err)
		}
		if discarded >= ideaMaxDiscardsPerDay {
			apierr.Write(w, http.StatusTooManyRequests, "ideas.too_many_rejected", "hoje já foram vários textos que não eram sugestões; tente de novo amanhã")
			return
		}
		ideaID, err := db.Ideas.Create(r.Context(), account.ID, text, day)
		if errors.Is(err, store.ErrConflict) {
			apierr.Write(w, http.StatusConflict, "ideas.daily_limit", "você já enviou uma sugestão hoje; amanhã tem outra")
			return
		}
		if err != nil {
			log.Printf("ideias: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "ideas.save_failed", "erro ao gravar a sugestão")
			return
		}

		noCheck := reasonNoCheck
		if !a.enabled() {
			_ = db.Ideas.FinishCheck(r.Context(), ideaID, store.IdeaReview, "", nil, &noCheck)
		} else if jobID, err := db.Ideas.CreateJob(r.Context(), store.AssistCheck, account.ID, day, &ideaID, nil); err != nil {
			log.Printf("ideias: %v", err)
			_ = db.Ideas.FinishCheck(r.Context(), ideaID, store.IdeaReview, "", nil, &noCheck)
		} else if err := a.submit(r.Context(), jobID, checkPrompt(text)); err != nil {
			log.Printf("ideias: %v", err)
			a.failCheck(r.Context(), jobID, "relay indisponível")
		}

		idea, err := db.Ideas.Get(r.Context(), ideaID, account.ID)
		if err != nil {
			log.Printf("ideias: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "ideas.job_read_failed", "erro ao ler a sugestão")
			return
		}
		writeJSON(w, http.StatusAccepted, toIdeaResponse(idea, account.ID, isAdmin))
	})
}

// PUT /api/ideas/{id}/vote {value: 1|-1|0} — like, dislike ou tirar o voto.
func handleVoteIdea(db *store.Store, cfg IdeasConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, _ := auth.AccountFromContext(r.Context())
		isAdmin := auth.InGroupFromContext(r.Context(), cfg.AdminGroup)
		var body struct {
			Value int `json:"value"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Value < -1 || body.Value > 1 {
			apierr.Write(w, http.StatusBadRequest, "ideas.vote_invalid", "voto inválido")
			return
		}
		idea, err := db.Ideas.Get(r.Context(), r.PathValue("id"), account.ID)
		if err != nil {
			apierr.Write(w, http.StatusNotFound, "ideas.not_found", "ideia não encontrada")
			return
		}
		if idea.Status != store.IdeaOpen && idea.Status != store.IdeaPlanned {
			apierr.Write(w, http.StatusConflict, "ideas.not_open_for_votes", "esta ideia não está aberta a votos")
			return
		}
		if idea.AccountID == account.ID {
			apierr.Write(w, http.StatusForbidden, "ideas.own_vote", "não dá para votar na própria ideia")
			return
		}
		if err := db.Ideas.Vote(r.Context(), idea.ID, account.ID, body.Value); err != nil {
			log.Printf("ideias: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "ideas.vote_failed", "erro ao votar")
			return
		}
		idea, err = db.Ideas.Get(r.Context(), idea.ID, account.ID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "ideas.read_failed", "erro ao ler a ideia")
			return
		}
		writeJSON(w, http.StatusOK, toIdeaResponse(idea, account.ID, isAdmin))
	})
}

// DELETE /api/ideas/{id} — ação do admin: apaga a ideia de vez, com os
// votos. Diferente de recusar, devolve ao autor a sugestão do dia se a
// ideia for de hoje.
func handleDeleteIdea(db *store.Store, cfg IdeasConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.InGroupFromContext(r.Context(), cfg.AdminGroup) {
			apierr.Write(w, http.StatusForbidden, "ideas.admin_only_delete", "só administradores excluem ideias")
			return
		}
		if err := db.Ideas.Delete(r.Context(), r.PathValue("id")); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				apierr.Write(w, http.StatusNotFound, "ideas.not_found", "ideia não encontrada")
				return
			}
			log.Printf("ideias: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "ideas.delete_failed", "erro ao excluir")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// PATCH /api/ideas/{id} {status, implementedVersion} — ação do admin:
// aprovar (open), planejar, marcar como implementada ou recusar.
func handleModerateIdea(db *store.Store, cfg IdeasConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, _ := auth.AccountFromContext(r.Context())
		if !auth.InGroupFromContext(r.Context(), cfg.AdminGroup) {
			apierr.Write(w, http.StatusForbidden, "ideas.admin_only_moderate", "só administradores moderam ideias")
			return
		}
		var body struct {
			Status             string  `json:"status"`
			ImplementedVersion *string `json:"implementedVersion"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		switch body.Status {
		case store.IdeaOpen, store.IdeaPlanned, store.IdeaRejected, store.IdeaReview:
			body.ImplementedVersion = nil
		case store.IdeaImplemented:
			if body.ImplementedVersion == nil || !versionPattern.MatchString(strings.TrimSpace(*body.ImplementedVersion)) {
				apierr.Write(w, http.StatusBadRequest, "ideas.version_invalid", `informe a versão no formato do CHANGELOG, ex. "client v0.15.0"`)
				return
			}
			v := strings.TrimSpace(*body.ImplementedVersion)
			body.ImplementedVersion = &v
		default:
			apierr.Write(w, http.StatusBadRequest, "ideas.status_invalid", "status inválido")
			return
		}
		id := r.PathValue("id")
		if err := db.Ideas.SetStatus(r.Context(), id, body.Status, body.ImplementedVersion); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				apierr.Write(w, http.StatusNotFound, "ideas.not_found", "ideia não encontrada")
				return
			}
			log.Printf("ideias: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "ideas.moderate_failed", "erro ao moderar")
			return
		}
		idea, err := db.Ideas.Get(r.Context(), id, account.ID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "ideas.read_failed", "erro ao ler a ideia")
			return
		}
		writeJSON(w, http.StatusOK, toIdeaResponse(idea, account.ID, true))
	})
}
