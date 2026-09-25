import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'

export type NotificationKind = 'info' | 'success' | 'error'

export interface AppNotification {
  id: number
  kind: NotificationKind
  text: string
}

export type Notify = (text: string, kind?: NotificationKind) => void

// Quanto tempo cada mensagem fica na tela.
export const NOTIFICATION_DURATION_MS = 5_000

// Central de mensagens do app (avisos curtos no canto da tela, ver
// components/NotificationStack.tsx). Várias podem estar na tela ao mesmo
// tempo: a nova entra embaixo, empurrando as mais antigas para cima, e cada
// uma some sozinha depois de NOTIFICATION_DURATION_MS.
//
// muted (status "Ocupado") descarta as mensagens em vez de guardar para
// depois: um aviso de 5s perde o sentido minutos mais tarde. As que já
// estavam na tela somem ao ativar.
export function useNotificationCenter(muted: boolean) {
  const [items, setItems] = useState<AppNotification[]>([])
  const nextIdRef = useRef(1)
  const timersRef = useRef(new Map<number, ReturnType<typeof setTimeout>>())
  const mutedRef = useRef(muted)
  useEffect(() => {
    mutedRef.current = muted
  }, [muted])

  const dismiss = useCallback((id: number) => {
    clearTimeout(timersRef.current.get(id))
    timersRef.current.delete(id)
    setItems((prev) => prev.filter((n) => n.id !== id))
  }, [])

  const notify = useCallback<Notify>(
    (text, kind = 'info') => {
      if (mutedRef.current) return
      const id = nextIdRef.current++
      setItems((prev) => [...prev, { id, kind, text }])
      timersRef.current.set(
        id,
        setTimeout(() => dismiss(id), NOTIFICATION_DURATION_MS),
      )
    },
    [dismiss],
  )

  useEffect(() => {
    const timers = timersRef.current
    return () => timers.forEach((timer) => clearTimeout(timer))
  }, [])

  // Com muted, o que estava na tela some junto (os timers terminam de
  // limpar a lista sozinhos).
  return { notifications: muted ? [] : items, notify, dismiss }
}

// Para componentes abaixo do App emitirem mensagens sem receber notify por
// prop. Fora do provider, notify não faz nada.
export const NotificationsContext = createContext<Notify>(() => {})

export function useNotify(): Notify {
  return useContext(NotificationsContext)
}
