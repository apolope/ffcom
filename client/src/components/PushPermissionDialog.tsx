import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import './Dialog.css'

interface PushPermissionDialogProps {
  // Pede a permissão do Android (ou abre as configurações de notificação).
  onAllow: () => void
  // "Agora não": não pergunta de novo nesta instalação.
  onDecline: () => void
}

// Explicação curta antes do pedido de permissão de notificação no app
// Android, uma vez depois do login. Ver hooks/useAndroidPush.ts e
// docs/architecture.md, "Decisão: notificações push no app Android (fase 6,
// client)".
export function PushPermissionDialog({ onAllow, onDecline }: PushPermissionDialogProps) {
  const { t } = useTranslation()

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    onAllow()
  }

  return (
    <div className="dialog-overlay" onClick={onDecline}>
      <form className="dialog-card" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <h2>{t('push.permission.title')}</h2>
        <p className="dialog-hint">{t('push.permission.body')}</p>
        <div className="dialog-actions">
          <button type="button" onClick={onDecline}>
            {t('push.permission.notNow')}
          </button>
          <button type="submit" className="dialog-submit" autoFocus>
            {t('push.permission.allow')}
          </button>
        </div>
      </form>
    </div>
  )
}
