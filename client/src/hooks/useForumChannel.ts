import { useEffect, useRef, useState } from 'react'
import i18n from '../i18n'
import { LocalizedError, problemFromFrame, useErrorText, type DisplayError } from '../lib/apiError'
import { isViewing, subscribeViewing } from '../lib/channelViewing'
import { createReconnectingSocket, type ChannelConnectionStatus, type ReconnectingSocket } from '../lib/reconnectingSocket'
import {
  decodeChannelSocketFrame,
  fetchForumThreads,
  fetchThreadMessages,
  mergeChannelHistory,
  mergeForumThreads,
  openChannelSocket,
  sendChannelViewing,
  sendCreatePost,
  sendCreateThread,
  type ChannelMessage,
  type RemoteThread,
} from '../lib/serverChannelApi'

export type ForumConnectionStatus = ChannelConnectionStatus

const POSTS_LIMIT = 50

interface UseForumChannelResult {
  threads: RemoteThread[]
  status: ForumConnectionStatus
  error: string | undefined
  activeThreadId: string | undefined
  posts: ChannelMessage[]
  postsLoading: boolean
  openThread: (threadId: string | undefined) => void
  createThread: (title: string, content: string) => void
  createPost: (content: string) => void
}

// Carrega as threads (REST) e mantém uma conexão WebSocket para um canal
// forum de server-channel — mesma rota/hub de canal de texto (ver
// docs/architecture.md, "Canal forum: threads/posts"), reconectando do zero
// sempre que channelId muda. Posts de uma thread só são carregados quando
// ela é aberta (openThread), via REST; novos posts da thread aberta chegam
// ao vivo pela mesma conexão. Se a conexão cai (ex. troca de versão do
// servidor), reconecta com backoff e recarrega as threads e os posts da
// thread aberta, mesclando com o que já está na tela (ver
// lib/reconnectingSocket.ts).
export function useForumChannel(
  serverBaseUrl: string,
  channelId: string,
  accessToken: string,
): UseForumChannelResult {
  const [threads, setThreads] = useState<RemoteThread[]>([])
  const [status, setStatus] = useState<ForumConnectionStatus>('loading')
  const [error, setError] = useState<DisplayError>()
  const [activeThreadId, setActiveThreadId] = useState<string>()
  const [posts, setPosts] = useState<ChannelMessage[]>([])
  const [postsLoading, setPostsLoading] = useState(false)

  const connectionRef = useRef<ReconnectingSocket | null>(null)
  const activeThreadIdRef = useRef<string | undefined>(undefined)
  useEffect(() => {
    activeThreadIdRef.current = activeThreadId
  }, [activeThreadId])

  // Mesmo tratamento de useChannelChat: renovar o token não derruba o
  // socket aberto, e cada reconexão usa o token mais recente.
  // Se a pessoa já tentou postar neste fórum (ver o tratamento de
  // forum.post_denied abaixo).
  const triedPostRef = useRef(false)
  const accessTokenRef = useRef(accessToken)
  useEffect(() => {
    accessTokenRef.current = accessToken
  }, [accessToken])

  useEffect(() => {
    let cancelled = false
    setThreads([])
    setStatus('loading')
    setError(undefined)
    setActiveThreadId(undefined)
    setPosts([])

    async function syncActiveThread() {
      const threadId = activeThreadIdRef.current
      if (!threadId) return
      const history = await fetchThreadMessages(serverBaseUrl, threadId, accessTokenRef.current, POSTS_LIMIT)
      if (!cancelled && activeThreadIdRef.current === threadId) {
        setPosts((prev) => mergeChannelHistory(prev, history, POSTS_LIMIT))
      }
    }

    const connection = createReconnectingSocket({
      label: 'canal forum',
      failureMessage: () => i18n.t('forum.connectFailed'),
      connect: () => openChannelSocket(serverBaseUrl, channelId, accessTokenRef.current),
      // Push: o servidor só poupa quem está vendo o canal (lib/channelViewing.ts).
      onOpen: (ws) => sendChannelViewing(ws, isViewing()),
      sync: async () => {
        const remoteThreads = await fetchForumThreads(serverBaseUrl, channelId, accessTokenRef.current)
        if (cancelled) return
        setThreads((prev) => mergeForumThreads(prev, remoteThreads))
        // Falha só nos posts não derruba a conexão: a lista de threads já
        // está em dia e a pessoa pode reabrir a thread.
        await syncActiveThread().catch((err) => {
          console.warn('ffcom: falha ao recarregar posts da thread aberta', err)
        })
      },
      onStatus: (next, err) => {
        if (cancelled) return
        setStatus(next)
        if (err) setError(err)
      },
      onMessage: (data) => {
        const frame = decodeChannelSocketFrame(data)
        if (!frame) return
        // Os dois casos podem já ter vindo pela REST recarregada numa
        // reconexão.
        if (frame.type === 'thread.created') {
          setThreads((prev) => (prev.some((t) => t.id === frame.thread.id) ? prev : [frame.thread, ...prev]))
        } else if (frame.type === 'post.created') {
          if (frame.message.threadId === activeThreadIdRef.current) {
            setPosts((prev) => (prev.some((m) => m.id === frame.message.id) ? prev : [...prev, frame.message]))
          }
        } else if (frame.type === 'error') {
          // Servidor anterior ao channel-v0.11.2 checa SendMessages antes do
          // tipo do frame e recusa o "channel.viewing" de quem só lê o fórum
          // com forum.post_denied; sem nenhum post tentado, não é da pessoa.
          if (frame.code === 'forum.post_denied' && !triedPostRef.current) return
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

  function openThread(threadId: string | undefined) {
    setActiveThreadId(threadId)
    setPosts([])
    setError(undefined)
    if (!threadId) return

    setPostsLoading(true)
    fetchThreadMessages(serverBaseUrl, threadId, accessToken)
      .then((history) => {
        if (activeThreadIdRef.current === threadId) setPosts(history)
      })
      .catch((err) => {
        setError(err instanceof Error ? err : new LocalizedError(() => i18n.t('forum.postsLoadFailed')))
      })
      .finally(() => setPostsLoading(false))
  }

  function createThread(title: string, content: string) {
    const socket = connectionRef.current?.current()
    if (!socket) return
    triedPostRef.current = true
    sendCreateThread(socket, title, content)
  }

  function createPost(content: string) {
    const socket = connectionRef.current?.current()
    const threadId = activeThreadIdRef.current
    if (!socket || !threadId) return
    triedPostRef.current = true
    sendCreatePost(socket, threadId, content)
  }

  const errorText = useErrorText(error)

  return {
    threads,
    status,
    error: errorText,
    activeThreadId,
    posts,
    postsLoading,
    openThread,
    createThread,
    createPost,
  }
}
