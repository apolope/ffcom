import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { errorMessage } from '../lib/apiError'
import './Dialog.css'
import './InviteCode.css'

interface InviteServerDialogProps {
  serverName: string
  serverBaseUrl: string
  onCreateInvite: () => Promise<string>
  onClose: () => void
}

// Monta um link de convite auto-contido (endereço + código) para não exigir
// que o dono divulgue o endereço do servidor por um canal separado do
// código — ver docs/architecture.md, "Decisão: convite auto-contido". O
// resgate acontece do outro lado, em AddServerDialog, que reconhece esse
// formato e separa endereço/código de volta. O nome com que quem convida
// chama o servidor vai junto (`&name=`) para já preencher o campo de nome.
function buildInviteLink(serverBaseUrl: string, code: string, serverName: string): string {
  const base = serverBaseUrl.replace(/\/+$/, '')
  const params = new URLSearchParams({ invite: code })
  if (serverName.trim()) params.set('name', serverName.trim())
  return `${base}/?${params.toString()}`
}

// Gera um código de convite para este server-channel (ver
// docs/architecture.md, "Convites obrigatórios para entrar em
// server-channel"). O resgate acontece do outro lado, em
// AddServerDialog, junto com o endereço do servidor.
export function InviteServerDialog({
  serverName,
  serverBaseUrl,
  onCreateInvite,
  onClose,
}: InviteServerDialogProps) {
  const { t } = useTranslation()
  const [invite, setInvite] = useState<string>()
  const [error, setError] = useState<unknown>()
  const [generating, setGenerating] = useState(false)
  const [copied, setCopied] = useState(false)

  async function handleGenerateInvite() {
    setError(undefined)
    setGenerating(true)
    try {
      const code = await onCreateInvite()
      setInvite(buildInviteLink(serverBaseUrl, code, serverName))
      setCopied(false)
    } catch (err) {
      setError(err)
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

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card" onClick={(e) => e.stopPropagation()}>
        <h2>{t('server.invite.title', { server: serverName })}</h2>

        <section className="invite-section">
          <p className="invite-hint">{t('server.invite.hint')}</p>
          {invite ? (
            <div className="invite-code link">
              <code>{invite}</code>
              <button type="button" onClick={handleCopyInvite}>
                {copied ? t('server.invite.copied') : t('common.copy')}
              </button>
            </div>
          ) : (
            <button type="button" onClick={handleGenerateInvite} disabled={generating}>
              {generating ? t('server.invite.generating') : t('server.invite.generate')}
            </button>
          )}
          {error !== undefined && (
            <p className="dialog-error">{errorMessage(error, t('server.invite.failed'))}</p>
          )}
        </section>

        <div className="dialog-actions">
          <button type="button" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
      </div>
    </div>
  )
}
