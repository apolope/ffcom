import type { AppNotification } from '../hooks/useNotificationCenter'
import './NotificationStack.css'

interface NotificationStackProps {
  notifications: AppNotification[]
  onDismiss: (id: number) => void
}

// Pilha de mensagens no canto inferior direito (ver
// hooks/useNotificationCenter.ts). Em ordem de chegada: a mais antiga fica
// no alto e sai primeiro.
export function NotificationStack({ notifications, onDismiss }: NotificationStackProps) {
  return (
    <div className="notification-stack" aria-live="polite">
      {notifications.map((n) => (
        <div key={n.id} className={`notification notification-${n.kind}`} role={n.kind === 'error' ? 'alert' : 'status'}>
          <span className="notification-text">{n.text}</span>
          <button type="button" className="notification-close" aria-label="Fechar aviso" onClick={() => onDismiss(n.id)}>
            ×
          </button>
        </div>
      ))}
    </div>
  )
}
