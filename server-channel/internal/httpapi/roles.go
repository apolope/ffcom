package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

type roleView struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Color       *string   `json:"color,omitempty"`
	Permissions int64     `json:"permissions"`
	Position    int       `json:"position"`
	IsDefault   bool      `json:"isDefault"`
	CreatedAt   time.Time `json:"createdAt"`
}

func toRoleView(r store.Role) roleView {
	return roleView{
		ID:          r.ID,
		Name:        r.Name,
		Color:       r.Color,
		Permissions: r.Permissions,
		Position:    r.Position,
		IsDefault:   r.IsDefault,
		CreatedAt:   r.CreatedAt,
	}
}

type roleRequest struct {
	Name        string  `json:"name"`
	Color       *string `json:"color,omitempty"`
	Permissions int64   `json:"permissions"`
	Position    int     `json:"position"`
}

// requireManageRoles resolve a permissão base do membro autenticado e
// devolve false (já com a resposta HTTP escrita) se ele não tiver
// ManageRoles nem for dono do servidor.
func requireManageRoles(w http.ResponseWriter, r *http.Request, roles *store.RoleStore) bool {
	member, ok := auth.MemberFromContext(r.Context())
	if !ok {
		http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
		return false
	}
	base, _, err := memberBasePermission(r.Context(), roles, member)
	if err != nil {
		http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
		return false
	}
	if !permissions.Has(base, permissions.ManageRoles) {
		http.Error(w, "requer a permissão ManageRoles", http.StatusForbidden)
		return false
	}
	return true
}

// GET /api/roles — lista todas as roles do servidor. Aberto a qualquer
// membro (precisa disso pra mostrar nome/cor de role em qualquer lugar da
// UI, não só no painel de administração).
func handleListRoles(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := roles.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao listar roles", http.StatusInternalServerError)
			return
		}
		out := make([]roleView, len(rows))
		for i, role := range rows {
			out[i] = toRoleView(role)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Roles []roleView `json:"roles"`
		}{Roles: out})
	})
}

// POST /api/roles — cria uma role nova. Requer ManageRoles.
func handleCreateRole(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireManageRoles(w, r, roles) {
			return
		}

		var body roleRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		if body.Name == "" {
			http.Error(w, "name é obrigatório", http.StatusBadRequest)
			return
		}

		role, err := roles.Create(r.Context(), body.Name, body.Color, body.Permissions, body.Position)
		if err != nil {
			http.Error(w, "erro ao criar role", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toRoleView(role))
	})
}

// PATCH /api/roles/{id} — edita name/color/permissions/position, inclusive
// da role default. Requer ManageRoles.
func handleUpdateRole(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireManageRoles(w, r, roles) {
			return
		}

		var body roleRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		if body.Name == "" {
			http.Error(w, "name é obrigatório", http.StatusBadRequest)
			return
		}

		role, err := roles.Update(r.Context(), r.PathValue("id"), body.Name, body.Color, body.Permissions, body.Position)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "role não encontrada", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao editar role", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toRoleView(role))
	})
}

// DELETE /api/roles/{id} — remove uma role. A role default ("@everyone")
// não pode ser removida (é seeded pela migration e é a base de permissão de
// todo membro). Requer ManageRoles.
func handleDeleteRole(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireManageRoles(w, r, roles) {
			return
		}

		id := r.PathValue("id")
		rows, err := roles.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao verificar role", http.StatusInternalServerError)
			return
		}
		for _, role := range rows {
			if role.ID == id && role.IsDefault {
				http.Error(w, "a role default não pode ser removida", http.StatusBadRequest)
				return
			}
		}

		if err := roles.Delete(r.Context(), id); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "role não encontrada", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "erro ao remover role", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// POST /api/members/{memberId}/roles/{roleId} — atribui uma role a um
// membro. Requer ManageRoles.
func handleAssignRole(members *store.MemberStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireManageRoles(w, r, roles) {
			return
		}
		if err := rejectDefaultRole(r, roles); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if _, err := members.GetByID(r.Context(), r.PathValue("memberId")); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "membro não encontrado", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "erro ao buscar membro", http.StatusInternalServerError)
			return
		}

		if err := roles.AssignToMember(r.Context(), r.PathValue("memberId"), r.PathValue("roleId")); err != nil {
			http.Error(w, "erro ao atribuir role", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// DELETE /api/members/{memberId}/roles/{roleId} — remove uma role de um
// membro. Requer ManageRoles.
func handleRemoveRole(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requireManageRoles(w, r, roles) {
			return
		}

		err := roles.RemoveFromMember(r.Context(), r.PathValue("memberId"), r.PathValue("roleId"))
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "membro não tinha essa role", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao remover role do membro", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// rejectDefaultRole impede atribuir/remover a role default via
// /api/members/{memberId}/roles/{roleId} — ela já é implícita a todo membro
// (ver memberBasePermission), então uma linha em member_roles pra ela seria
// só um no-op confuso.
func rejectDefaultRole(r *http.Request, roles *store.RoleStore) error {
	rows, err := roles.List(r.Context())
	if err != nil {
		return nil // deixa a operação seguinte reportar o erro de infra
	}
	roleID := r.PathValue("roleId")
	for _, role := range rows {
		if role.ID == roleID && role.IsDefault {
			return errDefaultRoleImplicit
		}
	}
	return nil
}

var errDefaultRoleImplicit = errors.New("a role default já é implícita a todos os membros")
