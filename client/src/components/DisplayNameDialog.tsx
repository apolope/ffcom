import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { errorMessage } from '../lib/apiError'
import './Dialog.css'

interface DisplayNameDialogProps {
  // Nome escolhido no FFCom, se houver (MyProfile.customDisplayName).
  currentName: string | undefined
  // Nome do perfil do Authentik, usado quando o campo fica vazio.
  authentikName: string | undefined
  onSave: (displayName: string | undefined) => Promise<void>
  onClose: () => void
}

// Nome de exibição da conta (server-central): o que amigos e DMs mostram e
// o nome nos servidores onde a pessoa não tem apelido. Vazio volta ao nome
// do Authentik. Ver docs/architecture.md, "Decisão: nome de exibição da
// conta".
export function DisplayNameDialog({ currentName, authentikName, onSave, onClose }: DisplayNameDialogProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState(currentName ?? '')
  const [error, setError] = useState<unknown>()
  const [saving, setSaving] = useState(false)

  async function handleSave() {
    setError(undefined)
    setSaving(true)
    try {
      await onSave(value.trim() || undefined)
      onClose()
    } catch (err) {
      setError(err)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <form
        className="dialog-card"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault()
          if (!saving) void handleSave()
        }}
      >
        <h2>{t('profile.displayName.title')}</h2>
        <input
          type="text"
          value={value}
          maxLength={64}
          placeholder={authentikName ?? t('profile.displayName.placeholder')}
          onChange={(e) => setValue(e.target.value)}
          autoFocus
        />
        <p className="dialog-hint">
          {t('profile.displayName.hint')}
          {authentikName && <> {t('profile.displayName.emptyHint', { name: authentikName })}</>}
        </p>
        {error !== undefined && (
          <p className="dialog-error">{errorMessage(error, t('profile.displayName.saveFailed'))}</p>
        )}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="dialog-submit" disabled={saving}>
            {saving ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </form>
    </div>
  )
}
