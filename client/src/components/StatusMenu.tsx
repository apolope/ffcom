import { useEffect, useRef } from 'react'
import type { ChosenStatus } from '../types'
import { STATUS_LABELS } from './PresenceContext'
import './StatusMenu.css'

const OPTIONS: { status: ChosenStatus; hint?: string }[] = [
  { status: 'online' },
  { status: 'busy' },
  { status: 'away' },
  { status: 'invisible', hint: 'Aparece offline para os amigos' },
]

interface StatusMenuProps {
  // O botão do avatar: dá a posição e não conta como "clique fora" (o
  // próprio botão já alterna o menu).
  anchor: HTMLElement
  chosen: ChosenStatus
  onChoose: (status: ChosenStatus) => void
  onEditAvatar: () => void
  onClose: () => void
}

// Menu do próprio avatar no ServerRail: escolher o status e abrir o diálogo
// de avatar. Posição fixa ao lado do botão, porque o rail rola
// (overflow-y) e cortaria um menu posicionado dentro dele. Fecha com Esc ou
// clique fora. Ver docs/architecture.md, "Decisão: status de presença e
// avatar nas listas de membros".
export function StatusMenu({ anchor, chosen, onChoose, onEditAvatar, onClose }: StatusMenuProps) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    const onPointer = (event: PointerEvent) => {
      const target = event.target as Node
      if (!ref.current?.contains(target) && !anchor.contains(target)) onClose()
    }
    window.addEventListener('keydown', onKey)
    // Captura: o clique que abriu o menu já passou, e o próximo fora dele
    // fecha antes de chegar a outro botão.
    window.addEventListener('pointerdown', onPointer, true)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('pointerdown', onPointer, true)
    }
  }, [anchor, onClose])

  const rect = anchor.getBoundingClientRect()

  return (
    <div
      ref={ref}
      className="status-menu"
      role="menu"
      aria-label="Seu status"
      style={{ left: rect.right + 8, bottom: Math.max(8, window.innerHeight - rect.bottom) }}
    >
      {OPTIONS.map(({ status, hint }) => (
        <button
          key={status}
          type="button"
          role="menuitemradio"
          aria-checked={status === chosen}
          className="status-menu-item"
          onClick={() => {
            onChoose(status)
            onClose()
          }}
        >
          <span className={`status-menu-dot presence-${status === 'invisible' ? 'offline' : status}`} aria-hidden="true" />
          <span>
            {STATUS_LABELS[status]}
            {hint && <span className="status-menu-hint">{hint}</span>}
          </span>
        </button>
      ))}
      <hr />
      <button
        type="button"
        role="menuitem"
        className="status-menu-item"
        onClick={() => {
          onEditAvatar()
          onClose()
        }}
      >
        Alterar avatar
      </button>
    </div>
  )
}
