package httpapi

import (
	"context"

	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// memberBasePermission resolve a permissão do membro no nível do servidor
// (roles atribuídas + a role default "@everyone", sem overwrites de canal) e
// os IDs de role usados para casar overwrites depois. Dono do servidor
// (member.IsOwner) devolve permissions.Owner direto, sem consultar roles.
func memberBasePermission(ctx context.Context, roles *store.RoleStore, member store.Member) (base int64, roleIDs []string, err error) {
	if member.IsOwner {
		return permissions.Owner, nil, nil
	}

	assigned, err := roles.ListForMember(ctx, member.ID)
	if err != nil {
		return 0, nil, err
	}
	defaultRole, err := roles.GetDefault(ctx)
	if err != nil {
		return 0, nil, err
	}

	all := append(assigned, defaultRole)
	perms := make([]int64, len(all))
	roleIDs = make([]string, len(all))
	for i, r := range all {
		perms[i] = r.Permissions
		roleIDs[i] = r.ID
	}
	return permissions.Base(perms), roleIDs, nil
}

// channelPermission resolve a permissão efetiva de member no canal
// channelID, aplicando os overwrites de canal das roles dele sobre a
// permissão base (ver memberBasePermission).
func channelPermission(ctx context.Context, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, member store.Member, channelID string) (int64, error) {
	base, roleIDs, err := memberBasePermission(ctx, roles, member)
	if err != nil {
		return 0, err
	}
	if member.IsOwner {
		return base, nil
	}

	rows, err := overwrites.ListForChannel(ctx, channelID)
	if err != nil {
		return 0, err
	}
	return permissions.Effective(base, roleIDs, toOverwriteList(rows)), nil
}

func toOverwriteList(rows []store.ChannelRoleOverwrite) []permissions.Overwrite {
	out := make([]permissions.Overwrite, len(rows))
	for i, o := range rows {
		out[i] = permissions.Overwrite{RoleID: o.RoleID, Allow: o.Allow, Deny: o.Deny}
	}
	return out
}
