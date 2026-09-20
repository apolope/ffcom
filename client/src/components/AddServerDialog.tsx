import { useState } from 'react'
import type { FormEvent } from 'react'
import './Dialog.css'

interface AddServerDialogProps {
  onAdd: (address: string, name: string, inviteCode?: string) => Promise<void>
  onClose: () => void
}

// Adiciona um server-channel ao diretório da conta via endereço informado
// manualmente (ver docs/architecture.md, "Decisão: descoberta de
// server-channel" — sem descoberta automática, só convite/IP manual).
// Entrar de fato no server-channel agora exige um convite válido, a menos
// que ninguém ainda seja membro (bootstrap do self-host) — ver
// docs/architecture.md, "Convites obrigatórios para entrar em
// server-channel". O campo de código é opcional na UI porque cobre os dois
// casos sem exigir que quem está configurando um servidor novo saiba disso.
export function AddServerDialog({ onAdd, onClose }: AddServerDialogProps) {
  const [address, setAddress] = useState('')
  const [name, setName] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [error, setError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setError(undefined)
    setSubmitting(true)
    try {
      await onAdd(address.trim(), name.trim(), inviteCode.trim() || undefined)
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
        <label>
          Código de convite
          <input
            type="text"
            placeholder="Deixe em branco se você for o dono deste servidor"
            value={inviteCode}
            onChange={(e) => setInviteCode(e.target.value)}
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
