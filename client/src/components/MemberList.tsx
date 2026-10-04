import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Member, Role } from '../types'
import { useMenuDismiss } from '../hooks/useMenuDismiss'
import { MemberAvatar } from './AvatarWithStatus'
import { usePresence } from './PresenceContext'
import './MemberList.css'
import './RailMenu.css'

// Relação de amizade entre a conta autenticada e a conta por trás de um
// membro (ver hooks/useFriends.ts).
export type FriendRelation = 'self' | 'friend' | 'incoming' | 'outgoing' | 'none'

export interface MemberFriendActions {
  relationOf: (accountId: string) => FriendRelation
  onAddFriend: (accountId: string, nickname: string) => void
  onAcceptRequest: (accountId: string) => void
  onDeclineRequest: (accountId: string) => void
  onMessage: (accountId: string) => void
}

interface MemberListProps {
  members: Member[]
  roles: Role[]
  friendActions: MemberFriendActions
}

// Lista única, ordenada por quem entrou primeiro (dono no topo), sem separar
// online/offline: o status só é visível entre amigos (ver types.ts), então
// quem não é amigo aparece sempre cinza e separar por status enganaria.
// Botão direito num membro abre as ações de amizade (ver docs/architecture.md,
// "Decisão: pedido de amizade pela lista de membros").
export function MemberList({ members, roles, friendActions }: MemberListProps) {
  const { t } = useTranslation()
  const roleById = new Map(roles.map((r) => [r.id, r]))
  const sorted = [...members].sort((a, b) => (a.isOwner === b.isOwner ? 0 : a.isOwner ? -1 : 1))
  const [menu, setMenu] = useState<{ member: Member; anchor: HTMLElement; x: number; y: number }>()

  return (
    <aside className="member-list" aria-label={t('members.title')}>
      <div className="member-group">
        <div className="member-group-name">{t('members.header', { total: members.length })}</div>
        {sorted.map((member) => (
          <MemberRow
            key={member.id}
            member={member}
            roleById={roleById}
            onContextMenu={(anchor, x, y) => setMenu({ member, anchor, x, y })}
          />
        ))}
      </div>
      {menu && (
        <MemberMenu
          key={menu.member.id}
          member={menu.member}
          anchor={menu.anchor}
          x={menu.x}
          y={menu.y}
          actions={friendActions}
          onClose={() => setMenu(undefined)}
        />
      )}
    </aside>
  )
}

function MemberRow({
  member,
  roleById,
  onContextMenu,
}: {
  member: Member
  roleById: Map<string, Role>
  onContextMenu: (anchor: HTMLElement, x: number, y: number) => void
}) {
  const { t } = useTranslation()
  const highestRole = member.roleIds
    .map((id) => roleById.get(id))
    .filter((r): r is Role => Boolean(r))
    .sort((a, b) => b.position - a.position)[0]

  return (
    <div
      className="member"
      onContextMenu={(event) => {
        event.preventDefault()
        onContextMenu(event.currentTarget, event.clientX, event.clientY)
      }}
    >
      <MemberAvatar member={member} size={32} />
      {/* A cor da role mais alta, que antes pintava a bolinha, vai para o
          nome: a bolinha agora é o status. */}
      <span className="member-name" style={highestRole?.color ? { color: highestRole.color } : undefined}>
        {member.nickname}
      </span>
      {member.isOwner && (
        <span className="member-owner-star" title={t('members.owner')} role="img" aria-label={t('members.owner')}>
          ★
        </span>
      )}
    </div>
  )
}

// Largura/altura aproximadas do menu, para não abrir para fora da tela: a
// lista de membros fica encostada na borda direita.
const MENU_WIDTH = 216
const MENU_HEIGHT = 110

// Menu de contexto de um membro. Mesmo visual e fechamento dos menus do
// ServerRail (RailMenu.css, useMenuDismiss), mas aberto onde o clique
// aconteceu.
function MemberMenu({
  member,
  anchor,
  x,
  y,
  actions,
  onClose,
}: {
  member: Member
  anchor: HTMLElement
  x: number
  y: number
  actions: MemberFriendActions
  onClose: () => void
}) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDivElement>(null)
  useMenuDismiss(ref, anchor, onClose)
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('button')?.focus()
  }, [])

  const { accountOf } = usePresence()
  const accountId = accountOf(member.oidcSubject)?.accountId
  const relation = accountId ? actions.relationOf(accountId) : undefined

  const items: { label: string; run: () => void }[] = []
  let note: string | undefined
  if (!accountId) {
    note = t('members.accountUnavailable')
  } else if (relation === 'none') {
    items.push({ label: t('members.addFriend'), run: () => actions.onAddFriend(accountId, member.nickname) })
  } else if (relation === 'incoming') {
    items.push({ label: t('members.acceptRequest'), run: () => actions.onAcceptRequest(accountId) })
    items.push({ label: t('members.declineRequest'), run: () => actions.onDeclineRequest(accountId) })
  } else if (relation === 'outgoing') {
    note = t('members.requestSent')
  } else if (relation === 'friend') {
    items.push({ label: t('members.sendMessage'), run: () => actions.onMessage(accountId) })
  } else {
    note = t('members.thisIsYou')
  }

  return (
    <div
      ref={ref}
      className="rail-menu"
      role="menu"
      aria-label={t('members.actionsFor', { name: member.nickname })}
      style={{
        left: Math.max(8, Math.min(x, window.innerWidth - MENU_WIDTH)),
        top: Math.max(8, Math.min(y, window.innerHeight - MENU_HEIGHT)),
      }}
    >
      {note && <span className="rail-menu-empty">{note}</span>}
      {items.map(({ label, run }) => (
        <button
          key={label}
          type="button"
          role="menuitem"
          className="rail-menu-item"
          onClick={() => {
            run()
            onClose()
          }}
        >
          {label}
        </button>
      ))}
    </div>
  )
}
