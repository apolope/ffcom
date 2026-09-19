import type { Category, ChannelType, KnownServer } from '../types'
import './ChannelSidebar.css'

const CHANNEL_ICON: Record<ChannelType, string> = {
  text: '#',
  voice: '🔊',
  forum: '💬',
}

interface ChannelSidebarProps {
  server: KnownServer
  categories: Category[]
  selectedChannelId: string | undefined
  onSelectChannel: (channelId: string) => void
}

export function ChannelSidebar({
  server,
  categories,
  selectedChannelId,
  onSelectChannel,
}: ChannelSidebarProps) {
  return (
    <nav className="channel-sidebar" aria-label="Canais">
      <div className="server-name">{server.name}</div>
      <div className="category-list">
        {categories.map((category) => (
          <div className="category" key={category.id}>
            <div className="category-name">{category.name}</div>
            <ul>
              {category.channels.map((channel) => (
                <li key={channel.id}>
                  <button
                    type="button"
                    className={
                      channel.id === selectedChannelId
                        ? 'channel-item active'
                        : 'channel-item'
                    }
                    onClick={() => onSelectChannel(channel.id)}
                  >
                    <span className="channel-icon">
                      {CHANNEL_ICON[channel.type]}
                    </span>
                    {channel.name}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </nav>
  )
}
