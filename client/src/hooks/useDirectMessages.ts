import { useCallback, useEffect, useState } from 'react'
import {
  decodePresenceSocketFrame,
  fetchDirectMessages,
  sendDirectMessageFrame,
  type RemoteDirectMessage,
} from '../lib/serverCentralApi'

export type DirectMessagesStatus = 'loading' | 'ready' | 'error'

interface UseDirectMessagesResult {
  messages: RemoteDirectMessage[]
  status: DirectMessagesStatus
  error: string | undefined
  sendMessage: (content: string) => void
}

// Conversa de DM com um amigo (peerId). Não abre conexão própria: recebe o
// WebSocket de presença já mantido por useFriends e anexa um listener extra
// nele (ver docs/architecture.md, "Decisão: DMs entregues no mesmo
// WebSocket de presença", e hooks/useFriends.ts).
export function useDirectMessages(
  accessToken: string,
  peerId: string | undefined,
  socket: WebSocket | null,
): UseDirectMessagesResult {
  const [messages, setMessages] = useState<RemoteDirectMessage[]>([])
  const [status, setStatus] = useState<DirectMessagesStatus>('loading')
  const [error, setError] = useState<string>()

  useEffect(() => {
    if (!peerId) return
    let cancelled = false
    setMessages([])
    setStatus('loading')
    setError(undefined)

    fetchDirectMessages(accessToken, peerId)
      .then((history) => {
        if (cancelled) return
        setMessages(history)
        setStatus('ready')
      })
      .catch((err) => {
        if (cancelled) return
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar conversa')
      })

    return () => {
      cancelled = true
    }
  }, [accessToken, peerId])

  useEffect(() => {
    if (!socket || !peerId) return

    function handleMessage(event: MessageEvent) {
      const frame = decodePresenceSocketFrame(String(event.data))
      if (!frame) return
      if (frame.type === 'dm.created') {
        const m = frame.message
        if (m.senderId !== peerId && m.recipientId !== peerId) return
        setMessages((prev) => [...prev, m])
      } else if (frame.type === 'error') {
        setError(frame.error)
      }
    }

    socket.addEventListener('message', handleMessage)
    return () => socket.removeEventListener('message', handleMessage)
  }, [socket, peerId])

  const sendMessage = useCallback(
    (content: string) => {
      if (!socket || socket.readyState !== WebSocket.OPEN || !peerId) return
      sendDirectMessageFrame(socket, peerId, content)
    },
    [socket, peerId],
  )

  return { messages, status, error, sendMessage }
}
