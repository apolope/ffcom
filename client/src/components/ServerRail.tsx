import type { KnownServer } from '../types'
import './ServerRail.css'

interface ServerRailProps {
  servers: KnownServer[]
  selectedServerId: string
  onSelectServer: (serverId: string) => void
}

export function ServerRail({
  servers,
  selectedServerId,
  onSelectServer,
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
      >
        +
      </button>
    </nav>
  )
}
