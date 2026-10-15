import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthProvider'
import { useForumChannel } from '../hooks/useForumChannel'
import { formatDateTime, formatMessageTime } from '../lib/format'
import { fetchMe } from '../lib/serverChannelApi'
import { AttachFileButton } from './AttachFileButton'
import { MessageAttachment } from './MessageAttachment'
import { MessageTime } from './TextChannelView'
import type { Channel } from '../types'
import './ForumChannelView.css'

interface ForumChannelViewProps {
  serverBaseUrl: string
  channel: Channel
  authorName: (memberId: string) => string
}

export function ForumChannelView({ serverBaseUrl, channel, authorName }: ForumChannelViewProps) {
  const { t } = useTranslation()
  const { accessToken } = useAuth()
  const [selfMemberId, setSelfMemberId] = useState<string>()
  const [composing, setComposing] = useState(false)
  const [draftTitle, setDraftTitle] = useState('')
  const [draftContent, setDraftContent] = useState('')
  const [draftFile, setDraftFile] = useState<File>()
  const [reply, setReply] = useState('')
  const [replyFile, setReplyFile] = useState<File>()
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
    createThreadWithFile,
    createPostWithFile,
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

  const activeThread = threads.find((thread) => thread.id === activeThreadId)

  function handleCreateThread(e: FormEvent) {
    e.preventDefault()
    const title = draftTitle.trim()
    const content = draftContent.trim()
    if (!title || (!content && !draftFile)) return
    if (draftFile) {
      createThreadWithFile(title, content, draftFile)
    } else {
      createThread(title, content)
    }
    closeComposer()
  }

  function closeComposer() {
    setDraftTitle('')
    setDraftContent('')
    setDraftFile(undefined)
    setComposing(false)
  }

  function handleReply(e: FormEvent) {
    e.preventDefault()
    const content = reply.trim()
    if (!content && !replyFile) return
    if (replyFile) {
      createPostWithFile(content, replyFile)
      setReplyFile(undefined)
    } else {
      createPost(content)
    }
    setReply('')
  }

  // Trocar de thread descarta o anexo escolhido para a anterior.
  function handleOpenThread(threadId: string | undefined) {
    setReplyFile(undefined)
    openThread(threadId)
  }

  if (activeThreadId) {
    return (
      <div className="forum-channel">
        <div className="forum-thread-header">
          <button className="forum-back" onClick={() => handleOpenThread(undefined)}>
            {t('forum.back')}
          </button>
          <span className="forum-thread-title">{activeThread?.title ?? t('forum.threadFallback')}</span>
        </div>
        <div className="message-list" ref={listRef}>
          {postsLoading && <p className="placeholder">{t('forum.loadingPosts')}</p>}
          {!postsLoading &&
            posts.map((m) => (
              <div
                key={m.id}
                className={m.authorMemberId === selfMemberId ? 'message message-self' : 'message'}
              >
                <span className="message-author">
                  {m.authorMemberId === selfMemberId ? t('common.you') : authorName(m.authorMemberId)}
                </span>
                <MessageTime createdAt={m.createdAt} />
                {m.content && <span className="message-content">{m.content}</span>}
                {m.attachments?.map((a) => (
                  <MessageAttachment key={a.id} serverBaseUrl={serverBaseUrl} attachment={a} />
                ))}
              </div>
            ))}
        </div>
        {status === 'reconnecting' && <div className="message-status">{t('chat.reconnecting')}</div>}
        {error && <div className="message-error">{error}</div>}
        {replyFile && (
          <PendingAttachment file={replyFile} onRemove={() => setReplyFile(undefined)} />
        )}
        <form className="message-form" onSubmit={handleReply}>
          <AttachFileButton onPick={setReplyFile} disabled={status !== 'open'} />
          <input
            type="text"
            value={reply}
            onChange={(e) => setReply(e.target.value)}
            placeholder={t('forum.replyPlaceholder')}
            disabled={status !== 'open'}
          />
          <button type="submit" disabled={status !== 'open' || (reply.trim() === '' && !replyFile)}>
            {t('common.send')}
          </button>
        </form>
      </div>
    )
  }

  return (
    <div className="forum-channel">
      <div className="forum-thread-list">
        {status === 'loading' && <p className="placeholder">{t('forum.loadingThreads')}</p>}
        {status !== 'loading' && threads.length === 0 && !composing && (
          <p className="placeholder">{t('forum.empty')}</p>
        )}
        {threads.map((thread) => (
          <button key={thread.id} className="forum-thread-item" onClick={() => handleOpenThread(thread.id)}>
            <span className="forum-thread-item-title">{thread.title}</span>
            <span className="forum-thread-item-meta">
              {thread.authorMemberId === selfMemberId ? t('common.you') : authorName(thread.authorMemberId)}
              {' · '}
              <time dateTime={thread.createdAt} title={formatDateTime(thread.createdAt)}>
                {formatMessageTime(thread.createdAt)}
              </time>
            </span>
          </button>
        ))}
      </div>
      {status === 'reconnecting' && <div className="message-status">{t('chat.reconnecting')}</div>}
      {error && <div className="message-error">{error}</div>}
      {composing ? (
        <form className="forum-new-thread-form" onSubmit={handleCreateThread}>
          <input
            type="text"
            value={draftTitle}
            onChange={(e) => setDraftTitle(e.target.value)}
            placeholder={t('forum.titlePlaceholder')}
            disabled={status !== 'open'}
            autoFocus
          />
          <textarea
            value={draftContent}
            onChange={(e) => setDraftContent(e.target.value)}
            placeholder={t('forum.contentPlaceholder')}
            disabled={status !== 'open'}
            rows={3}
          />
          {draftFile && <PendingAttachment file={draftFile} onRemove={() => setDraftFile(undefined)} />}
          <div className="forum-new-thread-actions">
            <AttachFileButton onPick={setDraftFile} disabled={status !== 'open'} />
            <button type="button" className="forum-cancel" onClick={closeComposer}>
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={
                status !== 'open' || draftTitle.trim() === '' || (draftContent.trim() === '' && !draftFile)
              }
            >
              {t('forum.publish')}
            </button>
          </div>
        </form>
      ) : (
        <div className="forum-new-thread-trigger">
          <button onClick={() => setComposing(true)} disabled={status !== 'open'}>
            {t('forum.newPost')}
          </button>
        </div>
      )}
    </div>
  )
}

function PendingAttachment({ file, onRemove }: { file: File; onRemove: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="pending-attachment">
      <span>{file.name}</span>
      <button type="button" onClick={onRemove}>
        {t('chat.removeAttachment')}
      </button>
    </div>
  )
}
