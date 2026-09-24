import { useEffect, useState } from 'react'
import { isVoiceConnected } from '../lib/voiceActivity'

// Depois de 10 minutos sem atividade, a pessoa conta como ociosa e, se o
// status escolhido for "online", os amigos a veem "ausente" (server-central
// decide, ver realtime.Hub). Ver docs/architecture.md, "Decisão: status de
// presença e avatar nas listas de membros".
export const IDLE_AFTER_MS = 10 * 60 * 1000
const CHECK_EVERY_MS = 30 * 1000

const ACTIVITY_EVENTS = ['pointermove', 'pointerdown', 'keydown', 'wheel', 'touchstart'] as const

// No web/PWA a atividade é a da página (mouse, teclado e toque nela); com a
// aba em segundo plano nada chega, então dez minutos fora dela contam como
// ociosidade. No Electron vale a do sistema inteiro (powerMonitor), para
// quem joga com o app em segundo plano continuar online. Em qualquer caso,
// estar conectado num canal de voz conta como atividade.
export function useIdle(): boolean {
  const [idle, setIdle] = useState(false)

  useEffect(() => {
    let lastActivity = Date.now()
    const markActive = () => {
      lastActivity = Date.now()
      setIdle(false)
    }
    const bridge = window.ffcomElectron
    if (!bridge) {
      ACTIVITY_EVENTS.forEach((type) => window.addEventListener(type, markActive, { passive: true }))
    }

    const check = async () => {
      if (isVoiceConnected()) {
        lastActivity = Date.now()
        setIdle(false)
        return
      }
      if (bridge) {
        const seconds = await bridge.getSystemIdleSeconds().catch(() => 0)
        setIdle(seconds * 1000 >= IDLE_AFTER_MS)
      } else {
        setIdle(Date.now() - lastActivity >= IDLE_AFTER_MS)
      }
    }
    const timer = window.setInterval(() => void check(), CHECK_EVERY_MS)

    return () => {
      window.clearInterval(timer)
      if (!bridge) ACTIVITY_EVENTS.forEach((type) => window.removeEventListener(type, markActive))
    }
  }, [])

  return idle
}
