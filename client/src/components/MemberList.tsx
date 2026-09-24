import type { Member, Role } from '../types'
import { MemberAvatar } from './AvatarWithStatus'
import './MemberList.css'

interface MemberListProps {
  members: Member[]
  roles: Role[]
}

// Lista única, ordenada por quem entrou primeiro (dono no topo), sem separar
// online/offline: o status só é visível entre amigos (ver types.ts), então
// quem não é amigo aparece sempre cinza e separar por status enganaria.
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
      <MemberAvatar member={member} size={32} />
      {/* A cor da role mais alta, que antes pintava a bolinha, vai para o
          nome: a bolinha agora é o status. */}
      <span className="member-name" style={highestRole?.color ? { color: highestRole.color } : undefined}>
        {member.nickname}
      </span>
      {member.isOwner && <span className="member-badge">dono</span>}
    </div>
  )
}
