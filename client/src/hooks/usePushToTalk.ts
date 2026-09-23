import { useCallback, useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent } from 'react'
import { isTypingTarget, keyFromCode, shortcutFromEvent, shortcutMainKey } from '../lib/shortcut'

// Atraso ao soltar antes de fechar o microfone, para não cortar a última
// sílaba (a pessoa costuma soltar a tecla junto com o fim da fala).
const RELEASE_DELAY_MS = 200

// De onde vem o "segurar": a tecla configurada, o botão na tela pelo
// ponteiro (mouse ou toque) ou o botão na tela pelo teclado (Espaço/Enter com
// ele em foco). O microfone fica aberto enquanto qualquer um estiver
// segurando, então soltar um não fecha se outro continua apertado.
type Holder = 'key' | 'pointer' | 'buttonKey'

interface PushToTalkButtonProps {
  onPointerDown: (e: PointerEvent<HTMLButtonElement>) => void
  onPointerUp: () => void
  onPointerCancel: () => void
  onKeyDown: (e: ReactKeyboardEvent<HTMLButtonElement>) => void
  onKeyUp: (e: ReactKeyboardEvent<HTMLButtonElement>) => void
  onContextMenu: (e: { preventDefault: () => void }) => void
}

// Push-to-talk enquanto `active` (conectado na voz, modo "apertar para
// falar" e fora do modo de gravar tecla): chama onTalkingChange(true) ao
// apertar e onTalkingChange(false) ao soltar, com RELEASE_DELAY_MS de atraso.
// Só funciona com a janela em foco, no web e no Electron: o globalShortcut do
// Electron não entrega o soltar da tecla. Ver docs/architecture.md, "Decisão:
// push-to-talk".
export function usePushToTalk(
  key: string | undefined,
  active: boolean,
  onTalkingChange: (talking: boolean) => void,
): { buttonProps: PushToTalkButtonProps } {
  const onTalkingChangeRef = useRef(onTalkingChange)
  useEffect(() => {
    onTalkingChangeRef.current = onTalkingChange
  }, [onTalkingChange])
  const holdersRef = useRef(new Set<Holder>())
  const talkingRef = useRef(false)
  const releaseTimerRef = useRef<number | undefined>(undefined)
  const activeRef = useRef(active)

  const clearReleaseTimer = () => {
    if (releaseTimerRef.current !== undefined) {
      window.clearTimeout(releaseTimerRef.current)
      releaseTimerRef.current = undefined
    }
  }

  const press = useCallback((holder: Holder) => {
    if (!activeRef.current) return
    holdersRef.current.add(holder)
    clearReleaseTimer()
    if (!talkingRef.current) {
      talkingRef.current = true
      onTalkingChangeRef.current(true)
    }
  }, [])

  // immediate: perder o foco com a tecla apertada fecha na hora, sem o
  // atraso, porque o keyup nunca vai chegar e o microfone ficaria preso.
  const release = useCallback((holder: Holder | 'all', immediate = false) => {
    if (holder === 'all') {
      holdersRef.current.clear()
    } else {
      holdersRef.current.delete(holder)
    }
    if (holdersRef.current.size > 0 || !talkingRef.current) return
    const close = () => {
      releaseTimerRef.current = undefined
      if (holdersRef.current.size > 0 || !talkingRef.current) return
      talkingRef.current = false
      onTalkingChangeRef.current(false)
    }
    clearReleaseTimer()
    if (immediate) {
      close()
    } else {
      releaseTimerRef.current = window.setTimeout(close, RELEASE_DELAY_MS)
    }
  }, [])

  useEffect(() => {
    activeRef.current = active
    if (!active) release('all', true)
  }, [active, release])

  useEffect(() => {
    if (!active) return
    const mainKey = key ? shortcutMainKey(key) : undefined
    const onKeyDown = (e: KeyboardEvent) => {
      if (!key || isTypingTarget(e.target) || shortcutFromEvent(e) !== key) return
      e.preventDefault()
      // Segurar a tecla repete o keydown; o microfone já está aberto.
      if (!e.repeat) press('key')
    }
    const onKeyUp = (e: KeyboardEvent) => {
      if (mainKey && keyFromCode(e.code) === mainKey) release('key')
    }
    const releaseAll = () => release('all', true)
    const onVisibilityChange = () => {
      if (document.visibilityState === 'hidden') releaseAll()
    }
    window.addEventListener('keydown', onKeyDown)
    window.addEventListener('keyup', onKeyUp)
    window.addEventListener('blur', releaseAll)
    document.addEventListener('visibilitychange', onVisibilityChange)
    return () => {
      window.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('keyup', onKeyUp)
      window.removeEventListener('blur', releaseAll)
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  }, [key, active, press, release])

  useEffect(() => () => clearReleaseTimer(), [])

  const buttonProps: PushToTalkButtonProps = {
    onPointerDown: (e) => {
      if (e.button !== 0) return
      // Captura: arrastar o dedo para fora do botão não solta.
      e.currentTarget.setPointerCapture(e.pointerId)
      press('pointer')
    },
    onPointerUp: () => release('pointer'),
    onPointerCancel: () => release('pointer', true),
    onKeyDown: (e) => {
      if (e.key !== ' ' && e.key !== 'Enter') return
      e.preventDefault()
      if (!e.repeat) press('buttonKey')
    },
    onKeyUp: (e) => {
      if (e.key === ' ' || e.key === 'Enter') release('buttonKey')
    },
    // Segurar o dedo no celular abriria o menu de contexto.
    onContextMenu: (e) => e.preventDefault(),
  }

  return { buttonProps }
}
