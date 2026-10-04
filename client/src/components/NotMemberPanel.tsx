import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { errorMessage } from '../lib/apiError'
import { parseInviteLink } from '../lib/inviteLink'
import './Dialog.css'
import './NotMemberPanel.css'

interface NotMemberPanelProps {
  serverName: string
  // Entra de novo com o código de um convite (POST /api/join); quem chama
  // recarrega /api/me e a estrutura depois.
  onJoin: (inviteCode: string) => Promise<void>
  // Tira o servidor da lista da conta no server-central.
  onRemove: () => Promise<void>
}

// Ocupa o lugar do canal quando a conta não é mais membro do servidor
// aberto (useMe.notMember): expulsa ou banida, mas com o servidor ainda na
// lista, que o server-central guarda à parte. Antes disso a tela ficava
// vazia, sem canal nem aviso. Ver docs/architecture.md, "Decisão: aviso de
// membro expulso no lugar da tela vazia".
export function NotMemberPanel({ serverName, onJoin, onRemove }: NotMemberPanelProps) {
  const { t } = useTranslation()
  const [invite, setInvite] = useState('')
  const [error, setError] = useState<unknown>()
  const [busy, setBusy] = useState(false)

  async function run(action: () => Promise<void>) {
    setError(undefined)
    setBusy(true)
    try {
      await action()
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }

  function handleJoin(event: FormEvent) {
    event.preventDefault()
    // Aceita o link inteiro (como vem do "Convidar") ou só o código.
    const value = invite.trim()
    const code = parseInviteLink(value)?.inviteCode ?? value
    void run(() => onJoin(code))
  }

  return (
    <div className="empty-state">
      <form className="dialog-card not-member-card" onSubmit={handleJoin}>
        <h2>{t('server.notMember.title', { server: serverName })}</h2>
        <p>{t('server.notMember.body')}</p>
        <label>
          {t('server.notMember.invite')}
          <input
            type="text"
            placeholder={t('server.notMember.invitePlaceholder')}
            value={invite}
            onChange={(e) => setInvite(e.target.value)}
            disabled={busy}
          />
        </label>
        {error !== undefined && (
          // ApiError já traz a mensagem do servidor traduzida pelo code (ver
          // lib/apiError.ts).
          <p className="dialog-error">{errorMessage(error, t('server.notMember.failed'))}</p>
        )}
        <div className="dialog-actions">
          <button type="button" onClick={() => void run(onRemove)} disabled={busy}>
            {t('server.notMember.removeFromList')}
          </button>
          <button type="submit" className="dialog-submit" disabled={busy || invite.trim() === ''}>
            {busy ? t('server.notMember.joining') : t('server.notMember.join')}
          </button>
        </div>
      </form>
    </div>
  )
}
