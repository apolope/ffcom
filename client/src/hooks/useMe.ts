import { useEffect, useState } from 'react'
import { fetchMe, type Me } from '../lib/serverChannelApi'

// Carrega o membro autenticado no server-channel selecionado (permissão
// base + isOwner), usado só para decidir se a UI de administração aparece
// (ver docs/architecture.md, "Sistema de permissões/roles por servidor e
// por canal") — o servidor sempre reforça a permissão de fato em cada rota.
export function useMe(serverBaseUrl: string, accessToken: string): Me | undefined {
  const [me, setMe] = useState<Me>()

  useEffect(() => {
    setMe(undefined)
    if (!serverBaseUrl || !accessToken) return

    let cancelled = false
    fetchMe(serverBaseUrl, accessToken)
      .then((remote) => {
        if (!cancelled) setMe(remote)
      })
      .catch(() => {
        /* sem permissão de admin nenhuma se /api/me falhar */
      })

    return () => {
      cancelled = true
    }
  }, [serverBaseUrl, accessToken])

  return me
}
