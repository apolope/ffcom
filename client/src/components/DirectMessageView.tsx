import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useDirectMessages } from '../hooks/useDirectMessages'
import type { E2EKeyPair } from '../crypto/e2e'
import type { Friend } from '../types'
import { MobileNavButton } from './MobileNavButtons'
import { UserAvatar } from './UserAvatar'
import './MainPanel.css'
import './TextChannelView.css'

interface DirectMessageViewProps {
  peer: Friend
  accessToken: string
  socket: WebSocket | null
  myKeyPair: E2EKeyPair
}

// Painel de conversa de DM, análogo a MainPanel + TextChannelView (reaproveita
// as mesmas classes CSS — ver docs/architecture.md sobre reuso de estilo
// genérico já registrado para Dialog.css).
export function DirectMessageView({ peer, accessToken, socket, myKeyPair }: DirectMessageViewProps) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState('')
  const listRef = useRef<HTMLDivElement>(null)

  const { messages, status, error, sendMessage } = useDirectMessages(accessToken, peer, socket, myKeyPair)
  const canSend = socket?.readyState === WebSocket.OPEN && !!peer.e2ePublicKey

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
        <MobileNavButton />
        <UserAvatar avatarUrl={peer.avatarUrl} displayName={peer.displayName} size={22} />
        <span className="channel-title">{peer.displayName}</span>
      </header>
      <div className="channel-content">
        <div className="text-channel">
          <div className="message-list" ref={listRef}>
            {status === 'loading' && <p className="placeholder">{t('dm.loadingConversation')}</p>}
            {messages.map((m) => (
              <div
                key={m.id}
                className={m.senderId === peer.accountId ? 'message' : 'message message-self'}
              >
                <span className="message-author">
                  {m.senderId === peer.accountId ? peer.displayName : t('common.you')}
                </span>
                {m.content !== null ? (
                  <span className="message-content">{m.content}</span>
                ) : (
                  <span className="message-content message-content-undecryptable">
                    {t('dm.undecryptable')}
                  </span>
                )}
              </div>
            ))}
            {messages.length === 0 && status === 'ready' && (
              <p className="placeholder">{t('dm.emptyConversation', { name: peer.displayName })}</p>
            )}
          </div>
          {error && <div className="message-error">{error}</div>}
          {!peer.e2ePublicKey && (
<p className="placeholder">{t('dm.peerHasNoKey', { name: peer.displayName })}</p>
          )}
          <form className="message-form" onSubmit={handleSubmit}>
            <input
              type="text"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={t('dm.inputPlaceholder', { name: peer.displayName })}
              disabled={!canSend}
            />
            <button type="submit" disabled={!canSend || draft.trim() === ''}>
              {t('common.send')}
            </button>
          </form>
        </div>
      </div>
    </section>
  )
}
