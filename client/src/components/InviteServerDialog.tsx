import { useState } from 'react'
import './Dialog.css'
import './InviteCode.css'

interface InviteServerDialogProps {
  serverName: string
  onCreateInvite: () => Promise<string>
  onClose: () => void
}

// Gera um código de convite para este server-channel (ver
// docs/architecture.md, "Convites obrigatórios para entrar em
// server-channel"). O resgate acontece do outro lado, em
// AddServerDialog, junto com o endereço do servidor.
export function InviteServerDialog({ serverName, onCreateInvite, onClose }: InviteServerDialogProps) {
  const [invite, setInvite] = useState<string>()
  const [error, setError] = useState<string>()
  const [generating, setGenerating] = useState(false)
  const [copied, setCopied] = useState(false)

  async function handleGenerateInvite() {
    setError(undefined)
    setGenerating(true)
    try {
      const code = await onCreateInvite()
      setInvite(code)
      setCopied(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'falha ao gerar convite')
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
        <h2>Convidar para {serverName}</h2>

        <section className="invite-section">
          <p className="invite-hint">
            Gere um código e compartilhe com quem você quer convidar. Quem entrar vai precisar
            também do endereço deste servidor.
          </p>
          {invite ? (
            <div className="invite-code">
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
          {error && <p className="dialog-error">{error}</p>}
        </section>

        <div className="dialog-actions">
          <button type="button" onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  )
}
