import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useDirectMessages } from '../hooks/useDirectMessages'
import type { Friend } from '../types'
import './MainPanel.css'
import './TextChannelView.css'

interface DirectMessageViewProps {
  peer: Friend
  accessToken: string
  socket: WebSocket | null
}

// Painel de conversa de DM, análogo a MainPanel + TextChannelView (reaproveita
// as mesmas classes CSS — ver docs/architecture.md sobre reuso de estilo
// genérico já registrado para Dialog.css).
export function DirectMessageView({ peer, accessToken, socket }: DirectMessageViewProps) {
  const [draft, setDraft] = useState('')
  const listRef = useRef<HTMLDivElement>(null)

  const { messages, status, error, sendMessage } = useDirectMessages(accessToken, peer.accountId, socket)
  const canSend = socket?.readyState === WebSocket.OPEN

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
    <section className="main-panel">
      <header className="channel-header">
        <span className="channel-title">{peer.displayName}</span>
      </header>
      <div className="channel-content">
        <div className="text-channel">
          <div className="message-list" ref={listRef}>
            {status === 'loading' && <p className="placeholder">Carregando conversa…</p>}
            {messages.map((m) => (
              <div
                key={m.id}
                className={m.senderId === peer.accountId ? 'message' : 'message message-self'}
              >
                <span className="message-author">
                  {m.senderId === peer.accountId ? peer.displayName : 'você'}
                </span>
                <span className="message-content">{m.content}</span>
              </div>
            ))}
            {messages.length === 0 && status === 'ready' && (
              <p className="placeholder">Nenhuma mensagem ainda. Diga oi para {peer.displayName}.</p>
            )}
          </div>
          {error && <div className="message-error">{error}</div>}
          <form className="message-form" onSubmit={handleSubmit}>
            <input
              type="text"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={`Enviar mensagem para ${peer.displayName}`}
              disabled={!canSend}
            />
            <button type="submit" disabled={!canSend || draft.trim() === ''}>
              Enviar
            </button>
          </form>
        </div>
      </div>
    </section>
  )
}
