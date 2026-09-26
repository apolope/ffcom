import { useEffect, useRef } from 'react'
import { useMenuDismiss } from '../hooks/useMenuDismiss'
import type { ChosenStatus } from '../types'
import { STATUS_LABELS } from './PresenceContext'
import './RailMenu.css'
import { UserAvatar } from './UserAvatar'
import './StatusMenu.css'

const OPTIONS: { status: ChosenStatus; hint?: string }[] = [
  { status: 'online' },
  { status: 'busy', hint: 'Notificações ficam desativadas' },
  { status: 'away' },
  { status: 'invisible', hint: 'Aparece offline para os amigos' },
]

// Quem está logado, para o cabeçalho do menu.
export interface AccountIdentity {
  displayName: string
  avatarUrl?: string
  // preferred_username do Authentik; omitido quando igual ao displayName.
  username?: string
  email?: string
  // Apelido no servidor aberto, se houver.
  nickname?: string
  serverName?: string
}

interface StatusMenuProps {
  // O botão do avatar: dá a posição e não conta como "clique fora" (o
  // próprio botão já alterna o menu).
  anchor: HTMLElement
  identity: AccountIdentity
  chosen: ChosenStatus
  onChoose: (status: ChosenStatus) => void
  onEditAvatar: () => void
  // Apelido é por server-channel: só vem com um servidor aberto.
  onEditNickname?: () => void
  onClose: () => void
}

// Menu do próprio avatar no ServerRail: mostra quem está logado (nome,
// usuário e e-mail do Authentik, apelido no servidor aberto), escolhe o status e abre os
// diálogos de apelido (do servidor aberto) e de avatar. Posição fixa ao lado do botão, porque o rail rola
// (overflow-y) e cortaria um menu posicionado dentro dele. Fecha com Esc ou
// clique fora. Ver docs/architecture.md, "Decisão: status de presença e
// avatar nas listas de membros".
export function StatusMenu({ anchor, identity, chosen, onChoose, onEditAvatar, onEditNickname, onClose }: StatusMenuProps) {
  const ref = useRef<HTMLDivElement>(null)

  useMenuDismiss(ref, anchor, onClose)
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus()
  }, [])

  const rect = anchor.getBoundingClientRect()

  return (
    <div
      ref={ref}
      className="rail-menu"
      role="menu"
      aria-label="Seu status"
      style={{ left: rect.right + 8, bottom: Math.max(8, window.innerHeight - rect.bottom) }}
    >
      <div className="status-menu-identity">
        <UserAvatar avatarUrl={identity.avatarUrl} displayName={identity.displayName} size={40} />
        <div className="status-menu-identity-text">
          <strong title={identity.displayName}>{identity.displayName}</strong>
          {identity.username && <span title={identity.username}>@{identity.username}</span>}
          {identity.email && <span title={identity.email}>{identity.email}</span>}
          {identity.nickname && (
            <span title={`Apelido em ${identity.serverName ?? 'este servidor'}`}>
              Apelido em {identity.serverName ?? 'este servidor'}: {identity.nickname}
            </span>
          )}
        </div>
      </div>
      <hr />
      {OPTIONS.map(({ status, hint }) => (
        <button
          key={status}
          type="button"
          role="menuitemradio"
          aria-checked={status === chosen}
          className="rail-menu-item"
          onClick={() => {
            onChoose(status)
            onClose()
          }}
        >
          <span className={`status-menu-dot presence-${status === 'invisible' ? 'offline' : status}`} aria-hidden="true" />
          <span>
            {STATUS_LABELS[status]}
            {hint && <span className="status-menu-hint">{hint}</span>}
          </span>
        </button>
      ))}
      <hr />
      {onEditNickname && (
        <button
          type="button"
          role="menuitem"
          className="rail-menu-item"
          onClick={() => {
            onEditNickname()
            onClose()
          }}
        >
          Alterar apelido
        </button>
      )}
      <button
        type="button"
        role="menuitem"
        className="rail-menu-item"
        onClick={() => {
          onEditAvatar()
          onClose()
        }}
      >
        Alterar avatar
      </button>
    </div>
  )
}
