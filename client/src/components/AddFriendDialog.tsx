import { useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { errorMessage } from '../lib/apiError'
import './Dialog.css'
import './InviteCode.css'

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
  const { t } = useTranslation()
  const [invite, setInvite] = useState<string>()
  const [inviteError, setInviteError] = useState<unknown>()
  const [generating, setGenerating] = useState(false)
  const [copied, setCopied] = useState(false)

  const [code, setCode] = useState('')
  const [redeemError, setRedeemError] = useState<unknown>()
  const [redeeming, setRedeeming] = useState(false)

  async function handleGenerateInvite() {
    setInviteError(undefined)
    setGenerating(true)
    try {
      const newCode = await onCreateInvite()
      setInvite(newCode)
      setCopied(false)
    } catch (err) {
      setInviteError(err)
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
      setRedeemError(err)
    } finally {
      setRedeeming(false)
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card" onClick={(e) => e.stopPropagation()}>
        <h2>{t('friends.addFriend')}</h2>

        <section className="invite-section">
          <p className="invite-hint">{t('friends.addDialog.hint')}</p>
          {invite ? (
            <div className="invite-code">
              <code>{invite}</code>
              <button type="button" onClick={handleCopyInvite}>
                {copied ? t('friends.addDialog.copied') : t('common.copy')}
              </button>
            </div>
          ) : (
            <button type="button" onClick={handleGenerateInvite} disabled={generating}>
              {generating ? t('friends.addDialog.generating') : t('friends.addDialog.generate')}
            </button>
          )}
          {inviteError !== undefined && (
            <p className="dialog-error">{errorMessage(inviteError, t('friends.addDialog.generateFailed'))}</p>
          )}
        </section>

        <form className="invite-section" onSubmit={handleRedeem}>
          <label>
            {t('friends.addDialog.haveCode')}
            <input
              type="text"
              placeholder={t('friends.addDialog.codePlaceholder')}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              required
            />
          </label>
          {redeemError !== undefined && (
            <p className="dialog-error">{errorMessage(redeemError, t('friends.addDialog.addFailed'))}</p>
          )}
          <div className="dialog-actions">
            <button type="button" onClick={onClose}>
              {t('common.close')}
            </button>
            <button type="submit" className="dialog-submit" disabled={redeeming || !code.trim()}>
              {redeeming ? t('friends.addDialog.adding') : t('common.add')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
