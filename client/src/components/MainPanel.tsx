import type { Channel, ChannelType, KnownServer } from '../types'
import { ForumChannelView } from './ForumChannelView'
import { TextChannelView } from './TextChannelView'
import { VoiceChannelView } from './VoiceChannelView'
import { MobileMembersButton, MobileNavButton } from './MobileNavButtons'
import './MainPanel.css'

const CHANNEL_ICON: Record<ChannelType, string> = {
  text: '#',
  voice: '🔊',
  forum: '💬',
}

interface MainPanelProps {
  channel: Channel | undefined
  server: KnownServer
  canModerateMessages: boolean
}

export function MainPanel({ channel, server, canModerateMessages }: MainPanelProps) {
  const serverBaseUrl = server.baseUrl
  return (
    <section className="main-panel">
      <header className="channel-header">
        <MobileNavButton />
        {channel ? (
          <>
            <span className="channel-icon">{CHANNEL_ICON[channel.type]}</span>
            <span className="channel-title">{channel.name}</span>
          </>
        ) : (
          <span className="channel-title">Nenhum canal selecionado</span>
        )}
        <MobileMembersButton />
      </header>
      <div className="channel-content">
        {!channel && <p className="placeholder">Selecione um canal para começar.</p>}
        {channel?.type === 'text' && (
          <TextChannelView
            key={channel.id}
            serverBaseUrl={serverBaseUrl}
            channel={channel}
            canModerateMessages={canModerateMessages}
          />
        )}
        {channel?.type === 'voice' && (
          <VoiceChannelView key={channel.id} server={server} channel={channel} />
        )}
        {channel?.type === 'forum' && (
          <ForumChannelView key={channel.id} serverBaseUrl={serverBaseUrl} channel={channel} />
        )}
      </div>
    </section>
  )
}
