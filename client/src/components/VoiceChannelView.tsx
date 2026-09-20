import { useAuth } from '../auth/AuthProvider'
import { useVoiceChannel } from '../hooks/useVoiceChannel'
import type { Channel } from '../types'
import './VoiceChannelView.css'

interface VoiceChannelViewProps {
  serverBaseUrl: string
  channel: Channel
}

// Canal de voz via LiveKit (ver docs/architecture.md, "Decisão: integração
// de voz com LiveKit"). Cada canal de voz é uma sala LiveKit própria; entrar
// pede um token novo em server-channel (hooks/useVoiceChannel.ts).
export function VoiceChannelView({ serverBaseUrl, channel }: VoiceChannelViewProps) {
  const { accessToken } = useAuth()
  const { status, error, participants, micEnabled, join, leave, toggleMic } = useVoiceChannel(
    serverBaseUrl,
    channel.id,
    accessToken!,
  )

  return (
    <div className="voice-channel">
      {status === 'idle' && (
        <div className="voice-channel-prompt">
          <p>Ninguém conectado ainda em "{channel.name}".</p>
          <button type="button" onClick={join}>
            Entrar no canal de voz
          </button>
        </div>
      )}

      {status === 'connecting' && <p className="placeholder">Conectando…</p>}

      {status === 'error' && (
        <div className="voice-channel-prompt">
          <p className="message-error">{error}</p>
          <button type="button" onClick={join}>
            Tentar novamente
          </button>
        </div>
      )}

      {status === 'connected' && (
        <>
          <ul className="voice-participant-list">
            {participants.map((p) => (
              <li key={p.identity} className="voice-participant">
                <span className={p.micEnabled ? 'voice-mic-icon' : 'voice-mic-icon muted'}>
                  {p.micEnabled ? '🎤' : '🔇'}
                </span>
                <span>
                  {p.name}
                  {p.isLocal ? ' (você)' : ''}
                </span>
              </li>
            ))}
          </ul>
          <div className="voice-controls">
            <button type="button" onClick={toggleMic}>
              {micEnabled ? 'Silenciar microfone' : 'Ativar microfone'}
            </button>
            <button type="button" onClick={leave}>
              Sair do canal de voz
            </button>
          </div>
        </>
      )}
    </div>
  )
}
