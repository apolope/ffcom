import type { Friend } from '../types'
import './FriendsView.css'

interface FriendsViewProps {
  friends: Friend[]
  selectedFriendId: string | undefined
  onSelectFriend: (accountId: string) => void
  onAddFriend: () => void
}

// Lista de amigos + presença (via server-central, ver hooks/useFriends.ts).
// Ocupa a coluna de "canais" quando o botão "Amigos" do ServerRail está
// selecionado; clicar num amigo abre a conversa de DM correspondente (ver
// components/DirectMessageView.tsx e docs/architecture.md, "Decisão: modelo
// de DMs").
export function FriendsView({ friends, selectedFriendId, onSelectFriend, onAddFriend }: FriendsViewProps) {
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
  onSelect,
}: {
  friend: Friend
  active: boolean
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
      <span className="friend-status" aria-hidden="true" />
      {friend.displayName}
    </button>
  )
}
