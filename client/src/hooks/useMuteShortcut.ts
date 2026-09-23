import { useEffect, useRef, useState } from 'react'
import { isTypingTarget, shortcutFromEvent } from '../lib/shortcut'

// Liga o atalho de mutar enquanto `active` (conectado na voz e fora do modo
// de gravar um atalho novo). No Electron o atalho é global, via
// globalShortcut no processo main, e funciona com o app em segundo plano; no
// web/PWA só existe keydown, então só vale com a janela em foco. Ver
// docs/architecture.md, "Decisão: atalho de teclado para mutar".
export function useMuteShortcut(
  shortcut: string | undefined,
  active: boolean,
  onTrigger: () => void,
): { registerFailed: boolean } {
  const onTriggerRef = useRef(onTrigger)
  useEffect(() => {
    onTriggerRef.current = onTrigger
  }, [onTrigger])
  const [registerFailed, setRegisterFailed] = useState(false)

  useEffect(() => {
    if (!active || !shortcut) return
    const bridge = window.ffcomElectron

    if (bridge) {
      // No Electron só o caminho global: o registro no sistema consome a
      // tecla, então um keydown aqui nunca chegaria e ouvir os dois só
      // arriscaria alternar duas vezes.
      let cancelled = false
      const unsubscribe = bridge.onMuteShortcut(() => onTriggerRef.current())
      bridge.setMuteShortcut(shortcut).then((ok) => {
        if (!cancelled) setRegisterFailed(!ok)
      })
      return () => {
        cancelled = true
        unsubscribe()
        bridge.setMuteShortcut(null)
        setRegisterFailed(false)
      }
    }

    const onKeyDown = (e: KeyboardEvent) => {
      // Segurar a tecla repete o keydown; sem isso o microfone piscaria.
      if (e.repeat || isTypingTarget(e.target)) return
      if (shortcutFromEvent(e) !== shortcut) return
      e.preventDefault()
      onTriggerRef.current()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [shortcut, active])

  return { registerFailed }
}
