import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { participantAudioOf } from '../lib/participantAudio'
import { formatNumber } from '../lib/format'
import { formatShortcut } from '../lib/shortcut'
import { MuteShortcutSetting } from './MuteShortcutSetting'
import { ParticipantVolumeControls } from './ParticipantVolumeControls'
import { ScreenSharePicker } from './ScreenSharePicker'
import { useVoiceSession } from './VoiceSessionContext'
import type { Channel, KnownServer } from '../types'
import './VoiceChannelView.css'

interface VoiceChannelViewProps {
  server: KnownServer
  channel: Channel
}

// Canal de voz via LiveKit (ver docs/architecture.md, "Decisão: integração
// de voz com LiveKit"). Cada canal de voz é uma sala LiveKit própria; entrar
// pede um token novo em server-channel (hooks/useVoiceChannel.ts). A chamada
// em si é da sessão (VoiceSessionProvider): esta tela só a mostra quando é a
// deste canal, e sair dela não sai da chamada.
export function VoiceChannelView({ server, channel }: VoiceChannelViewProps) {
  const {
    target,
    status: sessionStatus,
    error,
    participants,
    micEnabled,
    cameraEnabled,
    cameraError,
    micError,
    screenSharing,
    screenShareAudio,
    audioPlaybackBlocked,
    noiseSuppressionError,
    startAudio,
    videoContainerRef,
    join: joinSession,
    leave,
    toggleMic,
    toggleCamera,
    toggleScreenShare,
    voicePrefs,
    updateVoicePrefs,
    participantAudio,
    changeParticipantAudio,
    recording,
    setRecording,
    shortcutRegisterFailed,
    pushToTalkButtonProps,
  } = useVoiceSession()
  const { t } = useTranslation()
  const here = target?.serverId === server.id && target.channelId === channel.id
  // Em outro canal, esta tela mostra só o convite para entrar (que troca de
  // canal).
  const status = here ? sessionStatus : 'idle'
  const inOtherChannel = !here && !!target && (sessionStatus === 'connected' || sessionStatus === 'connecting')
  const join = useCallback(
    () =>
      joinSession({
        serverId: server.id,
        serverName: server.name,
        baseUrl: server.baseUrl,
        channelId: channel.id,
        channelName: channel.name,
      }),
    [joinSession, server.id, server.name, server.baseUrl, channel.id, channel.name],
  )
  const setMuteShortcut = useCallback(
    (muteShortcut: string | undefined) => updateVoicePrefs({ muteShortcut }),
    [updateVoicePrefs],
  )
  const setPushToTalkKey = useCallback(
    (pushToTalkKey: string | undefined) => updateVoicePrefs({ pushToTalkKey }),
    [updateVoicePrefs],
  )
  const setRecordingMute = useCallback((on: boolean) => setRecording(on ? 'mute' : undefined), [setRecording])
  const setRecordingPushToTalk = useCallback(
    (on: boolean) => setRecording(on ? 'pushToTalk' : undefined),
    [setRecording],
  )
  // No app desktop, compartilhar abre o seletor próprio antes (o Electron não
  // tem o do navegador); parar continua direto.
  const electronBridge = window.ffcomElectron
  const [pickingScreen, setPickingScreen] = useState(false)
  // No máximo um painel de volume aberto por vez, pela identity da pessoa.
  const [volumeOpenFor, setVolumeOpenFor] = useState<string>()

  return (
    <div className="voice-channel">
      {status === 'idle' && (
        <div className="voice-channel-prompt">
          {inOtherChannel && target ? (
            <p>
              {target.serverId !== server.id
                ? t('voice.prompt.inOtherChannelServer', { channel: target.channelName, server: target.serverName })
                : t('voice.prompt.inOtherChannel', { channel: target.channelName })}
            </p>
          ) : (
            <p>{t('voice.prompt.empty', { channel: channel.name })}</p>
          )}
          <button type="button" onClick={join}>
            {t('voice.joinChannel')}
          </button>
        </div>
      )}

      {status === 'connecting' && <p className="placeholder">{t('voice.connecting')}</p>}

      {status === 'error' && (
        <div className="voice-channel-prompt">
          <p className="message-error">{error}</p>
          <button type="button" onClick={join}>
            {t('voice.retry')}
          </button>
        </div>
      )}

      {status === 'connected' && (
        <>
          {audioPlaybackBlocked && (
            <div className="voice-audio-blocked" role="alert">
              <span>{t('voice.audioBlocked')}</span>
              <button type="button" onClick={startAudio}>
                {t('voice.enableAudio')}
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
                    <span
                      className={
                        'voice-mic-icon' + (p.micEnabled ? '' : ' muted') + (p.speaking ? ' speaking' : '')
                      }
                    >
                      {p.micEnabled ? '🎤' : '🔇'}
                    </span>
                    <span>
                      {p.isLocal ? t('voice.selfName', { name: p.name }) : p.name}
                      {p.cameraEnabled ? ' 📷' : ''}
                      {p.screenSharing ? ' 🖥️' : ''}
                      {p.screenShareAudio ? ' 🔊' : ''}
                    </span>
                    {!p.isLocal && (
                      <>
                        {audio.muted ? (
                          <span className="voice-volume-badge">{t('voice.mutedForYou')}</span>
                        ) : (
                          audio.voice !== 1 && (
                            <span className="voice-volume-badge">
                              {formatNumber(audio.voice, { style: 'percent', maximumFractionDigits: 0 })}
                            </span>
                          )
                        )}
                        <button
                          type="button"
                          className="voice-volume-toggle"
                          aria-expanded={volumeOpen}
                          aria-label={t('voice.volumeOf', { name: p.name })}
                          title={t('voice.volume')}
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
          {micError && <p className="message-error voice-media-error">{micError}</p>}
          {cameraError && <p className="message-error voice-media-error">{cameraError}</p>}
          {screenSharing && !screenShareAudio && (
            <p className="voice-media-hint">
              {!electronBridge
                ? t('voice.screenNoAudio.browser')
                : electronBridge.canShareSystemAudio
                  ? t('voice.screenNoAudio.desktop')
                  : t('voice.screenNoAudio.unsupported')}
            </p>
          )}
          {pickingScreen && electronBridge && (
            <ScreenSharePicker
              bridge={electronBridge}
              onShare={toggleScreenShare}
              onClose={() => setPickingScreen(false)}
            />
          )}
          <div className="voice-controls">
            {voicePrefs.pushToTalk ? (
              <button
                type="button"
                className={micEnabled ? 'voice-ptt-button talking' : 'voice-ptt-button'}
                aria-pressed={micEnabled}
                {...pushToTalkButtonProps}
              >
                {micEnabled ? t('voice.talking') : t('voice.holdToTalk')}
              </button>
            ) : (
              <button type="button" onClick={toggleMic}>
                {micEnabled ? t('voice.muteMic') : t('voice.unmuteMic')}
              </button>
            )}
            <button type="button" onClick={toggleCamera}>
              {cameraEnabled ? t('voice.cameraOff') : t('voice.cameraOn')}
            </button>
            {/* Chrome no Android não tem getDisplayMedia: sem ele o botão só daria erro. */}
            {(electronBridge || screenSharing || !!navigator.mediaDevices?.getDisplayMedia) && (
              <button
                type="button"
                onClick={electronBridge && !screenSharing ? () => setPickingScreen(true) : toggleScreenShare}
              >
                {screenSharing ? t('voice.stopScreenShare') : t('voice.shareScreen')}
              </button>
            )}
            <button type="button" onClick={leave}>
              {t('voice.leaveChannel')}
            </button>
          </div>
          <div className="voice-prefs">
            <div className="voice-pref voice-mode" role="radiogroup" aria-label={t('voice.micMode')}>
              <label className="voice-pref">
                <input
                  type="radio"
                  name="voice-mode"
                  checked={!voicePrefs.pushToTalk}
                  onChange={() => updateVoicePrefs({ pushToTalk: false })}
                />
                {t('voice.openMic')}
              </label>
              <label className="voice-pref">
                <input
                  type="radio"
                  name="voice-mode"
                  checked={voicePrefs.pushToTalk}
                  onChange={() => updateVoicePrefs({ pushToTalk: true })}
                />
                {t('voice.pushToTalk')}
              </label>
            </div>
            <label className="voice-pref" title={t('voice.enhancedNoiseSuppressionHint')}>
              <input
                type="checkbox"
                checked={voicePrefs.enhancedNoiseSuppression}
                onChange={() => updateVoicePrefs({ enhancedNoiseSuppression: !voicePrefs.enhancedNoiseSuppression })}
              />
              {t('voice.enhancedNoiseSuppression')}
            </label>
            {noiseSuppressionError && <span className="voice-shortcut-hint">{noiseSuppressionError}</span>}
            <label className="voice-pref">
              <input
                type="checkbox"
                checked={voicePrefs.micToggleSound}
                onChange={() => updateVoicePrefs({ micToggleSound: !voicePrefs.micToggleSound })}
              />
              {voicePrefs.pushToTalk ? t('voice.soundOnPushToTalk') : t('voice.soundOnMute')}
            </label>
            <label className="voice-pref">
              <input
                type="checkbox"
                checked={voicePrefs.presenceSound}
                onChange={() => updateVoicePrefs({ presenceSound: !voicePrefs.presenceSound })}
              />
              {t('voice.soundOnJoinLeave')}
            </label>
            {voicePrefs.pushToTalk ? (
              <>
                <MuteShortcutSetting
                  label={t('voice.shortcut.pushToTalkLabel')}
                  allowSingleKey
                  shortcut={voicePrefs.pushToTalkKey}
                  onChange={setPushToTalkKey}
                  recording={recording === 'pushToTalk'}
                  onRecordingChange={setRecordingPushToTalk}
                />
                <span className="voice-prefs-note">
                  {voicePrefs.pushToTalkKey
                    ? t('voice.pushToTalkHint.withKey', { key: formatShortcut(voicePrefs.pushToTalkKey) })
                    : t('voice.pushToTalkHint.noKey')}{' '}
                  {window.ffcomElectron ? t('voice.pushToTalkHint.desktop') : t('voice.pushToTalkHint.browser')}
                </span>
              </>
            ) : (
              <>
                <MuteShortcutSetting
                  shortcut={voicePrefs.muteShortcut}
                  onChange={setMuteShortcut}
                  recording={recording === 'mute'}
                  onRecordingChange={setRecordingMute}
                />
                {shortcutRegisterFailed && (
                  <span className="voice-shortcut-hint">
                    {t('voice.shortcut.registerFailed')}
                  </span>
                )}
                {!window.ffcomElectron && voicePrefs.muteShortcut && (
                  <span className="voice-prefs-note">{t('voice.shortcut.browserFocusOnly')}</span>
                )}
              </>
            )}
          </div>
        </>
      )}
    </div>
  )
}
