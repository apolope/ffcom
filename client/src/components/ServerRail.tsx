import type { KnownServer } from '../types'
import type { MyProfile } from '../lib/serverCentralApi'
import { UserAvatar } from './UserAvatar'
import './ServerRail.css'

interface ServerRailProps {
  servers: KnownServer[]
  selectedServerId: string | undefined
  friendsSelected: boolean
  myProfile: MyProfile | undefined
  onSelectServer: (serverId: string) => void
  onSelectFriends: () => void
  onAddServer: () => void
  onOpenMyAvatar: () => void
}

export function ServerRail({
  servers,
  selectedServerId,
  friendsSelected,
  myProfile,
  onSelectServer,
  onSelectFriends,
  onAddServer,
  onOpenMyAvatar,
}: ServerRailProps) {
  return (
    <nav className="server-rail" aria-label="Servidores">
      <button
        type="button"
        className={friendsSelected ? 'server-icon active' : 'server-icon'}
        onClick={onSelectFriends}
        title="Amigos"
      >
        <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true">
          <path d="M12 3 2 12h3v8h6v-6h2v6h6v-8h3L12 3z" />
        </svg>
      </button>
      <ul>
        {servers.map((server) => (
          <li key={server.id}>
            <button
              type="button"
              className={
                server.id === selectedServerId ? 'server-icon active' : 'server-icon'
              }
              onClick={() => onSelectServer(server.id)}
              title={server.name}
            >
              {server.initials}
            </button>
          </li>
        ))}
      </ul>
      <button
        type="button"
        className="server-icon add-server"
        title="Adicionar servidor"
        onClick={onAddServer}
      >
        +
      </button>
      <button type="button" className="account-button" title="Seu avatar" onClick={onOpenMyAvatar}>
        <UserAvatar
          avatarUrl={myProfile?.avatarUrl}
          displayName={myProfile?.displayName ?? myProfile?.oidcSubject ?? '?'}
          size={44}
        />
      </button>
    </nav>
  )
}
