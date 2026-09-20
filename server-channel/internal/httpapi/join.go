package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

type joinRequest struct {
	Code string `json:"code,omitempty"`
}

type joinResponse struct {
	MemberID string `json:"memberId"`
	Founder  bool   `json:"founder,omitempty"`
}

// POST /api/join — associa o "sub" autenticado (anexado por auth.VerifyToken)
// a este server-channel. Sem nenhum membro ainda, o primeiro "sub" a chamar
// esta rota entra como fundador, sem precisar de convite — é o próprio
// self-hoster subindo a instância pela primeira vez. Depois disso, entrar
// exige `code` de um convite válido (ver docs/architecture.md, "Convites
// obrigatórios para entrar em server-channel"). Idempotente: quem já é
// membro recebe 200 mesmo sem `code`.
//
// A checagem "count == 0" do bootstrap não é atômica com a criação do
// membro — em teoria duas requisições simultâneas no instante exato da
// primeira configuração do servidor poderiam ambas virar fundador. Aceitável
// aqui porque só o próprio self-hoster está nesse momento, sozinho.
func handleJoin(members *store.MemberStore, invites *store.InviteStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			http.Error(w, "subject ausente no contexto", http.StatusInternalServerError)
			return
		}

		existing, err := members.GetByOIDCSubject(r.Context(), subject)
		if err == nil {
			writeJoinResponse(w, http.StatusOK, existing, false)
			return
		}
		if !errors.Is(err, store.ErrNotFound) {
			http.Error(w, "erro ao resolver membro", http.StatusInternalServerError)
			return
		}

		count, err := members.Count(r.Context())
		if err != nil {
			http.Error(w, "erro ao contar membros", http.StatusInternalServerError)
			return
		}

		if count == 0 {
			member, err := members.CreateFounder(r.Context(), subject)
			if err != nil {
				http.Error(w, "erro ao criar membro", http.StatusInternalServerError)
				return
			}
			writeJoinResponse(w, http.StatusCreated, member, true)
			return
		}

		var body joinRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		code := strings.TrimSpace(body.Code)
		if code == "" {
			http.Error(w, "convite necessário para entrar neste servidor", http.StatusForbidden)
			return
		}

		invite, err := invites.GetByCode(r.Context(), code)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "convite não encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao buscar convite", http.StatusInternalServerError)
			return
		}
		if invite.ExpiresAt != nil && invite.ExpiresAt.Before(time.Now()) {
			http.Error(w, "convite expirado", http.StatusGone)
			return
		}
		if invite.MaxUses != nil && invite.Uses >= *invite.MaxUses {
			http.Error(w, "convite esgotado", http.StatusConflict)
			return
		}

		if _, err := invites.Redeem(r.Context(), invite.ID); err != nil {
			if errors.Is(err, store.ErrConflict) {
				http.Error(w, "convite esgotado ou expirado", http.StatusConflict)
				return
			}
			http.Error(w, "erro ao resgatar convite", http.StatusInternalServerError)
			return
		}

		member, err := members.GetOrCreateByOIDCSubject(r.Context(), subject)
		if err != nil {
			http.Error(w, "erro ao criar membro", http.StatusInternalServerError)
			return
		}
		writeJoinResponse(w, http.StatusCreated, member, false)
	})
}

func writeJoinResponse(w http.ResponseWriter, status int, member store.Member, founder bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(joinResponse{MemberID: member.ID, Founder: founder})
}
