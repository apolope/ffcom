// Volume por pessoa num canal de voz (voz e áudio da tela separados) e
// "silenciar para mim", guardados no dispositivo por conta (`sub` do OIDC) e
// por memberId (a identity da pessoa na sala LiveKit, UUID único), mesmo
// padrão de lib/unread.ts e lib/voicePrefs.ts: não sincroniza entre
// dispositivos e quem loga depois de outra pessoa no mesmo navegador não
// herda as escolhas dela. Ver docs/architecture.md, "Decisão: volume por
// pessoa no canal de voz".
const PREFIX = 'ffcom:participantAudio:v1:'

export const MIN_VOLUME = 0
export const MAX_VOLUME = 2
export const DEFAULT_VOLUME = 1

export interface ParticipantAudio {
  // Ganho da voz (microfone) e do áudio da tela, de 0 a 2 (0% a 200%).
  voice: number
  screen: number
  // Silencia a pessoa inteira (voz e tela) só para quem escolheu, sem perder
  // os volumes escolhidos.
  muted: boolean
}

export type ParticipantAudioMap = Record<string, ParticipantAudio>

export const DEFAULT_PARTICIPANT_AUDIO: ParticipantAudio = {
  voice: DEFAULT_VOLUME,
  screen: DEFAULT_VOLUME,
  muted: false,
}

function clampVolume(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return DEFAULT_VOLUME
  return Math.min(MAX_VOLUME, Math.max(MIN_VOLUME, value))
}

function isDefault(audio: ParticipantAudio): boolean {
  return audio.voice === DEFAULT_VOLUME && audio.screen === DEFAULT_VOLUME && !audio.muted
}

export function participantAudioOf(map: ParticipantAudioMap, memberId: string): ParticipantAudio {
  return map[memberId] ?? DEFAULT_PARTICIPANT_AUDIO
}

export function getParticipantAudio(accountSub: string): ParticipantAudioMap {
  if (!accountSub) return {}
  try {
    const raw = localStorage.getItem(PREFIX + accountSub)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as Record<string, Partial<ParticipantAudio>>
    const map: ParticipantAudioMap = {}
    for (const [memberId, entry] of Object.entries(parsed)) {
      if (!entry || typeof entry !== 'object') continue
      map[memberId] = {
        voice: clampVolume(entry.voice),
        screen: clampVolume(entry.screen),
        muted: entry.muted === true,
      }
    }
    return map
  } catch {
    return {}
  }
}

// Devolve o mapa novo com a mudança aplicada e já persistido; entradas que
// voltaram ao padrão saem do mapa para o localStorage não crescer à toa.
export function updateParticipantAudio(
  accountSub: string,
  map: ParticipantAudioMap,
  memberId: string,
  change: Partial<ParticipantAudio>,
): ParticipantAudioMap {
  const entry = { ...participantAudioOf(map, memberId), ...change }
  entry.voice = clampVolume(entry.voice)
  entry.screen = clampVolume(entry.screen)
  const next = { ...map }
  if (isDefault(entry)) {
    delete next[memberId]
  } else {
    next[memberId] = entry
  }
  if (accountSub) {
    try {
      localStorage.setItem(PREFIX + accountSub, JSON.stringify(next))
    } catch {
      // localStorage indisponível (aba anônima, quota, etc.): a escolha vale
      // só até recarregar a página.
    }
  }
  return next
}
