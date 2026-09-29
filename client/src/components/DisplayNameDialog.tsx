import { useState } from 'react'
import './Dialog.css'

interface DisplayNameDialogProps {
  // Nome escolhido no FFCom, se houver (MyProfile.customDisplayName).
  currentName: string | undefined
  // Nome do perfil do Authentik, usado quando o campo fica vazio.
  authentikName: string | undefined
  onSave: (displayName: string | undefined) => Promise<void>
  onClose: () => void
}

// Nome de exibição da conta (server-central): o que amigos e DMs mostram e
// o nome nos servidores onde a pessoa não tem apelido. Vazio volta ao nome
// do Authentik. Ver docs/architecture.md, "Decisão: nome de exibição da
// conta".
export function DisplayNameDialog({ currentName, authentikName, onSave, onClose }: DisplayNameDialogProps) {
  const [value, setValue] = useState(currentName ?? '')
  const [error, setError] = useState<string>()
  const [saving, setSaving] = useState(false)

  async function handleSave() {
    setError(undefined)
    setSaving(true)
    try {
      await onSave(value.trim() || undefined)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'falha ao salvar o nome')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <form
        className="dialog-card"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault()
          if (!saving) void handleSave()
        }}
      >
        <h2>Seu nome de exibição</h2>
        <input
          type="text"
          value={value}
          maxLength={64}
          placeholder={authentikName ?? 'Como seus amigos vão te ver'}
          onChange={(e) => setValue(e.target.value)}
          autoFocus
        />
        <p className="dialog-hint">
          Aparece para os amigos, nas mensagens diretas e nos servidores onde você não tem apelido.
          {authentikName && <> Deixe vazio para usar o nome da sua conta ({authentikName}).</>}
        </p>
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={saving}>
            Cancelar
          </button>
          <button type="submit" className="dialog-submit" disabled={saving}>
            {saving ? 'Salvando…' : 'Salvar'}
          </button>
        </div>
      </form>
    </div>
  )
}
