import { useEffect, useState } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { fetchAvatarBlob } from '../lib/serverCentralApi'
import './UserAvatar.css'

interface UserAvatarProps {
  avatarUrl?: string
  displayName: string
  size?: number
}

// Avatar de conta (server-central), com fallback pra inicial do nome. O
// endpoint exige Bearer token (ver docs/architecture.md, "Decisão: upload
// de avatar de conta"), então não dá pra apontar um <img src="..."> direto
// -- busca como Blob e gera uma object URL local, revogada quando o
// component desmonta ou avatarUrl muda (mesmo padrão de
// components/MessageAttachment.tsx em server-channel).
export function UserAvatar({ avatarUrl, displayName, size = 32 }: UserAvatarProps) {
  const { accessToken } = useAuth()
  const [blobUrl, setBlobUrl] = useState<string>()

  useEffect(() => {
    setBlobUrl(undefined)
    if (!avatarUrl || !accessToken) return

    let cancelled = false
    let url: string | undefined
    fetchAvatarBlob(avatarUrl, accessToken)
      .then((blob) => {
        if (cancelled) return
        url = URL.createObjectURL(blob)
        setBlobUrl(url)
      })
      .catch(() => {
        /* falha ao carregar -- cai pro fallback de inicial */
      })

    return () => {
      cancelled = true
      if (url) URL.revokeObjectURL(url)
    }
  }, [avatarUrl, accessToken])

  const style = { width: size, height: size, fontSize: Math.round(size * 0.45) }

  if (blobUrl) {
    return (
      <img
        className="user-avatar"
        src={blobUrl}
        alt={displayName}
        style={style}
        width={size}
        height={size}
      />
    )
  }
  return (
    <span className="user-avatar user-avatar-fallback" style={style} aria-hidden="true">
      {initial(displayName)}
    </span>
  )
}

function initial(name: string): string {
  const trimmed = name.trim()
  return trimmed ? trimmed[0].toUpperCase() : '?'
}
