import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { useForumChannel } from '../hooks/useForumChannel'
import { fetchMe } from '../lib/serverChannelApi'
import type { Channel } from '../types'
import './ForumChannelView.css'

interface ForumChannelViewProps {
  serverBaseUrl: string
  channel: Channel
}

export function ForumChannelView({ serverBaseUrl, channel }: ForumChannelViewProps) {
  const { accessToken } = useAuth()
  const [selfMemberId, setSelfMemberId] = useState<string>()
  const [composing, setComposing] = useState(false)
  const [draftTitle, setDraftTitle] = useState('')
  const [draftContent, setDraftContent] = useState('')
  const [reply, setReply] = useState('')
  const listRef = useRef<HTMLDivElement>(null)

  const {
    threads,
    status,
    error,
    activeThreadId,
    posts,
    postsLoading,
    openThread,
    createThread,
    createPost,
  } = useForumChannel(serverBaseUrl, channel.id, accessToken!)

  useEffect(() => {
    fetchMe(serverBaseUrl, accessToken!)
      .then((me) => setSelfMemberId(me.memberId))
      .catch(() => {
        /* não bloqueia a UI só por não saber o próprio memberId */
      })
  }, [serverBaseUrl, accessToken])

  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight })
  }, [posts])

  const activeThread = threads.find((t) => t.id === activeThreadId)

  function handleCreateThread(e: FormEvent) {
    e.preventDefault()
    const title = draftTitle.trim()
    const content = draftContent.trim()
    if (!title || !content) return
    createThread(title, content)
    setDraftTitle('')
    setDraftContent('')
    setComposing(false)
  }

  function handleReply(e: FormEvent) {
    e.preventDefault()
    const content = reply.trim()
    if (!content) return
    createPost(content)
    setReply('')
  }

  if (activeThreadId) {
    return (
      <div className="forum-channel">
        <div className="forum-thread-header">
          <button className="forum-back" onClick={() => openThread(undefined)}>
            ← Threads
          </button>
          <span className="forum-thread-title">{activeThread?.title ?? 'Thread'}</span>
        </div>
        <div className="message-list" ref={listRef}>
          {postsLoading && <p className="placeholder">Carregando posts…</p>}
          {!postsLoading &&
            posts.map((m) => (
              <div
                key={m.id}
                className={m.authorMemberId === selfMemberId ? 'message message-self' : 'message'}
              >
                <span className="message-author">
                  {m.authorMemberId === selfMemberId ? 'você' : m.authorMemberId.slice(0, 8)}
                </span>
                <span className="message-content">{m.content}</span>
              </div>
            ))}
        </div>
        {error && <div className="message-error">{error}</div>}
        <form className="message-form" onSubmit={handleReply}>
          <input
            type="text"
            value={reply}
            onChange={(e) => setReply(e.target.value)}
            placeholder="Responder na thread"
            disabled={status !== 'open'}
          />
          <button type="submit" disabled={status !== 'open' || reply.trim() === ''}>
            Enviar
          </button>
        </form>
      </div>
    )
  }

  return (
    <div className="forum-channel">
      <div className="forum-thread-list">
        {status === 'loading' && <p className="placeholder">Carregando threads…</p>}
        {status !== 'loading' && threads.length === 0 && !composing && (
          <p className="placeholder">Nenhum post ainda. Seja o primeiro a abrir uma thread.</p>
        )}
        {threads.map((t) => (
          <button key={t.id} className="forum-thread-item" onClick={() => openThread(t.id)}>
            <span className="forum-thread-item-title">{t.title}</span>
            <span className="forum-thread-item-meta">
              {t.authorMemberId === selfMemberId ? 'você' : t.authorMemberId.slice(0, 8)}
            </span>
          </button>
        ))}
      </div>
      {error && <div className="message-error">{error}</div>}
      {composing ? (
        <form className="forum-new-thread-form" onSubmit={handleCreateThread}>
          <input
            type="text"
            value={draftTitle}
            onChange={(e) => setDraftTitle(e.target.value)}
            placeholder="Título do post"
            disabled={status !== 'open'}
            autoFocus
          />
          <textarea
            value={draftContent}
            onChange={(e) => setDraftContent(e.target.value)}
            placeholder="Conteúdo do post inicial"
            disabled={status !== 'open'}
            rows={3}
          />
          <div className="forum-new-thread-actions">
            <button type="button" className="forum-cancel" onClick={() => setComposing(false)}>
              Cancelar
            </button>
            <button
              type="submit"
              disabled={status !== 'open' || draftTitle.trim() === '' || draftContent.trim() === ''}
            >
              Publicar
            </button>
          </div>
        </form>
      ) : (
        <div className="forum-new-thread-trigger">
          <button onClick={() => setComposing(true)} disabled={status !== 'open'}>
            Novo post
          </button>
        </div>
      )}
    </div>
  )
}
