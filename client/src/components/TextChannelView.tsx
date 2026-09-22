import { useEffect, useRef, useState, type ChangeEvent, type FormEvent } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { useChannelChat } from '../hooks/useChannelChat'
import { fetchMe } from '../lib/serverChannelApi'
import { MessageAttachment } from './MessageAttachment'
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
  const [pendingFile, setPendingFile] = useState<File>()
  const [editingId, setEditingId] = useState<string>()
  const [editDraft, setEditDraft] = useState('')
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
        {status === 'loading' && <p className="placeholder">Carregando histórico…</p>}
        {messages.map((m) => {
          const isSelf = m.authorMemberId === selfMemberId
          return (
            <div key={m.id} className={isSelf ? 'message message-self' : 'message'}>
              <span className="message-author">{isSelf ? 'você' : m.authorMemberId.slice(0, 8)}</span>
              {editingId === m.id ? (
                <form className="message-edit-form" onSubmit={handleEditSubmit}>
                  <input
                    type="text"
                    value={editDraft}
                    onChange={(e) => setEditDraft(e.target.value)}
                    autoFocus
                  />
                  <button type="submit" disabled={editDraft.trim() === ''}>
                    Salvar
                  </button>
                  <button type="button" onClick={cancelEditing}>
                    Cancelar
                  </button>
                </form>
              ) : (
                <>
                  {m.content && <span className="message-content">{m.content}</span>}
                  {m.editedAt && <span className="message-edited">(editada)</span>}
                  {m.attachments?.map((a) => (
                    <MessageAttachment key={a.id} serverBaseUrl={serverBaseUrl} attachment={a} />
                  ))}
                  {isSelf && (
                    <span className="message-actions">
                      <button type="button" onClick={() => startEditing(m.id, m.content)}>
                        editar
                      </button>
                      <button type="button" onClick={() => deleteMessage(m.id)}>
                        apagar
                      </button>
                    </span>
                  )}
                </>
              )}
            </div>
          )
        })}
        {messages.length === 0 && status === 'open' && (
          <p className="placeholder">Nenhuma mensagem ainda. Seja o primeiro a escrever.</p>
        )}
      </div>
      {error && <div className="message-error">{error}</div>}
      {pendingFile && (
        <div className="pending-attachment">
          <span>{pendingFile.name}</span>
          <button type="button" onClick={clearPendingFile}>
            remover
          </button>
        </div>
      )}
      <form className="message-form" onSubmit={handleSubmit}>
        <input
          type="file"
          ref={fileInputRef}
          onChange={handleFileChange}
          disabled={status !== 'open'}
          className="message-file-input"
          title="Anexar arquivo"
        />
        <input
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={`Enviar mensagem em #${channel.name}`}
          disabled={status !== 'open'}
        />
        <button type="submit" disabled={status !== 'open' || (draft.trim() === '' && !pendingFile)}>
          Enviar
        </button>
      </form>
    </div>
  )
}
