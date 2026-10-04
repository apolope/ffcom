import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { errorMessage } from '../lib/apiError'
import './Dialog.css'

interface NicknameDialogProps {
  currentNickname: string | undefined
  onSave: (nickname: string | undefined) => Promise<void>
  onClose: () => void
}

// Define o apelido exibido na lista de membros deste server-channel (ver
// docs/architecture.md, endpoint PATCH /api/me novo em
// server-channel/internal/httpapi/me.go) — sem apelido, a lista cai no
// UUID truncado do membro (ver hooks/useServerMembers.ts).
export function NicknameDialog({ currentNickname, onSave, onClose }: NicknameDialogProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState(currentNickname ?? '')
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
      <div className="dialog-card" onClick={(e) => e.stopPropagation()}>
        <h2>{t('profile.nickname.title')}</h2>
        <input
          type="text"
          value={value}
          maxLength={64}
          placeholder={t('profile.nickname.placeholder')}
          onChange={(e) => setValue(e.target.value)}
          autoFocus
        />
        {error !== undefined && (
          <p className="dialog-error">{errorMessage(error, t('profile.nickname.saveFailed'))}</p>
        )}
        <div className="dialog-actions">
          <button type="button" onClick={onClose} disabled={saving}>
            {t('common.cancel')}
          </button>
          <button type="button" onClick={handleSave} disabled={saving}>
            {saving ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </div>
    </div>
  )
}
