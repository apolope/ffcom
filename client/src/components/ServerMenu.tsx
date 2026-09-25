import { useEffect, useRef } from 'react'
import { useMenuDismiss } from '../hooks/useMenuDismiss'
import './RailMenu.css'

// Ações do servidor aberto. Cada uma vem undefined sem a permissão
// correspondente (o servidor reforça de novo em cada rota); loading enquanto
// o /api/me do servidor ainda não voltou, para não mostrar "sem ações" à toa.
export interface ServerMenuActions {
  loading: boolean
  onManageMembers?: () => void
  onInvite?: () => void
}

interface ServerMenuProps {
  anchor: HTMLElement
  serverName: string
  actions: ServerMenuActions
  onClose: () => void
}

// Menu de contexto (botão direito) do ícone do servidor no ServerRail.
// Mesma posição fixa e fechamento do StatusMenu, mas alinhado pelo topo do
// ícone, já que os servidores ficam no alto do rail.
export function ServerMenu({ anchor, serverName, actions, onClose }: ServerMenuProps) {
  const ref = useRef<HTMLDivElement>(null)
  useMenuDismiss(ref, anchor, onClose)
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('button')?.focus()
  }, [actions.loading])

  const rect = anchor.getBoundingClientRect()
  const items: { label: string; run: () => void }[] = []
  if (actions.onManageMembers) items.push({ label: 'Membros', run: actions.onManageMembers })
  if (actions.onInvite) items.push({ label: 'Convidar', run: actions.onInvite })

  return (
    <div
      ref={ref}
      className="rail-menu"
      role="menu"
      aria-label={`Configurações de ${serverName}`}
      style={{ left: rect.right + 8, top: rect.top }}
    >
      {actions.loading ? (
        <span className="rail-menu-empty">Carregando…</span>
      ) : items.length === 0 ? (
        <span className="rail-menu-empty">Nenhuma ação disponível</span>
      ) : (
        items.map(({ label, run }) => (
          <button
            key={label}
            type="button"
            role="menuitem"
            className="rail-menu-item"
            onClick={() => {
              run()
              onClose()
            }}
          >
            {label}
          </button>
        ))
      )}
    </div>
  )
}
