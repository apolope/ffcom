import type { Member, Role } from '../types'
import './MemberList.css'

interface MemberListProps {
  members: Member[]
  roles: Role[]
}

// Não há presença por membro de server-channel ainda (ver types.ts), então,
// ao contrário da lista de amigos, não faz sentido separar online/offline —
// só uma lista única, ordenada por quem entrou primeiro (dono no topo).
export function MemberList({ members, roles }: MemberListProps) {
  const roleById = new Map(roles.map((r) => [r.id, r]))
  const sorted = [...members].sort((a, b) => (a.isOwner === b.isOwner ? 0 : a.isOwner ? -1 : 1))

  return (
    <aside className="member-list" aria-label="Membros">
      <div className="member-group">
        <div className="member-group-name">Membros — {members.length}</div>
        {sorted.map((member) => (
          <MemberRow key={member.id} member={member} roleById={roleById} />
        ))}
      </div>
    </aside>
  )
}

function MemberRow({ member, roleById }: { member: Member; roleById: Map<string, Role> }) {
  const highestRole = member.roleIds
    .map((id) => roleById.get(id))
    .filter((r): r is Role => Boolean(r))
    .sort((a, b) => b.position - a.position)[0]

  return (
    <div className="member">
      <span
        className="member-status"
        aria-hidden="true"
        style={highestRole?.color ? { background: highestRole.color } : undefined}
      />
      {member.nickname}
      {member.isOwner && <span className="member-badge">dono</span>}
    </div>
  )
}
