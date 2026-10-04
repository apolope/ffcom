package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// requireMemberPermission resolve a permissão base do membro autenticado e
// devolve 403 (já escrito) se ele não tiver bit nem for dono do servidor.
// Mesmo formato de requireManageRoles (internal/httpapi/roles.go),
// generalizado aqui para não duplicar a função a cada bit novo
// (KickMembers/BanMembers).
func requireMemberPermission(w http.ResponseWriter, r *http.Request, roles *store.RoleStore, bit int64, bitName string) (store.Member, bool) {
	member, ok := auth.MemberFromContext(r.Context())
	if !ok {
		apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
		return store.Member{}, false
	}
	base, _, err := memberBasePermission(r.Context(), roles, member)
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
		return store.Member{}, false
	}
	if !permissions.Has(base, bit) {
		apierr.WriteParams(w, http.StatusForbidden, "permissions.required", "requer a permissão "+bitName, apierr.Params{"permission": bitName})
		return store.Member{}, false
	}
	return member, true
}

// resolveModerationTarget busca o membro alvo de kick/ban e recusa alvos
// inválidos: o próprio requisitante (sair do servidor não é uma feature
// desta v1) e o dono do servidor (nunca pode ser removido — não é uma role,
// ver docs/architecture.md), e quem tem algum bit que o requisitante não tem
// (um moderador não expulsa um Administrador; ver "Decisão: teto por bits
// para remover e rebaixar"). Devolve a resposta HTTP já escrita quando
// recusa.
func resolveModerationTarget(w http.ResponseWriter, r *http.Request, members *store.MemberStore, roles *store.RoleStore, requester store.Member) (store.Member, bool) {
	targetID := r.PathValue("memberId")
	if targetID == requester.ID {
		apierr.Write(w, http.StatusBadRequest, "members.moderate_self", "não é possível expulsar/banir a si mesmo")
		return store.Member{}, false
	}

	target, err := members.GetByID(r.Context(), targetID)
	if errors.Is(err, store.ErrNotFound) {
		apierr.Write(w, http.StatusNotFound, "members.not_found", "membro não encontrado")
		return store.Member{}, false
	}
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "members.fetch_failed", "erro ao buscar membro")
		return store.Member{}, false
	}
	if target.IsOwner {
		apierr.Write(w, http.StatusForbidden, "members.moderate_owner", "não é possível expulsar/banir o dono do servidor")
		return store.Member{}, false
	}

	requesterBase, _, err := memberBasePermission(r.Context(), roles, requester)
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
		return store.Member{}, false
	}
	targetBase, _, err := memberBasePermission(r.Context(), roles, target)
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
		return store.Member{}, false
	}
	if !permissions.Grants(requesterBase, targetBase) {
		apierr.Write(w, http.StatusForbidden, "members.moderate_exceeds_own", "não é possível expulsar/banir quem tem permissões que você não possui")
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
		requester, ok := requireMemberPermission(w, r, roles, permissions.KickMembers, "KickMembers")
		if !ok {
			return
		}
		target, ok := resolveModerationTarget(w, r, members, roles, requester)
		if !ok {
			return
		}

		if _, err := members.Kick(r.Context(), target.ID); errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusConflict, "members.already_gone", "membro já não está mais no servidor")
			return
		} else if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.kick_failed", "erro ao expulsar membro")
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
		requester, ok := requireMemberPermission(w, r, roles, permissions.BanMembers, "BanMembers")
		if !ok {
			return
		}
		target, ok := resolveModerationTarget(w, r, members, roles, requester)
		if !ok {
			return
		}

		var body banRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}

		if _, err := bans.Create(r.Context(), target.OIDCSubject, requester.ID, body.Reason); err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.ban_failed", "erro ao banir membro")
			return
		}

		if _, err := members.Kick(r.Context(), target.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusInternalServerError, "members.kick_banned_failed", "erro ao expulsar membro banido")
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
		if _, ok := requireMemberPermission(w, r, roles, permissions.BanMembers, "BanMembers"); !ok {
			return
		}

		rows, err := bans.List(r.Context())
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.bans_list_failed", "erro ao listar banimentos")
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
		if _, ok := requireMemberPermission(w, r, roles, permissions.BanMembers, "BanMembers"); !ok {
			return
		}

		if err := bans.Delete(r.Context(), r.PathValue("oidcSubject")); errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "members.ban_not_found", "banimento não encontrado")
			return
		} else if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.unban_failed", "erro ao revogar banimento")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
