import type { ReactNode } from 'react'
import { friendRequestName, type FriendRequest } from '../lib/serverCentralApi'
import type { Friend } from '../types'
import { AvatarWithStatus } from './AvatarWithStatus'
import { UserAvatar } from './UserAvatar'
import './FriendsView.css'

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
}: FriendsViewProps) {
  const online = friends.filter((f) => f.online)
  const offline = friends.filter((f) => !f.online)

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
                />
              ))}
            </div>
          )}
        </div>
      )}
    </nav>
  )
}

function FriendRow({
  friend,
  active,
  unread,
  onSelect,
}: {
  friend: Friend
  active: boolean
  unread: boolean
  onSelect: (accountId: string) => void
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
