import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { errorMessage } from '../lib/apiError'
import { useMenuDismiss } from '../hooks/useMenuDismiss'
import { friendRequestName, type FriendRequest } from '../lib/serverCentralApi'
import type { Friend } from '../types'
import { AvatarWithStatus } from './AvatarWithStatus'
import { UserAvatar } from './UserAvatar'
import './FriendsView.css'
import './RailMenu.css'

interface FriendsViewProps {
  friends: Friend[]
  selectedFriendId: string | undefined
  // Ver hooks/useUnread.ts e docs/architecture.md, "Decisão: indicador de
  // não lida".
  unreadFriendIds: Set<string>
  onSelectFriend: (accountId: string) => void
  onAddFriend: () => void
  // Pedidos pendentes (ver hooks/useFriends.ts): recebidos com aprovar e
  // recusar, enviados com cancelar.
  incomingRequests: FriendRequest[]
  outgoingRequests: FriendRequest[]
  onAcceptRequest: (id: string) => void
  onRemoveRequest: (id: string) => void
  // Desfaz a amizade (botão direito num amigo); rejeita com o erro do
  // servidor, que o menu mostra sem fechar.
  onRemoveFriend: (accountId: string) => Promise<void>
}

// Lista de amigos + presença (via server-central, ver hooks/useFriends.ts).
// Ocupa a coluna de "canais" quando o botão "Amigos" do ServerRail está
// selecionado; clicar num amigo abre a conversa de DM correspondente (ver
// components/DirectMessageView.tsx e docs/architecture.md, "Decisão: modelo
// de DMs").
export function FriendsView({
  friends,
  selectedFriendId,
  unreadFriendIds,
  onSelectFriend,
  onAddFriend,
  incomingRequests,
  outgoingRequests,
  onAcceptRequest,
  onRemoveRequest,
  onRemoveFriend,
}: FriendsViewProps) {
  const online = friends.filter((f) => f.online)
  const offline = friends.filter((f) => !f.online)
  const [menu, setMenu] = useState<{ friend: Friend; anchor: HTMLElement; x: number; y: number }>()
  const { t } = useTranslation()

  return (
    <nav className="friends-view" aria-label={t('friends.title')}>
      <header className="friends-header">
        <h1>{t('friends.title')}</h1>
        <button type="button" onClick={onAddFriend}>
          {t('friends.addFriend')}
        </button>
      </header>

      {(incomingRequests.length > 0 || outgoingRequests.length > 0) && (
        <div className="friends-list friends-requests">
          {incomingRequests.length > 0 && (
            <div className="friends-group">
              <div className="friends-group-name">
                {t('friends.incomingRequests', { total: incomingRequests.length })}
              </div>
              {incomingRequests.map((request) => (
                <FriendRequestRow key={request.id} request={request} hint={t('friends.wantsToBeFriends')}>
                  <button
                    type="button"
                    className="friend-request-action accept"
                    title={t('friends.accept')}
                    aria-label={t('friends.acceptRequestFrom', { name: friendRequestName(request) })}
                    onClick={() => onAcceptRequest(request.id)}
                  >
                    ✓
                  </button>
                  <button
                    type="button"
                    className="friend-request-action decline"
                    title={t('friends.decline')}
                    aria-label={t('friends.declineRequestFrom', { name: friendRequestName(request) })}
                    onClick={() => onRemoveRequest(request.id)}
                  >
                    ✕
                  </button>
                </FriendRequestRow>
              ))}
            </div>
          )}
          {outgoingRequests.length > 0 && (
            <div className="friends-group">
              <div className="friends-group-name">
                {t('friends.outgoingRequests', { total: outgoingRequests.length })}
              </div>
              {outgoingRequests.map((request) => (
                <FriendRequestRow key={request.id} request={request} hint={t('friends.awaitingResponse')}>
                  <button
                    type="button"
                    className="friend-request-action decline"
                    title={t('friends.cancelRequest')}
                    aria-label={t('friends.cancelRequestTo', { name: friendRequestName(request) })}
                    onClick={() => onRemoveRequest(request.id)}
                  >
                    ✕
                  </button>
                </FriendRequestRow>
              ))}
            </div>
          )}
        </div>
      )}

      {friends.length === 0 ? (
        <div className="friends-empty">
          <p>{t('friends.empty')}</p>
        </div>
      ) : (
        <div className="friends-list">
          {online.length > 0 && (
            <div className="friends-group">
              <div className="friends-group-name">{t('friends.onlineGroup', { total: online.length })}</div>
              {online.map((friend) => (
                <FriendRow
                  key={friend.accountId}
                  friend={friend}
                  active={friend.accountId === selectedFriendId}
                  unread={unreadFriendIds.has(friend.accountId)}
                  onSelect={onSelectFriend}
                  onContextMenu={(anchor, x, y) => setMenu({ friend, anchor, x, y })}
                />
              ))}
            </div>
          )}
          {offline.length > 0 && (
            <div className="friends-group">
              <div className="friends-group-name">{t('friends.offlineGroup', { total: offline.length })}</div>
              {offline.map((friend) => (
                <FriendRow
                  key={friend.accountId}
                  friend={friend}
                  active={friend.accountId === selectedFriendId}
                  unread={unreadFriendIds.has(friend.accountId)}
                  onSelect={onSelectFriend}
                  onContextMenu={(anchor, x, y) => setMenu({ friend, anchor, x, y })}
                />
              ))}
            </div>
          )}
        </div>
      )}

      {menu && (
        <FriendMenu
          key={menu.friend.accountId}
          friend={menu.friend}
          anchor={menu.anchor}
          x={menu.x}
          y={menu.y}
          onMessage={() => onSelectFriend(menu.friend.accountId)}
          onRemove={() => onRemoveFriend(menu.friend.accountId)}
          onClose={() => setMenu(undefined)}
        />
      )}
    </nav>
  )
}

function FriendRow({
  friend,
  active,
  unread,
  onSelect,
  onContextMenu,
}: {
  friend: Friend
  active: boolean
  unread: boolean
  onSelect: (accountId: string) => void
  onContextMenu: (anchor: HTMLElement, x: number, y: number) => void
}) {
  const { t } = useTranslation()
  return (
    <button
      type="button"
      className={
        friend.online
          ? active
            ? 'friend-row active'
            : 'friend-row'
          : active
            ? 'friend-row offline active'
            : 'friend-row offline'
      }
      onClick={() => onSelect(friend.accountId)}
      onContextMenu={(event) => {
        event.preventDefault()
        onContextMenu(event.currentTarget, event.clientX, event.clientY)
      }}
    >
      <AvatarWithStatus
        avatarUrl={friend.avatarUrl}
        displayName={friend.displayName}
        status={friend.status}
        size={24}
      />
      {friend.displayName}
      {unread && <span className="unread-dot" aria-label={t('friends.unreadMessages')} />}
    </button>
  )
}

// Largura/altura aproximadas do menu com a confirmação aberta, para não abrir
// para fora da tela.
const MENU_WIDTH = 240
const MENU_HEIGHT = 150

// Menu de contexto de um amigo, mesmo visual e fechamento do menu de membro
// (components/MemberList.tsx). Remover pede um segundo clique dentro do
// próprio menu, no lugar de um confirm() do navegador, como a exclusão de
// canal em StructureDialogs.tsx.
function FriendMenu({
  friend,
  anchor,
  x,
  y,
  onMessage,
  onRemove,
  onClose,
}: {
  friend: Friend
  anchor: HTMLElement
  x: number
  y: number
  onMessage: () => void
  onRemove: () => Promise<void>
  onClose: () => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  useMenuDismiss(ref, anchor, onClose)
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>()
  const { t } = useTranslation()
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('button')?.focus()
  }, [confirming])

  async function remove() {
    setBusy(true)
    setError(undefined)
    try {
      await onRemove()
      onClose()
    } catch (err) {
      setError(err)
      setBusy(false)
    }
  }

  return (
    <div
      ref={ref}
      className="rail-menu friend-menu"
      role="menu"
      aria-label={t('friends.actionsFor', { name: friend.displayName })}
      style={{
        left: Math.max(8, Math.min(x, window.innerWidth - MENU_WIDTH)),
        top: Math.max(8, Math.min(y, window.innerHeight - MENU_HEIGHT)),
      }}
    >
      {confirming ? (
        <>
          <span className="rail-menu-empty">{t('friends.removeConfirm', { name: friend.displayName })}</span>
          {error !== undefined && (
            <span className="friend-menu-error">{errorMessage(error, t('friends.removeFailed'))}</span>
          )}
          <button
            type="button"
            role="menuitem"
            className="rail-menu-item friend-menu-danger"
            disabled={busy}
            onClick={() => void remove()}
          >
            {busy ? t('friends.removing') : t('friends.confirmRemove')}
          </button>
          <button type="button" role="menuitem" className="rail-menu-item" disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </button>
        </>
      ) : (
        <>
          <button
            type="button"
            role="menuitem"
            className="rail-menu-item"
            onClick={() => {
              onMessage()
              onClose()
            }}
          >
            {t('friends.sendMessage')}
          </button>
          <button
            type="button"
            role="menuitem"
            className="rail-menu-item friend-menu-danger"
            onClick={() => setConfirming(true)}
          >
            {t('friends.removeFriend')}
          </button>
        </>
      )}
    </div>
  )
}

function FriendRequestRow({
  request,
  hint,
  children,
}: {
  request: FriendRequest
  hint: string
  children: ReactNode
}) {
  const name = friendRequestName(request)
  return (
    <div className="friend-request-row">
      <UserAvatar avatarUrl={request.avatarUrl} displayName={name} size={24} />
      <span className="friend-request-text">
        <span className="friend-request-name">{name}</span>
        <span className="friend-request-hint">{hint}</span>
      </span>
      {children}
    </div>
  )
}
