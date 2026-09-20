import type { Friend } from '../types'
import './FriendsView.css'

interface FriendsViewProps {
  friends: Friend[]
  onAddFriend: () => void
}

// Lista de amigos + presença (via server-central, ver hooks/useFriends.ts).
// Substitui o painel de canal quando o botão "Amigos" do ServerRail está
// selecionado — ainda não há DMs (ver TODO.md), então esta view só mostra a
// lista, sem abrir conversa ao clicar num amigo.
export function FriendsView({ friends, onAddFriend }: FriendsViewProps) {
  const online = friends.filter((f) => f.online)
  const offline = friends.filter((f) => !f.online)

  return (
    <div className="friends-view">
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
                <FriendRow key={friend.accountId} friend={friend} />
              ))}
            </div>
          )}
          {offline.length > 0 && (
            <div className="friends-group">
              <div className="friends-group-name">Offline — {offline.length}</div>
              {offline.map((friend) => (
                <FriendRow key={friend.accountId} friend={friend} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function FriendRow({ friend }: { friend: Friend }) {
  return (
    <div className={friend.online ? 'friend-row' : 'friend-row offline'}>
      <span className="friend-status" aria-hidden="true" />
      {friend.displayName}
    </div>
  )
}
