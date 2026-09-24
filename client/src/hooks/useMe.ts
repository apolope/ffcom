import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchMe, updateMyNickname, updateMyProfileName, type Me } from '../lib/serverChannelApi'

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
//
// profileName é o nome do perfil do Authentik desta pessoa: se o servidor
// guarda outro (ou nenhum), o hook grava o atual e chama onProfileNameSaved,
// para a lista de membros recarregar com o nome novo. Ver
// docs/architecture.md, "Decisão: nome exibido do membro".
export function useMe(
  serverBaseUrl: string,
  accessToken: string,
  profileName: string | undefined,
  onProfileNameSaved?: () => void,
): UseMeResult {
  const [me, setMe] = useState<Me>()
  const onProfileNameSavedRef = useRef(onProfileNameSaved)
  useEffect(() => {
    onProfileNameSavedRef.current = onProfileNameSaved
  }, [onProfileNameSaved])

  useEffect(() => {
    setMe(undefined)
    if (!serverBaseUrl || !accessToken) return

    let cancelled = false
    fetchMe(serverBaseUrl, accessToken)
      .then(async (remote) => {
        if (cancelled) return
        setMe(remote)
        if (!profileName || remote.profileName === profileName) return
        const updated = await updateMyProfileName(serverBaseUrl, accessToken, profileName)
        if (cancelled) return
        setMe(updated)
        onProfileNameSavedRef.current?.()
      })
      .catch(() => {
        /* sem permissão de admin nenhuma se /api/me falhar */
      })

    return () => {
      cancelled = true
    }
  }, [serverBaseUrl, accessToken, profileName])

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
