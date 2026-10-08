import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import './Dialog.css'

interface InstallPermissionDialogProps {
  // Leva às configurações do Android ("Permitir desta fonte").
  onAllow: () => void
  onClose: () => void
}

// Aviso curto antes da primeira atualização do APK: o Android só deixa o app
// abrir o instalador depois que a pessoa libera "Instalar apps desconhecidos"
// para o FFCom. Ver hooks/useAppUpdate.ts e docs/architecture.md, "Decisão:
// atualização do APK (fase 2)".
export function InstallPermissionDialog({ onAllow, onClose }: InstallPermissionDialogProps) {
  const { t } = useTranslation()

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    onAllow()
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <form className="dialog-card" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <h2>{t('update.installPermission.title')}</h2>
        <p className="dialog-hint">{t('update.installPermission.body')}</p>
        <div className="dialog-actions">
          <button type="button" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="dialog-submit" autoFocus>
            {t('update.installPermission.openSettings')}
          </button>
        </div>
      </form>
    </div>
  )
}
