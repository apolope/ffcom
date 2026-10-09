import { useTranslation } from 'react-i18next'
import type { VoiceTarget } from '../hooks/useVoiceChannel'
import { useVoiceSession } from './VoiceSessionContext'
import './VoiceConnectionBar.css'

interface VoiceConnectionBarProps {
  // Abre o canal da chamada (troca de servidor e sai de Amigos se preciso).
  onOpen: (target: VoiceTarget) => void
}

// "Conectado em 🔊 canal" no pé da coluna de canais, como no Discord: com a
// chamada em andamento e outra tela aberta, mostra onde a pessoa está, deixa
// mutar e sair sem voltar ao canal, e clicar no nome volta a ele. Ver
// docs/architecture.md, "Decisão: chamada de voz continua ao navegar".
export function VoiceConnectionBar({ onOpen }: VoiceConnectionBarProps) {
  const { target, status, reconnecting, micEnabled, toggleMic, leave, voicePrefs, audioPlaybackBlocked, startAudio } =
    useVoiceSession()
  const { t } = useTranslation()
  if (!target || (status !== 'connected' && status !== 'connecting')) return null
  const connected = status === 'connected'

  return (
    <div className="voice-connection-bar" role="status">
      <div className="voice-connection-info">
        <span className={connected && !reconnecting ? 'voice-connection-state connected' : 'voice-connection-state'}>
          {reconnecting ? t('voice.reconnecting') : connected ? t('voice.connected') : t('voice.connecting')}
        </span>
        <button
          type="button"
          className="voice-connection-channel"
          onClick={() => onOpen(target)}
          title={t('voice.openChannel')}
        >
          🔊 {target.channelName} · {target.serverName}
        </button>
      </div>
      {connected && audioPlaybackBlocked && (
        <button type="button" className="voice-connection-action" onClick={startAudio}>
          {t('voice.enableAudio')}
        </button>
      )}
      {/* No push-to-talk o microfone é da tecla de falar: alternar aqui o
          deixaria aberto. */}
      {connected && !voicePrefs.pushToTalk && (
        <button
          type="button"
          className="voice-connection-icon"
          onClick={toggleMic}
          aria-pressed={!micEnabled}
          aria-label={micEnabled ? t('voice.muteMic') : t('voice.unmuteMic')}
          title={micEnabled ? t('voice.muteMic') : t('voice.unmuteMic')}
        >
          {micEnabled ? '🎤' : '🔇'}
        </button>
      )}
      <button
        type="button"
        className="voice-connection-icon leave"
        onClick={leave}
        aria-label={t('voice.leaveChannel')}
        title={t('voice.leaveChannel')}
      >
        ✕
      </button>
    </div>
  )
}
