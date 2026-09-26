import { useState, type FormEvent } from 'react'
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
  const [forgot, setForgot] = useState(false)
  const [passphrase, setPassphrase] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)

  if (e2e.status === 'loading' || e2e.status === 'ready') {
    return (
      <div className="empty-state">
        <p>Preparando a chave de criptografia…</p>
      </div>
    )
  }

  if (e2e.status === 'error') {
    return (
      <div className="empty-state">
        <div className="dialog-card e2e-key-card">
          <h2>Criptografia indisponível</h2>
          <p className="dialog-error">{e2e.error}</p>
          <div className="dialog-actions">
            <button type="button" className="dialog-submit" onClick={e2e.retry}>
              Tentar de novo
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
        setError(`A frase precisa ter pelo menos ${MIN_PASSPHRASE_LENGTH} caracteres.`)
        return
      }
      if (passphrase !== confirmation) {
        setError('As duas frases não são iguais.')
        return
      }
    }
    setBusy(true)
    try {
      if (mode === 'setup') await e2e.setupPassphrase(passphrase)
      else if (mode === 'reset') await e2e.resetWithNewPassphrase(passphrase)
      else await e2e.unlock(passphrase)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Falha ao preparar a chave.')
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
            <h2>Crie sua frase de recuperação</h2>
            <p>
              Ela protege a chave que cifra suas mensagens diretas. Você vai digitá-la uma vez em cada
              dispositivo novo para ler o seu histórico. Ninguém consegue recuperá-la por você, nem quem
              administra o FFCom: se esquecer, as mensagens antigas ficam ilegíveis.
            </p>
            {e2e.publishedElsewhere && (
              <div className="dialog-danger-zone">
                <p>
                  Sua chave atual foi criada em outro dispositivo. Para manter o histórico, crie a frase
                  naquele dispositivo. Se criar aqui, as mensagens antigas ficam ilegíveis.
                </p>
              </div>
            )}
          </>
        )}
        {mode === 'unlock' && (
          <>
            <h2>Digite sua frase de recuperação</h2>
            <p>Ela libera neste dispositivo a chave das suas mensagens diretas.</p>
          </>
        )}
        {mode === 'reset' && (
          <>
            <h2>Criar uma chave nova</h2>
            <div className="dialog-danger-zone">
              <p>
                Sem a frase antiga, o histórico de mensagens diretas fica ilegível em todos os dispositivos,
                e os outros dispositivos vão pedir a frase nova. Mensagens novas funcionam normalmente.
              </p>
            </div>
          </>
        )}

        <label>
          {choosing ? 'Frase de recuperação' : 'Frase'}
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
            Repita a frase
            <input
              type="password"
              value={confirmation}
              autoComplete="new-password"
              onChange={(e) => setConfirmation(e.target.value)}
              disabled={busy}
            />
          </label>
        )}
        {error && <p className="dialog-error">{error}</p>}

        {mode !== 'setup' && (
          <button type="button" className="dialog-danger-link" onClick={toggleForgot} disabled={busy}>
            {mode === 'unlock' ? 'Esqueci a frase' : 'Voltar e digitar a frase'}
          </button>
        )}
        <div className="dialog-actions">
          <button type="submit" className="dialog-submit" disabled={busy || passphrase === ''}>
            {busy
              ? mode === 'unlock'
                ? 'Desbloqueando…'
                : 'Protegendo a chave…'
              : mode === 'setup'
                ? 'Criar frase'
                : mode === 'reset'
                  ? 'Criar chave nova'
                  : 'Desbloquear'}
          </button>
        </div>
      </form>
    </div>
  )
}
