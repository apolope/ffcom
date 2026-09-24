import { useRef, useState } from 'react'
import { AvatarCropper } from './AvatarCropper'
import { UserAvatar } from './UserAvatar'
import type { MyProfile } from '../lib/serverCentralApi'
import './Dialog.css'

interface AvatarDialogProps {
  profile: MyProfile | undefined
  onUpload: (file: File) => Promise<void>
  onRemove: () => Promise<void>
  onClose: () => void
}

// A imagem escolhida passa pelo recorte (AvatarCropper) e sai com no máximo
// 512 px, bem abaixo do AVATAR_MAX_MB do servidor; este limite só evita abrir
// arquivos enormes no navegador.
const MAX_SOURCE_MB = 20

// Upload/remoção do avatar de conta (server-central, ver
// docs/architecture.md, "Decisão: upload de avatar de conta" e "Decisão:
// recorte do avatar no client"). O servidor reforça o próprio limite
// (AVATAR_MAX_MB) e não confia no client.
export function AvatarDialog({ profile, onUpload, onRemove, onClose }: AvatarDialogProps) {
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  // Imagem escolhida, esperando o recorte.
  const [cropFile, setCropFile] = useState<File>()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const displayName = profile?.displayName ?? profile?.oidcSubject ?? ''

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return

    if (file.size > MAX_SOURCE_MB * 1024 * 1024) {
      setError(`imagem maior que o limite de ${MAX_SOURCE_MB}MB`)
      return
    }

    setError(undefined)
    setCropFile(file)
  }

  async function handleCropped(blob: Blob) {
    setError(undefined)
    setBusy(true)
    try {
      const extension = blob.type === 'image/webp' ? 'webp' : 'png'
      await onUpload(new File([blob], `avatar.${extension}`, { type: blob.type }))
      setCropFile(undefined)
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

  if (cropFile) {
    return (
      <div className="dialog-overlay" onClick={busy ? undefined : () => setCropFile(undefined)}>
        <div className="dialog-card avatar-dialog-cropping" onClick={(e) => e.stopPropagation()}>
          <h2>Recortar avatar</h2>
          <AvatarCropper
            file={cropFile}
            busy={busy}
            onCancel={() => setCropFile(undefined)}
            onConfirm={(blob) => void handleCropped(blob)}
          />
          {error && <p className="dialog-error">{error}</p>}
          {cropFile.type === 'image/gif' && (
            <p className="hint">GIF animado vira uma imagem parada no recorte.</p>
          )}
        </div>
      </div>
    )
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
