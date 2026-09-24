import { useEffect, useState, useSyncExternalStore } from 'react'
import { useAuth } from '../auth/AuthProvider'
import { avatarCacheVersion, loadAvatar, subscribeAvatarCache } from '../lib/avatarCache'
import './UserAvatar.css'

interface UserAvatarProps {
  avatarUrl?: string
  displayName: string
  size?: number
}

// Avatar de conta (server-central), com fallback pra inicial do nome. O
// endpoint exige Bearer token (ver docs/architecture.md, "Decisão: upload
// de avatar de conta"), então não dá pra apontar um <img src="..."> direto
// -- busca como Blob e gera uma object URL local, guardada no cache da
// sessão (lib/avatarCache.ts) e compartilhada por todas as listas.
export function UserAvatar({ avatarUrl, displayName, size = 32 }: UserAvatarProps) {
  const { accessToken } = useAuth()
  const [blobUrl, setBlobUrl] = useState<string>()
  const cacheVersion = useSyncExternalStore(subscribeAvatarCache, avatarCacheVersion)

  useEffect(() => {
    setBlobUrl(undefined)
    if (!avatarUrl || !accessToken) return

    let cancelled = false
    loadAvatar(avatarUrl, accessToken)
      .then((url) => {
        if (!cancelled) setBlobUrl(url)
      })
      .catch(() => {
        /* falha ao carregar -- cai pro fallback de inicial */
      })

    return () => {
      cancelled = true
    }
  }, [avatarUrl, accessToken, cacheVersion])

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
