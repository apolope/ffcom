package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// GET /api/me — devolve o membro autenticado nesta instância de
// server-channel (associado via POST /api/join, ver internal/httpapi/join.go),
// junto com a permissão base efetiva (roles + default) e os IDs de role
// atribuídas — o client usa isso para decidir se mostra UI de administração
// (gerenciar convites/roles) sem precisar adivinhar via tentativa e erro.
func handleMe(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}

		base, roleIDs, err := memberBasePermission(r.Context(), roles, member)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}

		resp := meResponse{
			MemberID:    member.ID,
			OIDCSubject: member.OIDCSubject,
			Nickname:    member.Nickname,
			JoinedAt:    member.JoinedAt,
			IsOwner:     member.IsOwner,
			Permissions: base,
			RoleIDs:     roleIDs,
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
	IsOwner     bool      `json:"isOwner,omitempty"`
	Permissions int64     `json:"permissions"`
	RoleIDs     []string  `json:"roleIds,omitempty"`
}
