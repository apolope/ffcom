package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

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
		if !requireManageRoles(w, r, roles) {
			return
		}

		channelID := r.PathValue("id")
		if _, err := channels.GetByID(r.Context(), channelID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "erro ao buscar canal", http.StatusInternalServerError)
			return
		}

		rows, err := overwrites.ListForChannel(r.Context(), channelID)
		if err != nil {
			http.Error(w, "erro ao listar overwrites", http.StatusInternalServerError)
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
		if !requireManageRoles(w, r, roles) {
			return
		}

		channelID := r.PathValue("id")
		if _, err := channels.GetByID(r.Context(), channelID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "erro ao buscar canal", http.StatusInternalServerError)
			return
		}

		var body setOverwriteRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}

		o, err := overwrites.Set(r.Context(), channelID, r.PathValue("roleId"), body.Allow, body.Deny)
		if err != nil {
			http.Error(w, "erro ao salvar overwrite", http.StatusInternalServerError)
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
		if !requireManageRoles(w, r, roles) {
			return
		}

		err := overwrites.Delete(r.Context(), r.PathValue("id"), r.PathValue("roleId"))
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "overwrite não encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao remover overwrite", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
