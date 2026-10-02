import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { useMuteShortcut } from '../hooks/useMuteShortcut'
import { usePushToTalk } from '../hooks/usePushToTalk'
import { useVoiceChannel } from '../hooks/useVoiceChannel'
import { getParticipantAudio, updateParticipantAudio, type ParticipantAudio } from '../lib/participantAudio'
import { getVoicePrefs, setVoicePrefs, type VoicePrefs } from '../lib/voicePrefs'
import { VoiceSessionContext, type VoiceSession } from './VoiceSessionContext'

interface VoiceSessionProviderProps {
  // Servidores da lista da conta: tirar da lista o servidor da chamada sai
  // dela.
  serverIds: string[]
  children: ReactNode
}

// Dono da chamada de voz enquanto a sessão estiver aberta: fica acima do
// canal selecionado, então abrir texto, fórum, outro servidor ou amigos não
// derruba a chamada (antes ela vivia em VoiceChannelView e caía com o
// LiveKit registrando CLIENT_REQUEST_LEAVE). Junto vêm as preferências de
// voz, os volumes por pessoa e os atalhos de mutar e de falar, que precisam
// funcionar com qualquer tela aberta. Ver docs/architecture.md, "Decisão:
// chamada de voz continua ao navegar".
export function VoiceSessionProvider({ serverIds, children }: VoiceSessionProviderProps) {
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
  const [recording, setRecording] = useState<'mute' | 'pushToTalk'>()
  const [participantAudio, setParticipantAudio] = useState(() => getParticipantAudio(accountSub))
  const changeParticipantAudio = useCallback(
    (memberId: string, change: Partial<ParticipantAudio>) => {
      setParticipantAudio((prev) => updateParticipantAudio(accountSub, prev, memberId, change))
    },
    [accountSub],
  )
  const voice = useVoiceChannel(
    accessToken!,
    voicePrefs.micToggleSound,
    participantAudio,
    voicePrefs.pushToTalk,
    voicePrefs.enhancedNoiseSuppression,
    voicePrefs.presenceSound,
  )
  // Em push-to-talk o atalho de mutar fica desligado (o microfone é da tecla
  // de falar); no Electron isso também libera a combinação global.
  const { registerFailed: shortcutRegisterFailed } = useMuteShortcut(
    voicePrefs.muteShortcut,
    voice.status === 'connected' && !voicePrefs.pushToTalk && recording === undefined,
    voice.toggleMic,
  )
  const { buttonProps: pushToTalkButtonProps } = usePushToTalk(
    voicePrefs.pushToTalkKey,
    voice.status === 'connected' && voicePrefs.pushToTalk && recording === undefined,
    voice.setTalking,
  )

  const { target, leave } = voice
  const targetServerGone = !!target && !serverIds.includes(target.serverId)
  useEffect(() => {
    if (targetServerGone) leave()
  }, [targetServerGone, leave])

  // useVoiceChannel e usePushToTalk devolvem objetos novos a cada render, e
  // quem lê o contexto (a tela do canal de voz e a barra) renderiza junto com
  // a chamada de qualquer forma: sem useMemo.
  const session: VoiceSession = {
    ...voice,
    voicePrefs,
    updateVoicePrefs,
    participantAudio,
    changeParticipantAudio,
    recording,
    setRecording,
    shortcutRegisterFailed,
    pushToTalkButtonProps,
  }

  return <VoiceSessionContext.Provider value={session}>{children}</VoiceSessionContext.Provider>
}
