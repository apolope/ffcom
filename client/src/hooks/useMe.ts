import { useCallback, useEffect, useState } from 'react'
import { fetchMe, updateMyNickname, type Me } from '../lib/serverChannelApi'

interface UseMeResult {
  me: Me | undefined
  setNickname: (nickname: string | undefined) => Promise<void>
}

// Carrega o membro autenticado no server-channel selecionado (permissão
// base + isOwner), usado só para decidir se a UI de administração aparece
// (ver docs/architecture.md, "Sistema de permissões/roles por servidor e
// por canal") — o servidor sempre reforça a permissão de fato em cada rota.
// setNickname também é exposto daqui, já que é a mesma linha (PATCH
// /api/me) que devolve o Me atualizado.
export function useMe(serverBaseUrl: string, accessToken: string): UseMeResult {
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

  const setNickname = useCallback(
    async (nickname: string | undefined) => {
      if (!serverBaseUrl || !accessToken) return
      const updated = await updateMyNickname(serverBaseUrl, accessToken, nickname)
      setMe(updated)
    },
    [serverBaseUrl, accessToken],
  )

  return { me, setNickname }
}
