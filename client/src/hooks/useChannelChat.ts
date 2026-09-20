import { useEffect, useRef, useState } from 'react'
import {
  decodeChannelSocketFrame,
  fetchChannelHistory,
  openChannelSocket,
  sendCreateMessage,
  type ChannelMessage,
} from '../lib/serverChannelApi'

export type ChatConnectionStatus = 'loading' | 'open' | 'closed' | 'error'

interface UseChannelChatResult {
  messages: ChannelMessage[]
  status: ChatConnectionStatus
  error: string | undefined
  sendMessage: (content: string) => void
}

// Carrega o histórico (REST) e mantém uma conexão WebSocket para um canal de
// texto de server-channel, reconectando do zero sempre que channelId muda.
export function useChannelChat(
  serverBaseUrl: string,
  channelId: string,
  accessToken: string,
): UseChannelChatResult {
  const [messages, setMessages] = useState<ChannelMessage[]>([])
  const [status, setStatus] = useState<ChatConnectionStatus>('loading')
  const [error, setError] = useState<string>()
  const socketRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    let cancelled = false
    setMessages([])
    setStatus('loading')
    setError(undefined)

    let socket: WebSocket | null = null

    fetchChannelHistory(serverBaseUrl, channelId, accessToken)
      .then((history) => {
        if (cancelled) return
        setMessages(history)

        socket = openChannelSocket(serverBaseUrl, channelId, accessToken)
        socketRef.current = socket

        socket.onopen = () => {
          if (!cancelled) setStatus('open')
        }
        socket.onclose = () => {
          if (!cancelled) setStatus('closed')
        }
        socket.onerror = () => {
          if (!cancelled) {
            setStatus('error')
            setError('conexão com o servidor foi interrompida')
          }
        }
        socket.onmessage = (event) => {
          const frame = decodeChannelSocketFrame(String(event.data))
          if (!frame) return
          if (frame.type === 'message.created') {
            setMessages((prev) => [...prev, frame.message])
          } else if (frame.type === 'error') {
            setError(frame.error)
          }
        }
      })
      .catch((err) => {
        if (cancelled) return
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar histórico')
      })

    return () => {
      cancelled = true
      socket?.close()
      socketRef.current = null
    }
  }, [serverBaseUrl, channelId, accessToken])

  function sendMessage(content: string) {
    const socket = socketRef.current
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    sendCreateMessage(socket, content)
  }

  return { messages, status, error, sendMessage }
}
