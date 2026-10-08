import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthProvider'
import { useChannelChat } from '../hooks/useChannelChat'
import { formatDateTime, formatMessageTime } from '../lib/format'
import { fetchMe } from '../lib/serverChannelApi'
import { MessageAttachment } from './MessageAttachment'
import type { Channel } from '../types'
import './TextChannelView.css'

interface TextChannelViewProps {
  serverBaseUrl: string
  channel: Channel
  authorName: (memberId: string) => string
  // Administrator ou dono: pode apagar mensagem de qualquer membro.
  canModerateMessages: boolean
}

export function TextChannelView({ serverBaseUrl, channel, authorName, canModerateMessages }: TextChannelViewProps) {
  const { t } = useTranslation()
  const { accessToken } = useAuth()
  const [selfMemberId, setSelfMemberId] = useState<string>()
  const [draft, setDraft] = useState('')
  const [pendingFile, setPendingFile] = useState<File>()
  const [editingId, setEditingId] = useState<string>()
  const [editDraft, setEditDraft] = useState('')
  // Apagar mensagem alheia pede um segundo clique (mesmo padrão dos
  // diálogos de estrutura, sem confirm() do navegador): quem modera não
  // tem como recuperar o que apagou por engano.
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string>()
  const listRef = useRef<HTMLDivElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const { messages, status, error, sendMessage, sendMessageWithFile, editMessage, deleteMessage } =
    useChannelChat(serverBaseUrl, channel.id, accessToken!)

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
    if (!content && !pendingFile) return
    if (pendingFile) {
      sendMessageWithFile(content, pendingFile)
      clearPendingFile()
    } else {
      sendMessage(content)
    }
    setDraft('')
  }

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    setPendingFile(e.target.files?.[0])
  }

  function clearPendingFile() {
    setPendingFile(undefined)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  function startEditing(id: string, content: string) {
    setEditingId(id)
    setEditDraft(content)
  }

  function cancelEditing() {
    setEditingId(undefined)
    setEditDraft('')
  }

  function handleEditSubmit(e: FormEvent) {
    e.preventDefault()
    const content = editDraft.trim()
    if (!content || !editingId) return
    editMessage(editingId, content)
    cancelEditing()
  }

  return (
    <div className="text-channel">
      <div className="message-list" ref={listRef}>
        {status === 'loading' && <p className="placeholder">{t('chat.loadingHistory')}</p>}
        {messages.map((m) => {
          const isSelf = m.authorMemberId === selfMemberId
          return (
            <div key={m.id} className={isSelf ? 'message message-self' : 'message'}>
              <span className="message-author">{isSelf ? t('common.you') : authorName(m.authorMemberId)}</span>
              <MessageTime createdAt={m.createdAt} />
              {editingId === m.id ? (
                <form className="message-edit-form" onSubmit={handleEditSubmit}>
                  <input
                    type="text"
                    value={editDraft}
                    onChange={(e) => setEditDraft(e.target.value)}
                    autoFocus
                  />
                  <button type="submit" disabled={editDraft.trim() === ''}>
                    {t('common.save')}
                  </button>
                  <button type="button" onClick={cancelEditing}>
                    {t('common.cancel')}
                  </button>
                </form>
              ) : (
                <>
                  {m.content && <span className="message-content">{m.content}</span>}
                  {m.editedAt && (
                    <span className="message-edited" title={formatDateTime(m.editedAt)}>
                      {t('chat.edited')}
                    </span>
                  )}
                  {m.attachments?.map((a) => (
                    <MessageAttachment key={a.id} serverBaseUrl={serverBaseUrl} attachment={a} />
                  ))}
                  {isSelf && (
                    <span className="message-actions">
                      <button type="button" onClick={() => startEditing(m.id, m.content)}>
                        {t('chat.edit')}
                      </button>
                      <button type="button" onClick={() => deleteMessage(m.id)}>
                        {t('chat.delete')}
                      </button>
                    </span>
                  )}
                  {!isSelf && canModerateMessages && (
                    <span
                      className={
                        confirmingDeleteId === m.id ? 'message-actions message-actions-confirming' : 'message-actions'
                      }
                    >
                      {confirmingDeleteId === m.id ? (
                        <>
                          <button type="button" className="message-action-danger" onClick={() => deleteMessage(m.id)}>
                            {t('chat.confirmDelete')}
                          </button>
                          <button type="button" onClick={() => setConfirmingDeleteId(undefined)}>
                            {t('chat.cancel')}
                          </button>
                        </>
                      ) : (
                        <button
                          type="button"
                          title={t('chat.moderateDelete')}
                          onClick={() => setConfirmingDeleteId(m.id)}
                        >
                          {t('chat.delete')}
                        </button>
                      )}
                    </span>
                  )}
                </>
              )}
            </div>
          )
        })}
        {messages.length === 0 && status === 'open' && (
          <p className="placeholder">{t('chat.empty')}</p>
        )}
      </div>
      {status === 'reconnecting' && <div className="message-status">{t('chat.reconnecting')}</div>}
      {error && <div className="message-error">{error}</div>}
      {pendingFile && (
        <div className="pending-attachment">
          <span>{pendingFile.name}</span>
          <button type="button" onClick={clearPendingFile}>
            {t('chat.removeAttachment')}
          </button>
        </div>
      )}
      <form className="message-form" onSubmit={handleSubmit}>
        {/* Input nativo escondido: o texto dele ("Nenhum ficheiro
            selecionado", no idioma do navegador) era cortado pela largura
            do formulário, e o nome do arquivo escolhido já aparece em
            .pending-attachment. O botão de clipe abre o seletor. */}
        <input
          type="file"
          ref={fileInputRef}
          onChange={handleFileChange}
          disabled={status !== 'open'}
          hidden
        />
        <button
          type="button"
          className="message-attach-button"
          onClick={() => fileInputRef.current?.click()}
          disabled={status !== 'open'}
          title={t('chat.attachFile')}
          aria-label={t('chat.attachFile')}
        >
          <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" aria-hidden="true">
            <path d="M16.5 6.5v10a4.5 4.5 0 0 1-9 0V5a3 3 0 0 1 6 0v10.5a1.5 1.5 0 0 1-3 0V6.5H9v9a3 3 0 0 0 6 0V5a4.5 4.5 0 0 0-9 0v11.5a6 6 0 0 0 12 0v-10h-1.5z" />
          </svg>
        </button>
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={t('chat.placeholder', { channel: channel.name })}
          disabled={status !== 'open'}
        />
        <button type="submit" disabled={status !== 'open' || (draft.trim() === '' && !pendingFile)}>
          {t('common.send')}
        </button>
      </form>
    </div>
  )
}

// Hora da mensagem no idioma ativo (só a hora se for de hoje, data e hora
// se não), com a data completa no title. Também usada pelo fórum.
export function MessageTime({ createdAt }: { createdAt: string }) {
  useTranslation()
  return (
    <time className="message-time" dateTime={createdAt} title={formatDateTime(createdAt)}>
      {formatMessageTime(createdAt)}
    </time>
  )
}
