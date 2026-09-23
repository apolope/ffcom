import { useCallback, useState } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { useMuteShortcut } from '../hooks/useMuteShortcut'
import { usePushToTalk } from '../hooks/usePushToTalk'
import { useVoiceChannel } from '../hooks/useVoiceChannel'
import {
  getParticipantAudio,
  participantAudioOf,
  updateParticipantAudio,
  type ParticipantAudio,
} from '../lib/participantAudio'
import { formatShortcut } from '../lib/shortcut'
import { getVoicePrefs, setVoicePrefs, type VoicePrefs } from '../lib/voicePrefs'
import { MuteShortcutSetting } from './MuteShortcutSetting'
import { ParticipantVolumeControls } from './ParticipantVolumeControls'
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
  const setPushToTalkKey = useCallback(
    (pushToTalkKey: string | undefined) => updateVoicePrefs({ pushToTalkKey }),
    [updateVoicePrefs],
  )
  // Qual tecla está sendo gravada: enquanto grava, o atalho correspondente
  // fica desligado para a tecla antiga não disparar.
  const [recording, setRecording] = useState<'mute' | 'pushToTalk'>()
  const setRecordingMute = useCallback((on: boolean) => setRecording(on ? 'mute' : undefined), [])
  const setRecordingPushToTalk = useCallback((on: boolean) => setRecording(on ? 'pushToTalk' : undefined), [])
  const [participantAudio, setParticipantAudio] = useState(() => getParticipantAudio(accountSub))
  const changeParticipantAudio = useCallback(
    (memberId: string, change: Partial<ParticipantAudio>) => {
      setParticipantAudio((prev) => updateParticipantAudio(accountSub, prev, memberId, change))
    },
    [accountSub],
  )
  // No máximo um painel de volume aberto por vez, pela identity da pessoa.
  const [volumeOpenFor, setVolumeOpenFor] = useState<string>()
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
    setTalking,
    toggleCamera,
    toggleScreenShare,
  } = useVoiceChannel(
    serverBaseUrl,
    channel.id,
    accessToken!,
    voicePrefs.micToggleSound,
    participantAudio,
    voicePrefs.pushToTalk,
  )
  // Em push-to-talk o atalho de mutar fica desligado (o microfone é da tecla
  // de falar); no Electron isso também libera a combinação global.
  const { registerFailed: shortcutRegisterFailed } = useMuteShortcut(
    voicePrefs.muteShortcut,
    status === 'connected' && !voicePrefs.pushToTalk && recording === undefined,
    toggleMic,
  )
  const { buttonProps: pushToTalkButtonProps } = usePushToTalk(
    voicePrefs.pushToTalkKey,
    status === 'connected' && voicePrefs.pushToTalk && recording === undefined,
    setTalking,
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
            {participants.map((p) => {
              const audio = participantAudioOf(participantAudio, p.identity)
              const volumeOpen = !p.isLocal && volumeOpenFor === p.identity
              return (
                <li key={p.identity} className="voice-participant">
                  <div className="voice-participant-row">
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
                    {!p.isLocal && (
                      <>
                        {audio.muted ? (
                          <span className="voice-volume-badge">silenciado para você</span>
                        ) : (
                          audio.voice !== 1 && (
                            <span className="voice-volume-badge">{Math.round(audio.voice * 100)}%</span>
                          )
                        )}
                        <button
                          type="button"
                          className="voice-volume-toggle"
                          aria-expanded={volumeOpen}
                          aria-label={`Volume de ${p.name}`}
                          title="Volume"
                          onClick={() => setVolumeOpenFor(volumeOpen ? undefined : p.identity)}
                        >
                          {audio.muted ? '🔕' : '🔉'}
                        </button>
                      </>
                    )}
                  </div>
                  {volumeOpen && (
                    <ParticipantVolumeControls
                      name={p.name}
                      audio={audio}
                      screenShareAudio={p.screenShareAudio}
                      onChange={(change) => changeParticipantAudio(p.identity, change)}
                    />
                  )}
                </li>
              )
            })}
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
            {voicePrefs.pushToTalk ? (
              <button
                type="button"
                className={micEnabled ? 'voice-ptt-button talking' : 'voice-ptt-button'}
                aria-pressed={micEnabled}
                {...pushToTalkButtonProps}
              >
                {micEnabled ? 'Falando…' : 'Segure para falar'}
              </button>
            ) : (
              <button type="button" onClick={toggleMic}>
                {micEnabled ? 'Silenciar microfone' : 'Ativar microfone'}
              </button>
            )}
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
            <div className="voice-pref voice-mode" role="radiogroup" aria-label="Modo do microfone">
              <label className="voice-pref">
                <input
                  type="radio"
                  name="voice-mode"
                  checked={!voicePrefs.pushToTalk}
                  onChange={() => updateVoicePrefs({ pushToTalk: false })}
                />
                Microfone aberto
              </label>
              <label className="voice-pref">
                <input
                  type="radio"
                  name="voice-mode"
                  checked={voicePrefs.pushToTalk}
                  onChange={() => updateVoicePrefs({ pushToTalk: true })}
                />
                Apertar para falar
              </label>
            </div>
            {voicePrefs.pushToTalk ? (
              <>
                <MuteShortcutSetting
                  label="Tecla para falar"
                  allowSingleKey
                  shortcut={voicePrefs.pushToTalkKey}
                  onChange={setPushToTalkKey}
                  recording={recording === 'pushToTalk'}
                  onRecordingChange={setRecordingPushToTalk}
                />
                <span className="voice-prefs-note">
                  {voicePrefs.pushToTalkKey
                    ? `Segure ${formatShortcut(voicePrefs.pushToTalkKey)} ou o botão "Segure para falar". `
                    : 'Sem tecla definida, segure o botão "Segure para falar". '}
                  Só funciona com esta janela em foco{window.ffcomElectron ? ', também no app desktop' : ''}.
                </span>
              </>
            ) : (
              <>
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
                  recording={recording === 'mute'}
                  onRecordingChange={setRecordingMute}
                />
                {shortcutRegisterFailed && (
                  <span className="voice-shortcut-hint">
                    O atalho não pôde ser registrado: outro programa já usa essa combinação. Escolha outra.
                  </span>
                )}
                {!window.ffcomElectron && voicePrefs.muteShortcut && (
                  <span className="voice-prefs-note">No navegador o atalho só funciona com esta janela em foco.</span>
                )}
              </>
            )}
          </div>
        </>
      )}
    </div>
  )
}
