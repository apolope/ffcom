package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

type overwriteView struct {
	RoleID string `json:"roleId"`
	Allow  int64  `json:"allow"`
	Deny   int64  `json:"deny"`
}

// GET /api/channels/{id}/overwrites — lista os overwrites de role deste
// canal. Requer ManageRoles (mesma permissão usada para editá-los): não há
// motivo para membros comuns verem o bitmask cru de outras roles.
func handleListChannelOverwrites(channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireManageRoles(w, r, roles); !ok {
			return
		}

		channelID := r.PathValue("id")
		if _, err := channels.GetByID(r.Context(), channelID); errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "channels.not_found", "canal não encontrado")
			return
		} else if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "channels.fetch_failed", "erro ao buscar canal")
			return
		}

		rows, err := overwrites.ListForChannel(r.Context(), channelID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "overwrites.list_failed", "erro ao listar overwrites")
			return
		}

		out := make([]overwriteView, len(rows))
		for i, o := range rows {
			out[i] = overwriteView{RoleID: o.RoleID, Allow: o.Allow, Deny: o.Deny}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Overwrites []overwriteView `json:"overwrites"`
		}{Overwrites: out})
	})
}

type setOverwriteRequest struct {
	Allow int64 `json:"allow"`
	Deny  int64 `json:"deny"`
}

// PUT /api/channels/{id}/overwrites/{roleId} — cria ou substitui o
// overwrite de roleId em id (ver internal/permissions, Effective). É o que
// torna um canal privado/restrito: allow/deny sobrescrevem, só neste canal,
// bits da permissão base de quem tiver essa role. Requer ManageRoles.
func handleSetChannelOverwrite(channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base, ok := requireManageRoles(w, r, roles)
		if !ok {
			return
		}

		channelID := r.PathValue("id")
		if _, err := channels.GetByID(r.Context(), channelID); errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "channels.not_found", "canal não encontrado")
			return
		} else if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "channels.fetch_failed", "erro ao buscar canal")
			return
		}

		var body setOverwriteRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo inválido")
			return
		}
		if !permissions.Grants(base, body.Allow) {
			apierr.Write(w, http.StatusForbidden, "overwrites.allow_exceeds_own", "não é possível liberar (allow) uma permissão que você mesmo não possui")
			return
		}
		if _, ok := requireRoleWithinGrants(w, r, roles, base, r.PathValue("roleId"), apierr.New("overwrites.role_exceeds_own", "não é possível mexer no overwrite de uma role com permissões que você mesmo não possui")); !ok {
			return
		}

		o, err := overwrites.Set(r.Context(), channelID, r.PathValue("roleId"), body.Allow, body.Deny)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "overwrites.save_failed", "erro ao salvar overwrite")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(overwriteView{RoleID: o.RoleID, Allow: o.Allow, Deny: o.Deny})
	})
}

// DELETE /api/channels/{id}/overwrites/{roleId} — remove o overwrite, se
// existir. Requer ManageRoles.
func handleDeleteChannelOverwrite(roles *store.RoleStore, overwrites *store.ChannelOverwriteStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base, ok := requireManageRoles(w, r, roles)
		if !ok {
			return
		}
		if _, ok := requireRoleWithinGrants(w, r, roles, base, r.PathValue("roleId"), apierr.New("overwrites.role_exceeds_own", "não é possível mexer no overwrite de uma role com permissões que você mesmo não possui")); !ok {
			return
		}

		err := overwrites.Delete(r.Context(), r.PathValue("id"), r.PathValue("roleId"))
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "overwrites.not_found", "overwrite não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "overwrites.remove_failed", "erro ao remover overwrite")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
