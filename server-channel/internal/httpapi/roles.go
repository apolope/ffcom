package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
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
// devolve (0, false) (já com a resposta HTTP escrita) se ele não tiver
// ManageRoles nem for dono do servidor. A permissão base devolvida é usada
// por quem chama para checar, via permissions.Grants, que ManageRoles
// sozinho não está sendo usado para conceder um bit que o requisitante não
// possui (ver "Decisão: ManageRoles não concede permissões além das
// próprias" em docs/architecture.md).
func requireManageRoles(w http.ResponseWriter, r *http.Request, roles *store.RoleStore) (int64, bool) {
	member, ok := auth.MemberFromContext(r.Context())
	if !ok {
		apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
		return 0, false
	}
	base, _, err := memberBasePermission(r.Context(), roles, member)
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
		return 0, false
	}
	if !permissions.Has(base, permissions.ManageRoles) {
		apierr.WriteParams(w, http.StatusForbidden, "permissions.required", "requer a permissão ManageRoles", apierr.Params{"permission": "ManageRoles"})
		return 0, false
	}
	return base, true
}

// findRole busca uma role por id entre todas as roles do servidor. Não há
// query dedicada (RoleStore não expõe GetByID) porque List já é usado por
// vários chamadores próximos (rejectDefaultRole, handleDeleteRole) e o
// número de roles por servidor é pequeno.
func findRole(ctx context.Context, roles *store.RoleStore, id string) (store.Role, error) {
	rows, err := roles.List(ctx)
	if err != nil {
		return store.Role{}, err
	}
	for _, role := range rows {
		if role.ID == id {
			return role, nil
		}
	}
	return store.Role{}, store.ErrNotFound
}

// requireRoleWithinGrants busca a role alvo (roleID) e recusa, com a
// resposta HTTP já escrita, se ela tiver algum bit que base não tem: quem
// tem ManageRoles só mexe (edita, apaga, desatribui, grava overwrite) em
// role que ele mesmo poderia ter criado. Sem isso, um moderador conseguia
// apagar a role de Administrador ou tirar o bit dela. Ver
// docs/architecture.md, "Decisão: teto por bits para remover e rebaixar".
func requireRoleWithinGrants(w http.ResponseWriter, r *http.Request, roles *store.RoleStore, base int64, roleID string, denied *apierr.Problem) (store.Role, bool) {
	role, err := findRole(r.Context(), roles, roleID)
	if errors.Is(err, store.ErrNotFound) {
		apierr.Write(w, http.StatusNotFound, "roles.not_found", "role não encontrada")
		return store.Role{}, false
	}
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "roles.fetch_failed", "erro ao buscar role")
		return store.Role{}, false
	}
	if !permissions.Grants(base, role.Permissions) {
		apierr.WriteProblem(w, http.StatusForbidden, denied)
		return store.Role{}, false
	}
	return role, true
}

// GET /api/roles — lista todas as roles do servidor. Aberto a qualquer
// membro (precisa disso pra mostrar nome/cor de role em qualquer lugar da
// UI, não só no painel de administração).
func handleListRoles(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := roles.List(r.Context())
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "roles.list_failed", "erro ao listar roles")
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
		base, ok := requireManageRoles(w, r, roles)
		if !ok {
			return
		}

		var body roleRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo inválido")
			return
		}
		if body.Name == "" {
			apierr.Write(w, http.StatusBadRequest, "common.name_required", "name é obrigatório")
			return
		}
		if !permissions.Grants(base, body.Permissions) {
			apierr.Write(w, http.StatusForbidden, "roles.grant_exceeds_own", "não é possível conceder permissões que você mesmo não possui")
			return
		}

		role, err := roles.Create(r.Context(), body.Name, body.Color, body.Permissions, body.Position)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "roles.create_failed", "erro ao criar role")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toRoleView(role))
	})
}

// PATCH /api/roles/{id} — edita name/color/permissions/position, inclusive
// da role default. Requer ManageRoles, e tanto as permissões novas quanto as
// atuais da role precisam caber nas de quem edita.
func handleUpdateRole(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base, ok := requireManageRoles(w, r, roles)
		if !ok {
			return
		}

		var body roleRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo inválido")
			return
		}
		if body.Name == "" {
			apierr.Write(w, http.StatusBadRequest, "common.name_required", "name é obrigatório")
			return
		}
		if !permissions.Grants(base, body.Permissions) {
			apierr.Write(w, http.StatusForbidden, "roles.grant_exceeds_own", "não é possível conceder permissões que você mesmo não possui")
			return
		}
		if _, ok := requireRoleWithinGrants(w, r, roles, base, r.PathValue("id"), apierr.New("roles.edit_exceeds_own", "não é possível editar uma role com permissões que você mesmo não possui")); !ok {
			return
		}

		role, err := roles.Update(r.Context(), r.PathValue("id"), body.Name, body.Color, body.Permissions, body.Position)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "roles.not_found", "role não encontrada")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "roles.update_failed", "erro ao editar role")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toRoleView(role))
	})
}

// DELETE /api/roles/{id} — remove uma role. A role default ("@everyone")
// não pode ser removida (é seeded pela migration e é a base de permissão de
// todo membro). Requer ManageRoles, e a role não pode ter bit que quem apaga
// não tem.
func handleDeleteRole(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base, ok := requireManageRoles(w, r, roles)
		if !ok {
			return
		}

		id := r.PathValue("id")
		role, ok := requireRoleWithinGrants(w, r, roles, base, id, apierr.New("roles.delete_exceeds_own", "não é possível apagar uma role com permissões que você mesmo não possui"))
		if !ok {
			return
		}
		if role.IsDefault {
			apierr.Write(w, http.StatusBadRequest, "roles.default_not_removable", "a role default não pode ser removida")
			return
		}

		if err := roles.Delete(r.Context(), id); errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "roles.not_found", "role não encontrada")
			return
		} else if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "roles.delete_failed", "erro ao remover role")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// POST /api/members/{memberId}/roles/{roleId} — atribui uma role a um
// membro. Requer ManageRoles.
func handleAssignRole(members *store.MemberStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base, ok := requireManageRoles(w, r, roles)
		if !ok {
			return
		}
		if err := rejectDefaultRole(r, roles); err != nil {
			apierr.Write(w, http.StatusBadRequest, "roles.default_implicit", err.Error())
			return
		}

		if _, ok := requireRoleWithinGrants(w, r, roles, base, r.PathValue("roleId"), apierr.New("roles.assign_exceeds_own", "não é possível atribuir uma role com permissões que você mesmo não possui")); !ok {
			return
		}

		if _, err := members.GetByID(r.Context(), r.PathValue("memberId")); errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "members.not_found", "membro não encontrado")
			return
		} else if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.fetch_failed", "erro ao buscar membro")
			return
		}

		if err := roles.AssignToMember(r.Context(), r.PathValue("memberId"), r.PathValue("roleId")); err != nil {
			apierr.Write(w, http.StatusInternalServerError, "roles.assign_failed", "erro ao atribuir role")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// DELETE /api/members/{memberId}/roles/{roleId} — remove uma role de um
// membro. Requer ManageRoles, e a role não pode ter bit que quem tira não
// tem.
func handleRemoveRole(roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base, ok := requireManageRoles(w, r, roles)
		if !ok {
			return
		}
		if _, ok := requireRoleWithinGrants(w, r, roles, base, r.PathValue("roleId"), apierr.New("roles.unassign_exceeds_own", "não é possível tirar uma role com permissões que você mesmo não possui")); !ok {
			return
		}

		err := roles.RemoveFromMember(r.Context(), r.PathValue("memberId"), r.PathValue("roleId"))
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "roles.member_lacks_role", "membro não tinha essa role")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "roles.unassign_failed", "erro ao remover role do membro")
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
