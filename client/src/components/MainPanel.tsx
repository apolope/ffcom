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
}

export function MainPanel({ channel, server, members, canModerateMessages }: MainPanelProps) {
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
