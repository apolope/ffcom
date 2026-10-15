// Preferências de voz guardadas no dispositivo, por conta (`sub` do OIDC),
// mesmo padrão de lib/unread.ts: não sincronizam entre dispositivos e quem
// loga depois de outra pessoa no mesmo navegador não herda as escolhas dela.
import { isShortcutFormat } from './shortcut'

const PREFIX = 'ffcom:voicePrefs:v1:'

// Como o microfone abre e fecha:
// - manual: só pelo botão na tela (ou na barra de voz conectada);
// - pushToTalk ("automático"): aberto enquanto segura a tecla ou o botão
//   "Segure para falar", fechado ao soltar;
// - toggle ("semiautomático"): a tecla de muteShortcut fecha e abre, além
//   do botão.
export type MicMode = 'manual' | 'pushToTalk' | 'toggle'
const MIC_MODES: readonly MicMode[] = ['manual', 'pushToTalk', 'toggle']

export interface VoicePrefs {
  // Aviso sonoro ao mutar/desmutar e, no push-to-talk, ao apertar e soltar
  // (lib/micToggleSound.ts).
  micToggleSound: boolean
  // Aviso sonoro quando alguém entra ou sai da chamada, inclusive a própria
  // pessoa (lib/presenceSound.ts).
  presenceSound: boolean
  // Atalho de mutar/desmutar do modo toggle, no formato de lib/shortcut.ts;
  // ausente = sem atalho (padrão, para não roubar nenhuma combinação sem a
  // pessoa pedir). Fica guardado ao trocar de modo.
  muteShortcut?: string
  micMode: MicMode
  // Tecla do push-to-talk, mesmo formato; ausente = só o botão de segurar
  // na tela. Pode ser uma tecla sozinha, porque só vale com a janela em foco.
  pushToTalkKey?: string
  // "Supressão de ruído reforçada" (RNNoise, lib/rnnoiseProcessor.ts) em vez
  // da supressão do navegador. Desligada por padrão: ver docs/architecture.md,
  // "Decisão: supressão de ruído no microfone".
  enhancedNoiseSuppression: boolean
}

const DEFAULTS: VoicePrefs = {
  micToggleSound: true,
  presenceSound: true,
  micMode: 'manual',
  enhancedNoiseSuppression: false,
}

export function getVoicePrefs(accountSub: string): VoicePrefs {
  if (!accountSub) return DEFAULTS
  try {
    const raw = localStorage.getItem(PREFIX + accountSub)
    if (!raw) return DEFAULTS
    const stored = JSON.parse(raw) as Partial<VoicePrefs> & { pushToTalk?: unknown }
    const prefs: VoicePrefs = { ...DEFAULTS, ...stored }
    if (prefs.muteShortcut !== undefined && !isShortcutFormat(prefs.muteShortcut)) {
      delete prefs.muteShortcut
    }
    if (prefs.pushToTalkKey !== undefined && !isShortcutFormat(prefs.pushToTalkKey)) {
      delete prefs.pushToTalkKey
    }
    prefs.presenceSound = prefs.presenceSound !== false
    if (!MIC_MODES.includes(stored.micMode as MicMode)) {
      // Antes do micMode só existia o booleano pushToTalk, e o atalho de
      // mutar valia no microfone aberto: quem tinha atalho já usava o que
      // hoje é o modo toggle.
      prefs.micMode = stored.pushToTalk === true ? 'pushToTalk' : prefs.muteShortcut ? 'toggle' : 'manual'
    }
    delete (prefs as { pushToTalk?: unknown }).pushToTalk
    prefs.enhancedNoiseSuppression = prefs.enhancedNoiseSuppression === true
    return prefs
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
