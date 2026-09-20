import { useState } from 'react'
import type { FormEvent } from 'react'
import './AddServerDialog.css'

interface AddServerDialogProps {
  onAdd: (address: string, name: string) => Promise<void>
  onClose: () => void
}

// Adiciona um server-channel ao diretório da conta via endereço informado
// manualmente (ver docs/architecture.md, "Decisão: descoberta de
// server-channel" — sem descoberta automática, só convite/IP manual).
export function AddServerDialog({ onAdd, onClose }: AddServerDialogProps) {
  const [address, setAddress] = useState('')
  const [name, setName] = useState('')
  const [error, setError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setError(undefined)
    setSubmitting(true)
    try {
      await onAdd(address.trim(), name.trim())
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'falha ao adicionar servidor')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <form
        className="dialog-card"
        onClick={(e) => e.stopPropagation()}
        onSubmit={handleSubmit}
      >
        <h2>Adicionar servidor</h2>
        <label>
          Endereço
          <input
            type="text"
            placeholder="http://localhost:8080"
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            required
            autoFocus
          />
        </label>
        <label>
          Nome
          <input
            type="text"
            placeholder="Nome do servidor"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </label>
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={submitting}>
            Cancelar
          </button>
          <button type="submit" className="dialog-submit" disabled={submitting}>
            {submitting ? 'Adicionando…' : 'Adicionar'}
          </button>
        </div>
      </form>
    </div>
  )
}
