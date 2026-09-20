import { useState } from 'react'
import type { FormEvent } from 'react'
import './Dialog.css'
import './AddFriendDialog.css'

interface AddFriendDialogProps {
  onCreateInvite: () => Promise<string>
  onRedeemInvite: (code: string) => Promise<void>
  onClose: () => void
}

// Adiciona um amigo via convite (ver docs/architecture.md, "Decisão:
// adicionar amigos via convite" — sem username pesquisável, mesma filosofia
// de AddServerDialog para server-channel). Duas ações independentes: gerar
// um código para compartilhar, ou resgatar um código que alguém te deu.
export function AddFriendDialog({ onCreateInvite, onRedeemInvite, onClose }: AddFriendDialogProps) {
  const [invite, setInvite] = useState<string>()
  const [inviteError, setInviteError] = useState<string>()
  const [generating, setGenerating] = useState(false)
  const [copied, setCopied] = useState(false)

  const [code, setCode] = useState('')
  const [redeemError, setRedeemError] = useState<string>()
  const [redeeming, setRedeeming] = useState(false)

  async function handleGenerateInvite() {
    setInviteError(undefined)
    setGenerating(true)
    try {
      const newCode = await onCreateInvite()
      setInvite(newCode)
      setCopied(false)
    } catch (err) {
      setInviteError(err instanceof Error ? err.message : 'falha ao gerar convite')
    } finally {
      setGenerating(false)
    }
  }

  async function handleCopyInvite() {
    if (!invite) return
    try {
      await navigator.clipboard.writeText(invite)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  async function handleRedeem(event: FormEvent) {
    event.preventDefault()
    setRedeemError(undefined)
    setRedeeming(true)
    try {
      await onRedeemInvite(code.trim())
      onClose()
    } catch (err) {
      setRedeemError(err instanceof Error ? err.message : 'falha ao adicionar amigo')
    } finally {
      setRedeeming(false)
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card" onClick={(e) => e.stopPropagation()}>
        <h2>Adicionar amigo</h2>

        <section className="friend-invite-section">
          <p className="friend-invite-hint">Gere um código e compartilhe com quem você quer adicionar.</p>
          {invite ? (
            <div className="friend-invite-code">
              <code>{invite}</code>
              <button type="button" onClick={handleCopyInvite}>
                {copied ? 'Copiado!' : 'Copiar'}
              </button>
            </div>
          ) : (
            <button type="button" onClick={handleGenerateInvite} disabled={generating}>
              {generating ? 'Gerando…' : 'Gerar código de convite'}
            </button>
          )}
          {inviteError && <p className="dialog-error">{inviteError}</p>}
        </section>

        <form className="friend-invite-section" onSubmit={handleRedeem}>
          <label>
            Já tenho um código
            <input
              type="text"
              placeholder="Código de convite"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              required
            />
          </label>
          {redeemError && <p className="dialog-error">{redeemError}</p>}
          <div className="dialog-actions">
            <button type="button" onClick={onClose}>
              Fechar
            </button>
            <button type="submit" className="dialog-submit" disabled={redeeming || !code.trim()}>
              {redeeming ? 'Adicionando…' : 'Adicionar'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
