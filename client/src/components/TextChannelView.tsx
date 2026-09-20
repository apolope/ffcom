import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { useChannelChat } from '../hooks/useChannelChat'
import { fetchMe } from '../lib/serverChannelApi'
import type { Channel } from '../types'
import './TextChannelView.css'

interface TextChannelViewProps {
  serverBaseUrl: string
  channel: Channel
}

export function TextChannelView({ serverBaseUrl, channel }: TextChannelViewProps) {
  const { accessToken } = useAuth()
  const [selfMemberId, setSelfMemberId] = useState<string>()
  const [draft, setDraft] = useState('')
  const listRef = useRef<HTMLDivElement>(null)

  const { messages, status, error, sendMessage } = useChannelChat(
    serverBaseUrl,
    channel.id,
    accessToken!,
  )

  useEffect(() => {
    fetchMe(serverBaseUrl, accessToken!)
      .then((me) => setSelfMemberId(me.memberId))
      .catch(() => {
        /* não bloqueia o chat só por não saber o próprio memberId */
      })
  }, [serverBaseUrl, accessToken])

  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight })
  }, [messages])

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const content = draft.trim()
    if (!content) return
    sendMessage(content)
    setDraft('')
  }

  return (
    <div className="text-channel">
      <div className="message-list" ref={listRef}>
        {status === 'loading' && <p className="placeholder">Carregando histórico…</p>}
        {messages.map((m) => (
          <div
            key={m.id}
            className={m.authorMemberId === selfMemberId ? 'message message-self' : 'message'}
          >
            <span className="message-author">{m.authorMemberId === selfMemberId ? 'você' : m.authorMemberId.slice(0, 8)}</span>
            <span className="message-content">{m.content}</span>
          </div>
        ))}
        {messages.length === 0 && status === 'open' && (
          <p className="placeholder">Nenhuma mensagem ainda. Seja o primeiro a escrever.</p>
        )}
      </div>
      {error && <div className="message-error">{error}</div>}
      <form className="message-form" onSubmit={handleSubmit}>
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={`Enviar mensagem em #${channel.name}`}
          disabled={status !== 'open'}
        />
        <button type="submit" disabled={status !== 'open' || draft.trim() === ''}>
          Enviar
        </button>
      </form>
    </div>
  )
}
