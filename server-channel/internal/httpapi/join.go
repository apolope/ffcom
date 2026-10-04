package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
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
//
// Banimento (member_bans, ver docs/architecture.md, "Decisão: kick/ban de
// membro") é checado antes de tudo, inclusive do bootstrap: um oidc_subject
// banido nunca entra, mesmo sem nenhum membro ainda no servidor.
func handleJoin(members *store.MemberStore, invites *store.InviteStore, bans *store.MemberBanStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "auth.subject_missing", "subject ausente no contexto")
			return
		}

		banned, err := bans.IsBanned(r.Context(), subject)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.ban_check_failed", "erro ao verificar banimento")
			return
		}
		if banned {
			apierr.Write(w, http.StatusForbidden, "members.banned", "banido deste servidor")
			return
		}

		existing, err := members.GetByOIDCSubject(r.Context(), subject)
		if err == nil {
			writeJoinResponse(w, http.StatusOK, existing, false)
			return
		}
		if !errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusInternalServerError, "auth.member_resolve_failed", "erro ao resolver membro")
			return
		}

		count, err := members.Count(r.Context())
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.count_failed", "erro ao contar membros")
			return
		}

		if count == 0 {
			member, err := members.CreateFounder(r.Context(), subject)
			if err != nil {
				apierr.Write(w, http.StatusInternalServerError, "members.create_failed", "erro ao criar membro")
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
			apierr.Write(w, http.StatusForbidden, "invites.required", "convite necessário para entrar neste servidor")
			return
		}

		invite, err := invites.GetByCode(r.Context(), code)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "invites.not_found", "convite não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "invites.fetch_failed", "erro ao buscar convite")
			return
		}
		if invite.ExpiresAt != nil && invite.ExpiresAt.Before(time.Now()) {
			apierr.Write(w, http.StatusGone, "invites.expired", "convite expirado")
			return
		}
		if invite.MaxUses != nil && invite.Uses >= *invite.MaxUses {
			apierr.Write(w, http.StatusConflict, "invites.exhausted", "convite esgotado")
			return
		}

		if _, err := invites.Redeem(r.Context(), invite.ID); err != nil {
			if errors.Is(err, store.ErrConflict) {
				apierr.Write(w, http.StatusConflict, "invites.exhausted_or_expired", "convite esgotado ou expirado")
				return
			}
			apierr.Write(w, http.StatusInternalServerError, "invites.redeem_failed", "erro ao resgatar convite")
			return
		}

		member, err := members.GetOrCreateByOIDCSubject(r.Context(), subject)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.create_failed", "erro ao criar membro")
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
