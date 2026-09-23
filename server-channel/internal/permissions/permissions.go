// Package permissions define os bits do bitmask de permissões (coluna
// roles.permissions / channel_role_overwrites.allow,deny) e a lógica de
// combiná-los, sem depender de nenhum detalhe de transporte HTTP — ver
// docs/architecture.md, "Sistema de permissões/roles por servidor e por
// canal".
package permissions

// Bits de permissão. Cabem folgados num BIGINT (63 bits úteis em int64
// positivo), então há bastante espaço para bits futuros sem migration.
const (
	ViewChannels int64 = 1 << iota
	SendMessages
	Voice
	ManageInvites
	ManageRoles
	Administrator
	// KickMembers e BanMembers foram adicionados depois dos bits acima —
	// ver docs/architecture.md, "Decisão: kick/ban de membro". Ficam no
	// fim do bloco de propósito: reordenar bits já existentes mudaria o
	// significado de valores de roles.permissions já persistidos.
	KickMembers
	BanMembers
	// ManageChannels (criar/renomear/mover/apagar categoria e canal) veio
	// depois — ver docs/architecture.md, "Decisão: gerenciar categorias e
	// canais". Mesmo critério: sempre no fim do bloco.
	ManageChannels
)

// Owner é o valor de retorno de quem é dono do servidor (member.IsOwner):
// todos os bits em 1 (representação em complemento de dois de -1), o que
// automaticamente faz Has e Effective tratarem como Administrator e ignorar
// qualquer overwrite de canal — dono não é uma role e não pode ser negado.
const Owner int64 = -1

// Has confere um bit numa permissão efetiva. Administrator (ou Owner, que
// já inclui esse bit) sempre passa em qualquer checagem, do mesmo jeito que
// o Discord trata ADMINISTRATOR.
func Has(effective, bit int64) bool {
	return effective&Administrator != 0 || effective&bit != 0
}

// Base combina (OR) as permissões de várias roles — nunca há subtração no
// nível de role, só overwrite de canal nega bits (ver Effective).
func Base(rolePermissions []int64) int64 {
	var mask int64
	for _, p := range rolePermissions {
		mask |= p
	}
	return mask
}

// Overwrite é um par allow/deny associado a uma role, aplicável só dentro de
// um canal específico.
type Overwrite struct {
	RoleID string
	Allow  int64
	Deny   int64
}

// Effective aplica, sobre a permissão base de um membro (já a soma de todas
// as suas roles, incluindo a default), os overwrites de canal das roles que
// ele tiver: primeiro soma todos os allow, depois remove todos os deny —
// mesma ordem usada pelo Discord (allow de todo overwrite aplicável vence
// deny de outro só se vier depois; aqui, com um overwrite por role, deny
// sempre vence porque é aplicado por último). base == Owner (todos os bits)
// ignora overwrites por completo.
// Grants reports whether base already holds every bit set in target —
// Administrator (or Owner) trivially grants anything. Used to stop
// ManageRoles from being, by itself, enough to hand out a permission (most
// dangerously Administrator) that the holder doesn't actually have: role
// creation/edit and role assignment must check the requester's own base
// permission against the bits being granted before applying them.
func Grants(base, target int64) bool {
	if base&Administrator != 0 {
		return true
	}
	return target&^base == 0
}

func Effective(base int64, memberRoleIDs []string, overwrites []Overwrite) int64 {
	if base&Administrator != 0 {
		return base
	}
	if len(overwrites) == 0 {
		return base
	}

	roleSet := make(map[string]bool, len(memberRoleIDs))
	for _, id := range memberRoleIDs {
		roleSet[id] = true
	}

	var allow, deny int64
	for _, o := range overwrites {
		if !roleSet[o.RoleID] {
			continue
		}
		allow |= o.Allow
		deny |= o.Deny
	}
	return (base | allow) &^ deny
}
