import type { Member } from '../types'
import './MemberList.css'

interface MemberListProps {
  members: Member[]
}

export function MemberList({ members }: MemberListProps) {
  const online = members.filter((member) => member.online)
  const offline = members.filter((member) => !member.online)

  return (
    <aside className="member-list" aria-label="Membros">
      {online.length > 0 && (
        <div className="member-group">
          <div className="member-group-name">Online — {online.length}</div>
          {online.map((member) => (
            <MemberRow key={member.id} member={member} />
          ))}
        </div>
      )}
      {offline.length > 0 && (
        <div className="member-group">
          <div className="member-group-name">Offline — {offline.length}</div>
          {offline.map((member) => (
            <MemberRow key={member.id} member={member} />
          ))}
        </div>
      )}
    </aside>
  )
}

function MemberRow({ member }: { member: Member }) {
  return (
    <div className={member.online ? 'member' : 'member offline'}>
      <span className="member-status" aria-hidden="true" />
      {member.nickname}
    </div>
  )
}
