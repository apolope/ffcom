import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError } from '../lib/apiError'
import { fetchMe, updateMyNickname, updateMyProfileName, type Me } from '../lib/serverChannelApi'

interface UseMeResult {
  me: Me | undefined
  // A conta não é (ou deixou de ser) membro deste server-channel: expulsa,
  // banida ou que nunca entrou, mas com o servidor ainda na lista do
  // server-central. Ver NotMemberPanel.
  notMember: boolean
  // Busca /api/me de novo: depois de entrar com convite, ou quando outra
  // rota deste servidor respondeu 403 e pode ser que a pessoa tenha sido
  // expulsa com o servidor aberto.
  reload: () => void
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
//
// /api/me não exige nenhum bit de permissão, então um 403 nele só acontece
// quando auth.RequireMember recusa: a conta não é membro. É o sinal usado
// para notMember, sem depender do texto do erro. Ver docs/architecture.md,
// "Decisão: aviso de membro expulso no lugar da tela vazia".
export function useMe(
  serverBaseUrl: string,
  accessToken: string,
  profileName: string | undefined,
  onProfileNameSaved?: () => void,
): UseMeResult {
  const [me, setMe] = useState<Me>()
  const [notMember, setNotMember] = useState(false)
  const [reloadToken, setReloadToken] = useState(0)
  const reload = useCallback(() => setReloadToken((n) => n + 1), [])
  const onProfileNameSavedRef = useRef(onProfileNameSaved)
  useEffect(() => {
    onProfileNameSavedRef.current = onProfileNameSaved
  }, [onProfileNameSaved])

  // Trocar de servidor zera o estado; um reload() mantém o que está na tela
  // até a resposta chegar.
  useEffect(() => {
    setMe(undefined)
    setNotMember(false)
  }, [serverBaseUrl, accessToken])

  useEffect(() => {
    if (!serverBaseUrl || !accessToken) return

    let cancelled = false
    fetchMe(serverBaseUrl, accessToken)
      .then(async (remote) => {
        if (cancelled) return
        setMe(remote)
        setNotMember(false)
        if (!profileName || remote.profileName === profileName) return
        const updated = await updateMyProfileName(serverBaseUrl, accessToken, profileName)
        if (cancelled) return
        setMe(updated)
        onProfileNameSavedRef.current?.()
      })
      .catch((err: unknown) => {
        // Outros erros (servidor fora do ar, rede) não dizem nada sobre ser
        // membro: fica sem permissão de admin nenhuma, como antes.
        if (cancelled || !(err instanceof ApiError) || err.status !== 403) return
        setMe(undefined)
        setNotMember(true)
      })

    return () => {
      cancelled = true
    }
  }, [serverBaseUrl, accessToken, profileName, reloadToken])

  const setNickname = useCallback(
    async (nickname: string | undefined) => {
      if (!serverBaseUrl || !accessToken) return
      const updated = await updateMyNickname(serverBaseUrl, accessToken, nickname)
      setMe(updated)
    },
    [serverBaseUrl, accessToken],
  )

  return { me, notMember, reload, setNickname }
}
