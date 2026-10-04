import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import i18n from '../i18n'
import { errorMessage, LocalizedError } from '../lib/apiError'
import { MIN_PASSPHRASE_LENGTH } from '../crypto/keyBackup'
import type { E2EKeysResult } from '../hooks/useE2EKeys'
import './Dialog.css'
import './E2EKeyPanel.css'

interface E2EKeyPanelProps {
  e2e: E2EKeysResult
}

type Mode = 'setup' | 'unlock' | 'reset'

// Painel mostrado no lugar da conversa de DM enquanto a chave de E2E da conta
// não está pronta neste dispositivo: criar a frase de recuperação, digitá-la
// num dispositivo novo, ou trocar a chave quando a frase foi esquecida (ver
// hooks/useE2EKeys.ts e docs/architecture.md, "Decisão: backup da chave de
// E2E com frase de recuperação").
export function E2EKeyPanel({ e2e }: E2EKeyPanelProps) {
  const { t } = useTranslation()
  const [forgot, setForgot] = useState(false)
  const [passphrase, setPassphrase] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState<unknown>()
  const [busy, setBusy] = useState(false)

  if (e2e.status === 'loading' || e2e.status === 'ready') {
    return (
      <div className="empty-state">
        <p>{t('e2e.preparing')}</p>
      </div>
    )
  }

  if (e2e.status === 'error') {
    return (
      <div className="empty-state">
        <div className="dialog-card e2e-key-card">
          <h2>{t('e2e.unavailable')}</h2>
          <p className="dialog-error">{e2e.error}</p>
          <div className="dialog-actions">
            <button type="button" className="dialog-submit" onClick={e2e.retry}>
              {t('common.retry')}
            </button>
          </div>
        </div>
      </div>
    )
  }

  const mode: Mode = e2e.status === 'needs-setup' ? 'setup' : forgot ? 'reset' : 'unlock'
  const choosing = mode !== 'unlock'

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(undefined)
    if (choosing) {
      if (passphrase.trim().length < MIN_PASSPHRASE_LENGTH) {
        setError(new LocalizedError(() => i18n.t('e2e.phraseTooShort', { min: MIN_PASSPHRASE_LENGTH })))
        return
      }
      if (passphrase !== confirmation) {
        setError(new LocalizedError(() => i18n.t('e2e.phrasesDontMatch')))
        return
      }
    }
    setBusy(true)
    try {
      if (mode === 'setup') await e2e.setupPassphrase(passphrase)
      else if (mode === 'reset') await e2e.resetWithNewPassphrase(passphrase)
      else await e2e.unlock(passphrase)
    } catch (err) {
      setError(err)
      setPassphrase('')
      setConfirmation('')
    } finally {
      setBusy(false)
    }
  }

  function toggleForgot() {
    setForgot((f) => !f)
    setError(undefined)
    setPassphrase('')
    setConfirmation('')
  }

  return (
    <div className="empty-state">
      <form className="dialog-card e2e-key-card" onSubmit={handleSubmit}>
        {mode === 'setup' && (
          <>
            <h2>{t('e2e.setupTitle')}</h2>
            <p>{t('e2e.setupBody')}</p>
            {e2e.publishedElsewhere && (
              <div className="dialog-danger-zone">
                <p>{t('e2e.publishedElsewhere')}</p>
              </div>
            )}
          </>
        )}
        {mode === 'unlock' && (
          <>
            <h2>{t('e2e.unlockTitle')}</h2>
            <p>{t('e2e.unlockBody')}</p>
          </>
        )}
        {mode === 'reset' && (
          <>
            <h2>{t('e2e.resetTitle')}</h2>
            <div className="dialog-danger-zone">
              <p>{t('e2e.resetBody')}</p>
            </div>
          </>
        )}

        <label>
          {choosing ? t('e2e.recoveryPhrase') : t('e2e.phrase')}
          <input
            type="password"
            value={passphrase}
            autoComplete={choosing ? 'new-password' : 'current-password'}
            onChange={(e) => setPassphrase(e.target.value)}
            disabled={busy}
            autoFocus
          />
        </label>
        {choosing && (
          <label>
            {t('e2e.repeatPhrase')}
            <input
              type="password"
              value={confirmation}
              autoComplete="new-password"
              onChange={(e) => setConfirmation(e.target.value)}
              disabled={busy}
            />
          </label>
        )}
        {error !== undefined && <p className="dialog-error">{errorMessage(error, t('e2e.prepareKeyFailed'))}</p>}

        {mode !== 'setup' && (
          <button type="button" className="dialog-danger-link" onClick={toggleForgot} disabled={busy}>
            {mode === 'unlock' ? t('e2e.forgotPhrase') : t('e2e.backToPhrase')}
          </button>
        )}
        <div className="dialog-actions">
          <button type="submit" className="dialog-submit" disabled={busy || passphrase === ''}>
            {busy
              ? mode === 'unlock'
                ? t('e2e.unlocking')
                : t('e2e.protecting')
              : mode === 'setup'
                ? t('e2e.createPhrase')
                : mode === 'reset'
                  ? t('e2e.createNewKey')
                  : t('e2e.unlock')}
          </button>
        </div>
      </form>
    </div>
  )
}
