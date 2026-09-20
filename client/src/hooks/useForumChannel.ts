import { useEffect, useRef, useState } from 'react'
import {
  decodeChannelSocketFrame,
  fetchForumThreads,
  fetchThreadMessages,
  openChannelSocket,
  sendCreatePost,
  sendCreateThread,
  type ChannelMessage,
  type RemoteThread,
} from '../lib/serverChannelApi'

export type ForumConnectionStatus = 'loading' | 'open' | 'closed' | 'error'

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
// ao vivo pela mesma conexão.
export function useForumChannel(
  serverBaseUrl: string,
  channelId: string,
  accessToken: string,
): UseForumChannelResult {
  const [threads, setThreads] = useState<RemoteThread[]>([])
  const [status, setStatus] = useState<ForumConnectionStatus>('loading')
  const [error, setError] = useState<string>()
  const [activeThreadId, setActiveThreadId] = useState<string>()
  const [posts, setPosts] = useState<ChannelMessage[]>([])
  const [postsLoading, setPostsLoading] = useState(false)

  const socketRef = useRef<WebSocket | null>(null)
  const activeThreadIdRef = useRef<string>()
  useEffect(() => {
    activeThreadIdRef.current = activeThreadId
  }, [activeThreadId])

  useEffect(() => {
    let cancelled = false
    setThreads([])
    setStatus('loading')
    setError(undefined)
    setActiveThreadId(undefined)
    setPosts([])

    let socket: WebSocket | null = null

    fetchForumThreads(serverBaseUrl, channelId, accessToken)
      .then((remoteThreads) => {
        if (cancelled) return
        setThreads(remoteThreads)

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
          if (frame.type === 'thread.created') {
            setThreads((prev) => [frame.thread, ...prev])
          } else if (frame.type === 'post.created') {
            if (frame.message.threadId === activeThreadIdRef.current) {
              setPosts((prev) => [...prev, frame.message])
            }
          } else if (frame.type === 'error') {
            setError(frame.error)
          }
        }
      })
      .catch((err) => {
        if (cancelled) return
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar threads')
      })

    return () => {
      cancelled = true
      socket?.close()
      socketRef.current = null
    }
  }, [serverBaseUrl, channelId, accessToken])

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
        setError(err instanceof Error ? err.message : 'falha ao carregar posts')
      })
      .finally(() => setPostsLoading(false))
  }

  function createThread(title: string, content: string) {
    const socket = socketRef.current
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    sendCreateThread(socket, title, content)
  }

  function createPost(content: string) {
    const socket = socketRef.current
    const threadId = activeThreadIdRef.current
    if (!socket || socket.readyState !== WebSocket.OPEN || !threadId) return
    sendCreatePost(socket, threadId, content)
  }

  return {
    threads,
    status,
    error,
    activeThreadId,
    posts,
    postsLoading,
    openThread,
    createThread,
    createPost,
  }
}
