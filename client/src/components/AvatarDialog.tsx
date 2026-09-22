import { useRef, useState } from 'react'
import { UserAvatar } from './UserAvatar'
import type { MyProfile } from '../lib/serverCentralApi'
import './Dialog.css'

interface AvatarDialogProps {
  profile: MyProfile | undefined
  onUpload: (file: File) => Promise<void>
  onRemove: () => Promise<void>
  onClose: () => void
}

const MAX_AVATAR_MB = 2

// Upload/remoção do avatar de conta (server-central, ver
// docs/architecture.md, "Decisão: upload de avatar de conta"). Limite de
// tamanho aqui é só uma checagem antecipada pra UX -- o servidor reforça o
// próprio limite (AVATAR_MAX_MB) e não confia neste valor.
export function AvatarDialog({ profile, onUpload, onRemove, onClose }: AvatarDialogProps) {
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const displayName = profile?.displayName ?? profile?.oidcSubject ?? ''

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return

    if (file.size > MAX_AVATAR_MB * 1024 * 1024) {
      setError(`imagem maior que o limite de ${MAX_AVATAR_MB}MB`)
      return
    }

    setError(undefined)
    setBusy(true)
    try {
      await onUpload(file)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'falha ao enviar avatar')
    } finally {
      setBusy(false)
    }
  }

  async function handleRemove() {
    setError(undefined)
    setBusy(true)
    try {
      await onRemove()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'falha ao remover avatar')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog-card" onClick={(e) => e.stopPropagation()}>
        <h2>Seu avatar</h2>
        <div className="avatar-dialog-preview">
          <UserAvatar avatarUrl={profile?.avatarUrl} displayName={displayName || '?'} size={72} />
        </div>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif"
          onChange={handleFileChange}
          disabled={busy}
        />
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-actions">
          {profile?.avatarUrl && (
            <button type="button" onClick={handleRemove} disabled={busy}>
              Remover avatar
            </button>
          )}
          <button type="button" onClick={onClose} disabled={busy}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  )
}
