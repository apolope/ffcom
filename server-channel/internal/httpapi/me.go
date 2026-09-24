package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
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
			ProfileName: member.ProfileName,
			JoinedAt:    member.JoinedAt,
			IsOwner:     member.IsOwner,
			Permissions: base,
			RoleIDs:     roleIDs,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

// PATCH /api/me — permite ao próprio membro definir (ou limpar, com
// {"nickname": null}) o apelido exibido neste server-channel. A coluna e o
// store (MemberStore.SetNickname) já existiam desde a migration inicial,
// sem endpoint — a lista de membros (GET /api/members) mostrava o UUID
// truncado do membro na ausência de nickname, já que server-channel não
// tem acesso ao nome real da conta (só o "sub" do Authentik, ver
// docs/architecture.md, "modelo de dados de server-channel").
func handleUpdateMe(members *store.MemberStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}

		var body struct {
			Nickname *string `json:"nickname"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		if body.Nickname != nil {
			trimmed := strings.TrimSpace(*body.Nickname)
			if trimmed == "" {
				body.Nickname = nil
			} else if len(trimmed) > 64 {
				http.Error(w, "apelido deve ter no máximo 64 caracteres", http.StatusBadRequest)
				return
			} else {
				body.Nickname = &trimmed
			}
		}

		updated, err := members.SetNickname(r.Context(), member.ID, body.Nickname)
		if err != nil {
			http.Error(w, "erro ao atualizar apelido", http.StatusInternalServerError)
			return
		}

		base, roleIDs, err := memberBasePermission(r.Context(), roles, updated)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}

		resp := meResponse{
			MemberID:    updated.ID,
			OIDCSubject: updated.OIDCSubject,
			Nickname:    updated.Nickname,
			ProfileName: updated.ProfileName,
			JoinedAt:    updated.JoinedAt,
			IsOwner:     updated.IsOwner,
			Permissions: base,
			RoleIDs:     roleIDs,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

// PUT /api/me/profile-name — o client grava aqui o nome do perfil do
// Authentik da pessoa (claim name, ou preferred_username sem ele), que
// server-channel não recebe de outro jeito: o token só traz o "sub". É o
// nome exibido de quem não escolheu apelido (ver Member.DisplayName e
// docs/architecture.md, "Decisão: nome exibido do membro"). Rota separada
// do PATCH /api/me porque ali "nickname" ausente limpa o apelido.
func handleSetProfileName(members *store.MemberStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}

		var body struct {
			ProfileName *string `json:"profileName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		if body.ProfileName != nil {
			trimmed := strings.TrimSpace(*body.ProfileName)
			if trimmed == "" {
				body.ProfileName = nil
			} else if len(trimmed) > 64 {
				http.Error(w, "nome deve ter no máximo 64 caracteres", http.StatusBadRequest)
				return
			} else {
				body.ProfileName = &trimmed
			}
		}

		updated, err := members.SetProfileName(r.Context(), member.ID, body.ProfileName)
		if err != nil {
			http.Error(w, "erro ao atualizar nome", http.StatusInternalServerError)
			return
		}
		base, roleIDs, err := memberBasePermission(r.Context(), roles, updated)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meResponse{
			MemberID:    updated.ID,
			OIDCSubject: updated.OIDCSubject,
			Nickname:    updated.Nickname,
			ProfileName: updated.ProfileName,
			JoinedAt:    updated.JoinedAt,
			IsOwner:     updated.IsOwner,
			Permissions: base,
			RoleIDs:     roleIDs,
		})
	})
}

type meResponse struct {
	MemberID    string    `json:"memberId"`
	OIDCSubject string    `json:"oidcSubject"`
	Nickname    *string   `json:"nickname,omitempty"`
	ProfileName *string   `json:"profileName,omitempty"`
	JoinedAt    time.Time `json:"joinedAt"`
	IsOwner     bool      `json:"isOwner,omitempty"`
	Permissions int64     `json:"permissions"`
	RoleIDs     []string  `json:"roleIds,omitempty"`
}
