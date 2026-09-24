import type { Member, PresenceStatus } from '../types'
import { STATUS_LABELS, usePresence } from './PresenceContext'
import { UserAvatar } from './UserAvatar'
import './AvatarWithStatus.css'

interface AvatarWithStatusProps {
  avatarUrl?: string
  displayName: string
  status: PresenceStatus
  size?: number
}

// Avatar redondo com a bolinha de status no canto: verde online, vermelha
// ocupado, amarela ausente, cinza offline (e invisível, que para os outros é
// offline). A borda da bolinha usa --presence-ring, para cada lista casar
// com a cor do próprio fundo.
export function AvatarWithStatus({ avatarUrl, displayName, status, size = 32 }: AvatarWithStatusProps) {
  const dot = Math.max(8, Math.round(size * 0.32))
  return (
    <span className="avatar-with-status" style={{ width: size, height: size }}>
      <UserAvatar avatarUrl={avatarUrl} displayName={displayName} size={size} />
      <span
        className={`presence-dot presence-${status}`}
        style={{ width: dot, height: dot }}
        title={STATUS_LABELS[status]}
        role="img"
        aria-label={STATUS_LABELS[status]}
      />
    </span>
  )
}

// Um membro de server-channel: avatar da conta de server-central (achada
// pelo oidcSubject) e status, se for você ou um amigo.
export function MemberAvatar({ member, size }: { member: Member; size?: number }) {
  const { accountOf, statusOf } = usePresence()
  const account = accountOf(member.oidcSubject)
  return (
    <AvatarWithStatus
      avatarUrl={account?.avatarUrl}
      displayName={member.nickname}
      status={statusOf(account?.accountId)}
      size={size}
    />
  )
}
