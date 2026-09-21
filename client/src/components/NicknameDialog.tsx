import { useState } from 'react'
import './Dialog.css'

interface NicknameDialogProps {
  currentNickname: string | undefined
  onSave: (nickname: string | undefined) => Promise<void>
  onClose: () => void
}

// Define o apelido exibido na lista de membros deste server-channel (ver
// docs/architecture.md, endpoint PATCH /api/me novo em
// server-channel/internal/httpapi/me.go) — sem apelido, a lista cai no
// UUID truncado do membro (ver hooks/useServerMembers.ts).
export function NicknameDialog({ currentNickname, onSave, onClose }: NicknameDialogProps) {
  const [value, setValue] = useState(currentNickname ?? '')
  const [error, setError] = useState<string>()
  const [saving, setSaving] = useState(false)

  async function handleSave() {
    setError(undefined)
    setSaving(true)
    try {
      await onSave(value.trim() || undefined)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'falha ao salvar apelido')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card" onClick={(e) => e.stopPropagation()}>
        <h2>Seu apelido neste servidor</h2>
        <input
          type="text"
          value={value}
          maxLength={64}
          placeholder="Como os outros membros vão te ver"
          onChange={(e) => setValue(e.target.value)}
          autoFocus
        />
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={saving}>
            Cancelar
          </button>
          <button type="button" onClick={handleSave} disabled={saving}>
            {saving ? 'Salvando…' : 'Salvar'}
          </button>
        </div>
      </div>
    </div>
  )
}
