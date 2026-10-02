import { createContext, useContext } from 'react'
import type { UseVoiceChannelResult } from '../hooks/useVoiceChannel'
import type { usePushToTalk } from '../hooks/usePushToTalk'
import type { ParticipantAudio, ParticipantAudioMap } from '../lib/participantAudio'
import type { VoicePrefs } from '../lib/voicePrefs'

// A chamada de voz da sessão (components/VoiceSessionProvider.tsx), para a
// tela do canal de voz e a barra "Conectado em" lerem a mesma chamada. Ver
// docs/architecture.md, "Decisão: chamada de voz continua ao navegar".
export interface VoiceSession extends UseVoiceChannelResult {
  voicePrefs: VoicePrefs
  updateVoicePrefs: (change: Partial<VoicePrefs>) => void
  participantAudio: ParticipantAudioMap
  changeParticipantAudio: (memberId: string, change: Partial<ParticipantAudio>) => void
  // Qual tecla está sendo gravada: enquanto grava, o atalho correspondente
  // fica desligado para a tecla antiga não disparar.
  recording: 'mute' | 'pushToTalk' | undefined
  setRecording: (recording: 'mute' | 'pushToTalk' | undefined) => void
  shortcutRegisterFailed: boolean
  pushToTalkButtonProps: ReturnType<typeof usePushToTalk>['buttonProps']
}

export const VoiceSessionContext = createContext<VoiceSession | null>(null)

export function useVoiceSession(): VoiceSession {
  const session = useContext(VoiceSessionContext)
  if (!session) throw new Error('useVoiceSession precisa ser usado dentro de <VoiceSessionProvider>')
  return session
}
