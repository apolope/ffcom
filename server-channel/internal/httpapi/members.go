package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

type memberView struct {
	ID       string    `json:"id"`
	Nickname *string   `json:"nickname,omitempty"`
	JoinedAt time.Time `json:"joinedAt"`
	IsOwner  bool      `json:"isOwner,omitempty"`
	RoleIDs  []string  `json:"roleIds,omitempty"`
}

// GET /api/members — lista os membros do servidor com suas roles
// atribuídas, para a UI de membros/gerenciamento de roles (ver
// docs/architecture.md, "Sistema de permissões/roles por servidor e por
// canal" — fecha a lacuna notada em client/src/App.tsx sobre não existir
// API real de lista de membros). Não expõe oidcSubject: não há necessidade
// de vazar o identificador do Authentik central para outros membros.
func handleListMembers(members *store.MemberStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := members.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao listar membros", http.StatusInternalServerError)
			return
		}

		ids := make([]string, len(rows))
		for i, m := range rows {
			ids[i] = m.ID
		}
		assignments, err := roles.AssignmentsForMembers(r.Context(), ids)
		if err != nil {
			http.Error(w, "erro ao buscar roles dos membros", http.StatusInternalServerError)
			return
		}

		out := make([]memberView, len(rows))
		for i, m := range rows {
			out[i] = memberView{
				ID:       m.ID,
				Nickname: m.Nickname,
				JoinedAt: m.JoinedAt,
				IsOwner:  m.IsOwner,
				RoleIDs:  assignments[m.ID],
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Members []memberView `json:"members"`
		}{Members: out})
	})
}
