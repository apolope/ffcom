import { useTranslation } from 'react-i18next'
import { useMemo } from 'react'
import type { Channel, ChannelType, KnownServer, Member } from '../types'
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
  members: Member[]
  canModerateMessages: boolean
  // Silenciar as notificações do canal aberto: só onde há push (app
  // Android), como o menu do canal na barra lateral. Lá o tocar e segurar
  // começa o arrastar para reordenar quando a pessoa pode reordenar, então
  // o sino no cabeçalho é o caminho que sempre funciona.
  notifications?: { muted: boolean; onToggle: () => void }
}

export function MainPanel({ channel, server, members, canModerateMessages, notifications }: MainPanelProps) {
  const { t } = useTranslation()
  const serverBaseUrl = server.baseUrl
  // Nome do autor nas mensagens: mesmo nome da lista de membros. Quem já
  // saiu do servidor não está mais nela e cai no começo do id.
  const authorName = useMemo(() => {
    const names = new Map(members.map((m) => [m.id, m.nickname]))
    return (memberId: string) => names.get(memberId) ?? memberId.slice(0, 8)
  }, [members])
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
          <span className="channel-title">{t('channels.noneSelected')}</span>
        )}
        {notifications && channel && channel.type !== 'voice' && (
          <button
            type="button"
            className="channel-mute-button"
            aria-pressed={notifications.muted}
            aria-label={notifications.muted ? t('push.mute.channelUnmute') : t('push.mute.channelMute')}
            title={notifications.muted ? t('push.mute.channelUnmute') : t('push.mute.channelMute')}
            onClick={notifications.onToggle}
          >
            {notifications.muted ? '🔕' : '🔔'}
          </button>
        )}
        <MobileMembersButton />
      </header>
      <div className="channel-content">
        {!channel && <p className="placeholder">{t('channels.selectPrompt')}</p>}
        {channel?.type === 'text' && (
          <TextChannelView
            key={channel.id}
            serverBaseUrl={serverBaseUrl}
            channel={channel}
            authorName={authorName}
            canModerateMessages={canModerateMessages}
          />
        )}
        {channel?.type === 'voice' && (
          <VoiceChannelView key={channel.id} server={server} channel={channel} />
        )}
        {channel?.type === 'forum' && (
          <ForumChannelView
            key={channel.id}
            serverBaseUrl={serverBaseUrl}
            channel={channel}
            authorName={authorName}
          />
        )}
      </div>
    </section>
  )
}
