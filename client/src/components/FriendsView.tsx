import { useEffect, useRef, useState, type ReactNode } from 'react'
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

  return (
    <nav className="friends-view" aria-label="Amigos">
      <header className="friends-header">
        <h1>Amigos</h1>
        <button type="button" onClick={onAddFriend}>
          Adicionar amigo
        </button>
      </header>

      {(incomingRequests.length > 0 || outgoingRequests.length > 0) && (
        <div className="friends-list friends-requests">
          {incomingRequests.length > 0 && (
            <div className="friends-group">
              <div className="friends-group-name">Pedidos recebidos — {incomingRequests.length}</div>
              {incomingRequests.map((request) => (
                <FriendRequestRow key={request.id} request={request} hint="Quer ser seu amigo">
                  <button
                    type="button"
                    className="friend-request-action accept"
                    title="Aceitar"
                    aria-label={`Aceitar pedido de ${friendRequestName(request)}`}
                    onClick={() => onAcceptRequest(request.id)}
                  >
                    ✓
                  </button>
                  <button
                    type="button"
                    className="friend-request-action decline"
                    title="Recusar"
                    aria-label={`Recusar pedido de ${friendRequestName(request)}`}
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
              <div className="friends-group-name">Pedidos enviados — {outgoingRequests.length}</div>
              {outgoingRequests.map((request) => (
                <FriendRequestRow key={request.id} request={request} hint="Aguardando resposta">
                  <button
                    type="button"
                    className="friend-request-action decline"
                    title="Cancelar pedido"
                    aria-label={`Cancelar pedido para ${friendRequestName(request)}`}
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
          <p>Você ainda não tem amigos adicionados. Use "Adicionar amigo" para gerar ou resgatar um convite.</p>
        </div>
      ) : (
        <div className="friends-list">
          {online.length > 0 && (
            <div className="friends-group">
              <div className="friends-group-name">Online — {online.length}</div>
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
              <div className="friends-group-name">Offline — {offline.length}</div>
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
      {unread && <span className="unread-dot" aria-label="mensagens não lidas" />}
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
  const [error, setError] = useState<string>()
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
      setError(err instanceof Error ? err.message : 'falha ao remover amigo')
      setBusy(false)
    }
  }

  return (
    <div
      ref={ref}
      className="rail-menu friend-menu"
      role="menu"
      aria-label={`Ações para ${friend.displayName}`}
      style={{
        left: Math.max(8, Math.min(x, window.innerWidth - MENU_WIDTH)),
        top: Math.max(8, Math.min(y, window.innerHeight - MENU_HEIGHT)),
      }}
    >
      {confirming ? (
        <>
          <span className="rail-menu-empty">
            Remover {friend.displayName} dos amigos? As mensagens ficam guardadas, mas só voltam a aparecer se vocês
            forem amigos de novo.
          </span>
          {error && <span className="friend-menu-error">{error}</span>}
          <button
            type="button"
            role="menuitem"
            className="rail-menu-item friend-menu-danger"
            disabled={busy}
            onClick={() => void remove()}
          >
            {busy ? 'Removendo…' : 'Confirmar remoção'}
          </button>
          <button type="button" role="menuitem" className="rail-menu-item" disabled={busy} onClick={onClose}>
            Cancelar
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
            Enviar mensagem
          </button>
          <button
            type="button"
            role="menuitem"
            className="rail-menu-item friend-menu-danger"
            onClick={() => setConfirming(true)}
          >
            Remover amigo
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
