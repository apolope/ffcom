import type { KnownServer } from '../types'
import './ServerRail.css'

interface ServerRailProps {
  servers: KnownServer[]
  selectedServerId: string | undefined
  onSelectServer: (serverId: string) => void
  onAddServer: () => void
}

export function ServerRail({
  servers,
  selectedServerId,
  onSelectServer,
  onAddServer,
}: ServerRailProps) {
  return (
    <nav className="server-rail" aria-label="Servidores">
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
    </nav>
  )
}
