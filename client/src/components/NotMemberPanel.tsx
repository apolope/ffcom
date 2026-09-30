import { useState, type FormEvent } from 'react'
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
  const [invite, setInvite] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)

  async function run(action: () => Promise<void>) {
    setError(undefined)
    setBusy(true)
    try {
      await action()
    } catch (err) {
      setError(errorText(err))
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
        <h2>Você não é mais membro de {serverName}</h2>
        <p>
          Você foi removido deste servidor, então os canais dele não aparecem mais. Para voltar, cole um convite
          novo de alguém que ainda está lá. Se não quiser voltar, tire o servidor da sua lista.
        </p>
        <label>
          Convite
          <input
            type="text"
            placeholder="Link ou código do convite"
            value={invite}
            onChange={(e) => setInvite(e.target.value)}
            disabled={busy}
          />
        </label>
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={() => void run(onRemove)} disabled={busy}>
            Remover da lista
          </button>
          <button type="submit" className="dialog-submit" disabled={busy || invite.trim() === ''}>
            {busy ? 'Entrando…' : 'Entrar com convite'}
          </button>
        </div>
      </form>
    </div>
  )
}

// HttpError vem como "403 Forbidden: banido deste servidor"; a parte depois
// do status é a mensagem do servidor, em português.
function errorText(err: unknown): string {
  if (!(err instanceof Error)) return 'Não deu certo. Tente de novo.'
  const match = /^\d{3} [^:]*: (.+)$/s.exec(err.message)
  return match ? match[1] : err.message
}
