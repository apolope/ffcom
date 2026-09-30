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
//
// O campo "Endereço" também aceita colar o link gerado por
// InviteServerDialog (endereço + `?invite=CODE`) — parseInviteLink separa os
// dois de volta, para quem só tem o link não precisar copiar/colar duas
// vezes. A separação acontece ao colar, ao sair do campo e no envio, nunca
// a cada tecla: digitando, `?invite=J` já é um link válido e o resto do
// código iria parar no fim do endereço. Ver docs/architecture.md, "Decisão: convite auto-contido".
function parseInviteLink(value: string): { address: string; inviteCode: string; name?: string } | undefined {
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return undefined
  }
  const code = url.searchParams.get('invite')
  if (!code) return undefined
  // Links anteriores ao `&name=` não trazem o nome; a pessoa digita.
  const name = url.searchParams.get('name')?.trim() || undefined
  url.search = ''
  return { address: url.toString().replace(/\/+$/, ''), inviteCode: code, name }
}

// TLS obrigatório fora de localhost (ver docs/architecture.md, "Criptografia
// em trânsito obrigatória") — http:// só é aceito contra a própria máquina,
// caso de desenvolvimento local; qualquer outro endereço precisa ser https://.
const LOCAL_HOSTNAMES = new Set(['localhost', '127.0.0.1', '::1'])

function isAddressSecure(address: string): boolean {
  let url: URL
  try {
    url = new URL(address)
  } catch {
    return false
  }
  if (url.protocol === 'https:') return true
  return url.protocol === 'http:' && LOCAL_HOSTNAMES.has(url.hostname)
}

export function AddServerDialog({ onAdd, onClose }: AddServerDialogProps) {
  const [address, setAddress] = useState('')
  const [name, setName] = useState('')
  const [inviteCode, setInviteCode] = useState('')
  const [error, setError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)

  // Devolve o endereço sem o `?invite=` (e preenche o código) quando o
  // valor é um link de convite; senão devolve o valor como veio.
  function splitInviteLink(value: string): string {
    const parsed = parseInviteLink(value.trim())
    if (!parsed) return value
    setAddress(parsed.address)
    setInviteCode(parsed.inviteCode)
    // Sugestão de quem convidou; não passa por cima do que a pessoa já digitou.
    if (parsed.name && !name.trim()) setName(parsed.name)
    return parsed.address
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setError(undefined)
    // Link digitado e enviado com Enter, sem passar pelo onBlur.
    const parsed = parseInviteLink(address.trim())
    const trimmedAddress = (parsed?.address ?? address).trim()
    const code = parsed?.inviteCode ?? inviteCode
    const serverName = name.trim() || parsed?.name || ''
    if (!isAddressSecure(trimmedAddress)) {
      setError('Endereço precisa usar https:// (http:// só é aceito para localhost)')
      return
    }
    setSubmitting(true)
    try {
      await onAdd(trimmedAddress, serverName, code.trim() || undefined)
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
            placeholder="https://… ou um link de convite"
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            onPaste={(e) => {
              const pasted = e.clipboardData.getData('text')
              if (parseInviteLink(pasted.trim())) {
                e.preventDefault()
                splitInviteLink(pasted)
              }
            }}
            onBlur={() => splitInviteLink(address)}
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
