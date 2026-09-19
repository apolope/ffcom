import type { Channel, ChannelType } from '../types'
import './MainPanel.css'

const CHANNEL_ICON: Record<ChannelType, string> = {
  text: '#',
  voice: '🔊',
  forum: '💬',
}

interface MainPanelProps {
  channel: Channel | undefined
}

export function MainPanel({ channel }: MainPanelProps) {
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
        {channel ? (
          <p className="placeholder">
            Conteúdo de "{channel.name}" ainda não implementado.
          </p>
        ) : (
          <p className="placeholder">Selecione um canal para começar.</p>
        )}
      </div>
    </section>
  )
}
