import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useMenuDismiss } from '../hooks/useMenuDismiss'
import './RailMenu.css'

interface ChannelMenuProps {
  anchor: HTMLElement
  channelName: string
  muted: boolean
  onToggleMute: () => void
  onClose: () => void
}

// Menu de contexto de um canal de texto ou fórum (botão direito, ou tocar e
// segurar no celular). Hoje só tem "Silenciar notificações", e por isso só
// existe onde há push (app Android; ver hooks/useAndroidPush.ts). Mesmo
// visual e fechamento do ServerMenu, aberto logo abaixo do canal.
export function ChannelMenu({ anchor, channelName, muted, onToggleMute, onClose }: ChannelMenuProps) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDivElement>(null)
  useMenuDismiss(ref, anchor, onClose)
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('button')?.focus()
  }, [])

  const rect = anchor.getBoundingClientRect()
  return (
    <div
      ref={ref}
      className="rail-menu"
      role="menu"
      aria-label={t('channels.menuLabel', { name: channelName })}
      style={{ left: rect.left + 8, top: rect.bottom + 4 }}
    >
      <button
        type="button"
        role="menuitem"
        className="rail-menu-item"
        onClick={() => {
          onToggleMute()
          onClose()
        }}
      >
        {muted ? t('push.mute.unmute') : t('push.mute.mute')}
      </button>
    </div>
  )
}
