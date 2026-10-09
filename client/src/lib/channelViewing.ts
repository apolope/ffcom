import { isAndroidApp } from './platform'

// Se a pessoa está "vendo" os canais abertos nesta página agora: página
// visível, janela em foco e sem estar ausente (a mesma ociosidade do
// "ausente" automático, hooks/useIdle.ts, que o App repassa por
// setViewingIdle). Cada WebSocket de canal manda o estado ao server-channel
// no frame "channel.viewing", na conexão e a cada mudança, e o servidor só
// deixa de mandar push para quem está vendo de verdade: um canal aberto
// numa aba escondida ou no app minimizado não segura mais a notificação do
// celular. Ver docs/architecture.md, "Decisão: notificações push (fase 6)",
// revisão de 2026-10-09.
//
// Módulo em vez de contexto, como lib/voiceActivity.ts: quem lê são os
// sockets, que não renderizam com a mudança.

// Espera antes de avisar uma mudança: junta o pisca de foco (um diálogo do
// sistema, trocar de janela e voltar) num frame só.
const DEBOUNCE_MS = 500

type Listener = (active: boolean) => void

let idle = false
// No app Android, o estado do app (primeiro ou segundo plano) do Capacitor,
// além do visibilitychange da WebView.
let appActive = true
let reported = false
let timer: ReturnType<typeof setTimeout> | undefined
let installed = false
const listeners = new Set<Listener>()

function compute(): boolean {
  if (typeof document === 'undefined') return false
  return appActive && !idle && document.visibilityState === 'visible' && document.hasFocus()
}

function schedule() {
  clearTimeout(timer)
  timer = setTimeout(() => {
    timer = undefined
    const next = compute()
    if (next === reported) return
    reported = next
    listeners.forEach((listener) => listener(next))
  }, DEBOUNCE_MS)
}

function install() {
  if (installed || typeof window === 'undefined') return
  installed = true
  reported = compute()
  document.addEventListener('visibilitychange', schedule)
  window.addEventListener('focus', schedule)
  window.addEventListener('blur', schedule)
  window.addEventListener('pagehide', schedule)
  window.addEventListener('pageshow', schedule)
  if (isAndroidApp()) {
    void import('@capacitor/app')
      .then(({ App }) =>
        App.addListener('appStateChange', ({ isActive }) => {
          appActive = isActive
          schedule()
        }),
      )
      .catch((err) => console.warn('ffcom: sem estado do app para o push', err))
  }
}

// Repassado pelo App a partir de useIdle.
export function setViewingIdle(next: boolean): void {
  if (idle === next) return
  idle = next
  schedule()
}

// Estado atual, para mandar logo que um socket abre.
export function isViewing(): boolean {
  install()
  return reported
}

// Avisa as mudanças (já com debounce) até a função devolvida ser chamada.
export function subscribeViewing(listener: Listener): () => void {
  install()
  listeners.add(listener)
  return () => listeners.delete(listener)
}
