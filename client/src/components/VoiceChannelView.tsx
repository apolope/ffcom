import { useCallback, useState } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { useMuteShortcut } from '../hooks/useMuteShortcut'
import { useVoiceChannel } from '../hooks/useVoiceChannel'
import { getVoicePrefs, setVoicePrefs, type VoicePrefs } from '../lib/voicePrefs'
import { MuteShortcutSetting } from './MuteShortcutSetting'
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
  const { accessToken, user } = useAuth()
  const accountSub = user?.profile.sub ?? ''
  const [voicePrefs, setVoicePrefsState] = useState(() => getVoicePrefs(accountSub))
  const updateVoicePrefs = useCallback(
    (change: Partial<VoicePrefs>) => {
      setVoicePrefsState((prev) => {
        const next = { ...prev, ...change }
        setVoicePrefs(accountSub, next)
        return next
      })
    },
    [accountSub],
  )
  const setMuteShortcut = useCallback(
    (muteShortcut: string | undefined) => updateVoicePrefs({ muteShortcut }),
    [updateVoicePrefs],
  )
  const [recordingShortcut, setRecordingShortcut] = useState(false)
  const {
    status,
    error,
    participants,
    micEnabled,
    cameraEnabled,
    cameraError,
    screenSharing,
    screenShareAudio,
    audioPlaybackBlocked,
    startAudio,
    videoContainerRef,
    join,
    leave,
    toggleMic,
    toggleCamera,
    toggleScreenShare,
  } = useVoiceChannel(serverBaseUrl, channel.id, accessToken!, voicePrefs.micToggleSound)
  const { registerFailed: shortcutRegisterFailed } = useMuteShortcut(
    voicePrefs.muteShortcut,
    status === 'connected' && !recordingShortcut,
    toggleMic,
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
          {audioPlaybackBlocked && (
            <div className="voice-audio-blocked" role="alert">
              <span>O navegador bloqueou o som da chamada.</span>
              <button type="button" onClick={startAudio}>
                Ativar som
              </button>
            </div>
          )}
          <div className="video-grid" ref={videoContainerRef} />
          <ul className="voice-participant-list">
            {participants.map((p) => (
              <li key={p.identity} className="voice-participant">
                <span className={p.micEnabled ? 'voice-mic-icon' : 'voice-mic-icon muted'}>
                  {p.micEnabled ? '🎤' : '🔇'}
                </span>
                <span>
                  {p.name}
                  {p.isLocal ? ' (você)' : ''}
                  {p.cameraEnabled ? ' 📷' : ''}
                  {p.screenSharing ? ' 🖥️' : ''}
                  {p.screenShareAudio ? ' 🔊' : ''}
                </span>
              </li>
            ))}
          </ul>
          {cameraError && <p className="message-error voice-media-error">{cameraError}</p>}
          {screenSharing && !screenShareAudio && (
            <p className="voice-media-hint">
              Compartilhando sem áudio. Para enviar o som, pare e compartilhe de novo marcando "Compartilhar
              áudio" no seletor (Chrome e Edge; em tela inteira, só no Windows). Firefox e Safari não enviam
              áudio de tela.
            </p>
          )}
          <div className="voice-controls">
            <button type="button" onClick={toggleMic}>
              {micEnabled ? 'Silenciar microfone' : 'Ativar microfone'}
            </button>
            <button type="button" onClick={toggleCamera}>
              {cameraEnabled ? 'Desligar câmera' : 'Ligar câmera'}
            </button>
            <button type="button" onClick={toggleScreenShare}>
              {screenSharing ? 'Parar compartilhamento' : 'Compartilhar tela'}
            </button>
            <button type="button" onClick={leave}>
              Sair do canal de voz
            </button>
          </div>
          <div className="voice-prefs">
            <label className="voice-pref">
              <input
                type="checkbox"
                checked={voicePrefs.micToggleSound}
                onChange={() => updateVoicePrefs({ micToggleSound: !voicePrefs.micToggleSound })}
              />
              Som ao mutar
            </label>
            <MuteShortcutSetting
              shortcut={voicePrefs.muteShortcut}
              onChange={setMuteShortcut}
              recording={recordingShortcut}
              onRecordingChange={setRecordingShortcut}
            />
            {shortcutRegisterFailed && (
              <span className="voice-shortcut-hint">
                O atalho não pôde ser registrado: outro programa já usa essa combinação. Escolha outra.
              </span>
            )}
            {!window.ffcomElectron && voicePrefs.muteShortcut && (
              <span className="voice-prefs-note">No navegador o atalho só funciona com esta janela em foco.</span>
            )}
          </div>
        </>
      )}
    </div>
  )
}
