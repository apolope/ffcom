import { useEffect, useRef, useState } from 'react'
import i18n from '../i18n'
import { LocalizedError, problemFromFrame, useErrorText, type DisplayError } from '../lib/apiError'
import { isViewing, subscribeViewing } from '../lib/channelViewing'
import { createReconnectingSocket, type ChannelConnectionStatus, type ReconnectingSocket } from '../lib/reconnectingSocket'
import {
  decodeChannelSocketFrame,
  fetchChannelHistory,
  mergeChannelHistory,
  openChannelSocket,
  sendChannelViewing,
  sendCreateMessage,
  sendDeleteMessage,
  sendMessageWithAttachment,
  sendUpdateMessage,
  type ChannelMessage,
} from '../lib/serverChannelApi'

export type ChatConnectionStatus = ChannelConnectionStatus

const HISTORY_LIMIT = 50

interface UseChannelChatResult {
  messages: ChannelMessage[]
  status: ChatConnectionStatus
  error: string | undefined
  sendMessage: (content: string) => void
  sendMessageWithFile: (content: string, file: File) => Promise<void>
  editMessage: (id: string, content: string) => void
  deleteMessage: (id: string) => void
}

// Carrega o histórico (REST) e mantém uma conexão WebSocket para um canal de
// texto de server-channel, reconectando do zero sempre que channelId muda.
// Se a conexão cai (ex. troca de versão do servidor), reconecta com backoff
// e recarrega o histórico mesclando com o que já está na tela (ver
// lib/reconnectingSocket.ts).
export function useChannelChat(
  serverBaseUrl: string,
  channelId: string,
  accessToken: string,
): UseChannelChatResult {
  const [messages, setMessages] = useState<ChannelMessage[]>([])
  const [status, setStatus] = useState<ChatConnectionStatus>('loading')
  const [error, setError] = useState<DisplayError>()
  const connectionRef = useRef<ReconnectingSocket | null>(null)

  // O token fica fora das dependências do efeito da conexão: renovar o
  // token não derruba um socket aberto (o servidor só confere no
  // handshake), e cada reconexão lê daqui o token mais recente.
  const accessTokenRef = useRef(accessToken)
  useEffect(() => {
    accessTokenRef.current = accessToken
  }, [accessToken])

  useEffect(() => {
    let cancelled = false
    setMessages([])
    setStatus('loading')
    setError(undefined)

    const connection = createReconnectingSocket({
      label: 'canal de texto',
      failureMessage: () => i18n.t('chat.connectFailed'),
      connect: () => openChannelSocket(serverBaseUrl, channelId, accessTokenRef.current),
      // Push: o servidor só poupa quem está vendo o canal (lib/channelViewing.ts).
      onOpen: (ws) => sendChannelViewing(ws, isViewing()),
      sync: async () => {
        const history = await fetchChannelHistory(serverBaseUrl, channelId, accessTokenRef.current, HISTORY_LIMIT)
        if (!cancelled) setMessages((prev) => mergeChannelHistory(prev, history, HISTORY_LIMIT))
      },
      onStatus: (next, err) => {
        if (cancelled) return
        setStatus(next)
        if (err) setError(err)
      },
      onMessage: (data) => {
        const frame = decodeChannelSocketFrame(data)
        if (!frame) return
        if (frame.type === 'message.created') {
          // Pode já ter vindo pelo histórico recarregado numa reconexão.
          setMessages((prev) => (prev.some((m) => m.id === frame.message.id) ? prev : [...prev, frame.message]))
        } else if (frame.type === 'message.updated') {
          setMessages((prev) =>
            prev.map((m) => (m.id === frame.message.id ? frame.message : m)),
          )
        } else if (frame.type === 'message.deleted') {
          setMessages((prev) => prev.filter((m) => m.id !== frame.id))
        } else if (frame.type === 'error') {
          setError(problemFromFrame(frame))
        }
      },
    })
    connectionRef.current = connection
    const stopViewing = subscribeViewing((active) => {
      const socket = connection.current()
      if (socket) sendChannelViewing(socket, active)
    })

    return () => {
      cancelled = true
      stopViewing()
      connection.cancel()
      connectionRef.current = null
    }
  }, [serverBaseUrl, channelId])

  function sendMessage(content: string) {
    const socket = connectionRef.current?.current()
    if (!socket) return
    sendCreateMessage(socket, content)
  }

  // Vai por REST (não pelo socket) porque o handshake de WebSocket não tem
  // como carregar um arquivo multipart — ver docs/architecture.md, "Decisão:
  // upload de anexo em mensagem". A mensagem enviada chega de volta pelo
  // broadcast do socket já aberto, então não precisa (e não deve) ser
  // adicionada aqui também.
  async function sendMessageWithFile(content: string, file: File) {
    try {
      await sendMessageWithAttachment(serverBaseUrl, channelId, accessToken, content, file)
    } catch (err) {
      setError(err instanceof Error ? err : new LocalizedError(() => i18n.t('chat.attachmentSendFailed')))
    }
  }

  function editMessage(id: string, content: string) {
    const socket = connectionRef.current?.current()
    if (!socket) return
    sendUpdateMessage(socket, id, content)
  }

  function deleteMessage(id: string) {
    const socket = connectionRef.current?.current()
    if (!socket) return
    sendDeleteMessage(socket, id)
  }

  const errorText = useErrorText(error)

  return { messages, status, error: errorText, sendMessage, sendMessageWithFile, editMessage, deleteMessage }
}
