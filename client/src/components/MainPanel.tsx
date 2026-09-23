import type { Channel, ChannelType } from '../types'
import { ForumChannelView } from './ForumChannelView'
import { TextChannelView } from './TextChannelView'
import { VoiceChannelView } from './VoiceChannelView'
import './MainPanel.css'

const CHANNEL_ICON: Record<ChannelType, string> = {
  text: '#',
  voice: '🔊',
  forum: '💬',
}

interface MainPanelProps {
  channel: Channel | undefined
  serverBaseUrl: string
  canModerateMessages: boolean
}

export function MainPanel({ channel, serverBaseUrl, canModerateMessages }: MainPanelProps) {
  return (
    <section className="main-panel">
      <header className="channel-header">
        {channel ? (
          <>
            <span className="channel-icon">{CHANNEL_ICON[channel.type]}</span>
            <span className="channel-title">{channel.name}</span>
          </>
        ) : (
          <span className="channel-title">Nenhum canal selecionado</span>
        )}
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
          <VoiceChannelView key={channel.id} serverBaseUrl={serverBaseUrl} channel={channel} />
        )}
        {channel?.type === 'forum' && (
          <ForumChannelView key={channel.id} serverBaseUrl={serverBaseUrl} channel={channel} />
        )}
      </div>
    </section>
  )
}
