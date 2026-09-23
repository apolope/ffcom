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

// requireMemberPermission resolve a permissão base do membro autenticado e
// devolve 403 (já escrito) se ele não tiver bit nem for dono do servidor.
// Mesmo formato de requireManageRoles (internal/httpapi/roles.go),
// generalizado aqui para não duplicar a função a cada bit novo
// (KickMembers/BanMembers).
func requireMemberPermission(w http.ResponseWriter, r *http.Request, roles *store.RoleStore, bit int64, deniedMessage string) (store.Member, bool) {
	member, ok := auth.MemberFromContext(r.Context())
	if !ok {
		http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
		return store.Member{}, false
	}
	base, _, err := memberBasePermission(r.Context(), roles, member)
	if err != nil {
		http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
		return store.Member{}, false
	}
	if !permissions.Has(base, bit) {
		http.Error(w, deniedMessage, http.StatusForbidden)
		return store.Member{}, false
	}
	return member, true
}

// resolveModerationTarget busca o membro alvo de kick/ban e recusa alvos
// inválidos: o próprio requisitante (sair do servidor não é uma feature
// desta v1) e o dono do servidor (nunca pode ser removido — não é uma role,
// ver docs/architecture.md). Devolve a resposta HTTP já escrita quando
// recusa.
func resolveModerationTarget(w http.ResponseWriter, r *http.Request, members *store.MemberStore, requester store.Member) (store.Member, bool) {
	targetID := r.PathValue("memberId")
	if targetID == requester.ID {
		http.Error(w, "não é possível expulsar/banir a si mesmo", http.StatusBadRequest)
		return store.Member{}, false
	}

	target, err := members.GetByID(r.Context(), targetID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "membro não encontrado", http.StatusNotFound)
		return store.Member{}, false
	}
	if err != nil {
		http.Error(w, "erro ao buscar membro", http.StatusInternalServerError)
		return store.Member{}, false
	}
	if target.IsOwner {
		http.Error(w, "não é possível expulsar/banir o dono do servidor", http.StatusForbidden)
		return store.Member{}, false
	}
	return target, true
}

// POST /api/members/{memberId}/kick — expulsa um membro (ver
// docs/architecture.md, "Decisão: kick/ban de membro"). Requer
// KickMembers. Não impede reentrada: quem for expulso volta a participar
// assim que resgatar um novo convite de alguém com CreateInvites ou ManageInvites.
func handleKickMember(members *store.MemberStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requester, ok := requireMemberPermission(w, r, roles, permissions.KickMembers, "requer a permissão KickMembers")
		if !ok {
			return
		}
		target, ok := resolveModerationTarget(w, r, members, requester)
		if !ok {
			return
		}

		if _, err := members.Kick(r.Context(), target.ID); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "membro já não está mais no servidor", http.StatusConflict)
			return
		} else if err != nil {
			http.Error(w, "erro ao expulsar membro", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

type banRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// POST /api/members/{memberId}/ban — expulsa um membro e bloqueia
// reentrada (member_bans, indexado por oidc_subject) até um unban explícito
// (ver docs/architecture.md, "Decisão: kick/ban de membro"). Requer
// BanMembers. O registro do banimento é criado mesmo que o Kick devolva
// ErrNotFound (membro já tinha sido expulso antes) — banir alguém que já
// saiu continua fazendo sentido, é o próprio propósito de impedir volta.
func handleBanMember(members *store.MemberStore, roles *store.RoleStore, bans *store.MemberBanStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requester, ok := requireMemberPermission(w, r, roles, permissions.BanMembers, "requer a permissão BanMembers")
		if !ok {
			return
		}
		target, ok := resolveModerationTarget(w, r, members, requester)
		if !ok {
			return
		}

		var body banRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}

		if _, err := bans.Create(r.Context(), target.OIDCSubject, requester.ID, body.Reason); err != nil {
			http.Error(w, "erro ao banir membro", http.StatusInternalServerError)
			return
		}

		if _, err := members.Kick(r.Context(), target.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			http.Error(w, "erro ao expulsar membro banido", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

type banView struct {
	OIDCSubject      string    `json:"oidcSubject"`
	BannedByMemberID *string   `json:"bannedByMemberId,omitempty"`
	Reason           *string   `json:"reason,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	LastNickname     *string   `json:"lastNickname,omitempty"`
}

// GET /api/bans — lista os banimentos ativos deste server-channel, para a
// UI de administração poder revisar/revogar. Requer BanMembers.
func handleListBans(bans *store.MemberBanStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireMemberPermission(w, r, roles, permissions.BanMembers, "requer a permissão BanMembers"); !ok {
			return
		}

		rows, err := bans.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao listar banimentos", http.StatusInternalServerError)
			return
		}
		out := make([]banView, len(rows))
		for i, b := range rows {
			out[i] = banView{
				OIDCSubject:      b.OIDCSubject,
				BannedByMemberID: b.BannedByMemberID,
				Reason:           b.Reason,
				CreatedAt:        b.CreatedAt,
				LastNickname:     b.LastNickname,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Bans []banView `json:"bans"`
		}{Bans: out})
	})
}

// DELETE /api/bans/{oidcSubject} — revoga um banimento (unban). Requer
// BanMembers. oidcSubject vai literal na URL: não existe id opaco para um
// banimento, e o "sub" do Authentik já é um identificador estável o
// suficiente para isso.
func handleUnbanMember(bans *store.MemberBanStore, roles *store.RoleStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireMemberPermission(w, r, roles, permissions.BanMembers, "requer a permissão BanMembers"); !ok {
			return
		}

		if err := bans.Delete(r.Context(), r.PathValue("oidcSubject")); errors.Is(err, store.ErrNotFound) {
			http.Error(w, "banimento não encontrado", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "erro ao revogar banimento", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
