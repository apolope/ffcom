package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
)

// GET /api/me — devolve o membro autenticado nesta instância de
// server-channel (criado no primeiro login pelo próprio auth.Middleware).
func handleMe() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}

		resp := meResponse{
			MemberID:    member.ID,
			OIDCSubject: member.OIDCSubject,
			Nickname:    member.Nickname,
			JoinedAt:    member.JoinedAt,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

type meResponse struct {
	MemberID    string    `json:"memberId"`
	OIDCSubject string    `json:"oidcSubject"`
	Nickname    *string   `json:"nickname,omitempty"`
	JoinedAt    time.Time `json:"joinedAt"`
}
