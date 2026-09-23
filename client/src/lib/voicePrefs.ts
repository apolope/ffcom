// Preferências de voz guardadas no dispositivo, por conta (`sub` do OIDC),
// mesmo padrão de lib/unread.ts: não sincronizam entre dispositivos e quem
// loga depois de outra pessoa no mesmo navegador não herda as escolhas dela.
const PREFIX = 'ffcom:voicePrefs:v1:'

export interface VoicePrefs {
  // Aviso sonoro ao mutar/desmutar (lib/micToggleSound.ts).
  micToggleSound: boolean
}

const DEFAULTS: VoicePrefs = {
  micToggleSound: true,
}

export function getVoicePrefs(accountSub: string): VoicePrefs {
  if (!accountSub) return DEFAULTS
  try {
    const raw = localStorage.getItem(PREFIX + accountSub)
    if (!raw) return DEFAULTS
    return { ...DEFAULTS, ...(JSON.parse(raw) as Partial<VoicePrefs>) }
  } catch {
    return DEFAULTS
  }
}

export function setVoicePrefs(accountSub: string, prefs: VoicePrefs): void {
  if (!accountSub) return
  try {
    localStorage.setItem(PREFIX + accountSub, JSON.stringify(prefs))
  } catch {
    // localStorage indisponível (aba anônima, quota, etc.): a escolha vale
    // só até recarregar a página.
  }
}
