import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import i18n from '../i18n'
import {
  dismissPushPermission,
  getPushStatus,
  getPushToken,
  onPushToken,
  registerDevice,
  requestPushPermission,
  syncGrants,
  syncPushTexts,
  unregisterDeviceIfRegistered,
  type PushStatus,
} from '../lib/androidPush'
import { fetchPushMutes, setPushMute, type PushMute } from '../lib/serverCentralApi'
import type { KnownServer } from '../types'

export interface PushMutes {
  isMuted: (serverAddress: string, channelId?: string) => boolean
  // Rejeita com o erro do server-central; o estado só muda depois de gravar.
  setMuted: (serverAddress: string, channelId: string | undefined, muted: boolean) => Promise<void>
}

interface UseAndroidPushResult {
  // Push funcionando neste aparelho (app Android com Firebase). Sem isso,
  // nada do push aparece na interface.
  available: boolean
  // Mostrar a explicação antes do pedido de permissão (PushPermissionDialog).
  askPermission: boolean
  acceptPermission: () => void
  declinePermission: () => void
  mutes: PushMutes
}

const NO_STATUS: PushStatus = { available: false, enabled: false, asked: true }

// Push do app Android depois do login (fase 6; ver lib/androidPush.ts):
// explicação e permissão uma vez, registro do aparelho no server-central,
// grant para cada servidor da lista (de novo quando um entra), textos no
// idioma ativo e os silêncios da conta para os menus. No navegador e no
// desktop o status é "indisponível" e nada roda.
export function useAndroidPush(accessToken: string, accountSub: string, servers: KnownServer[]): UseAndroidPushResult {
  const [status, setStatus] = useState<PushStatus>(NO_STATUS)
  const [device, setDevice] = useState<string>()
  const [mutes, setMutes] = useState<PushMute[]>([])
  const signedIn = !!accessToken && !!accountSub
  // O access token se renova sozinho; os efeitos abaixo não devem rodar de
  // novo a cada renovação, só usar o mais recente.
  const tokenRef = useRef(accessToken)
  useEffect(() => {
    tokenRef.current = accessToken
  }, [accessToken])

  const refreshStatus = useCallback(() => {
    void getPushStatus().then(setStatus)
  }, [])

  // Status ao entrar e ao voltar ao app (a pessoa pode ter ligado ou
  // desligado as notificações nas configurações do Android).
  useEffect(() => {
    if (!signedIn) {
      setStatus(NO_STATUS)
      setDevice(undefined)
      return
    }
    refreshStatus()
    const onVisible = () => {
      if (document.visibilityState === 'visible') refreshStatus()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => document.removeEventListener('visibilitychange', onVisible)
  }, [signedIn, refreshStatus])

  // Textos das notificações no idioma ativo.
  useEffect(() => {
    if (!status.available) return
    syncPushTexts()
    i18n.on('languageChanged', syncPushTexts)
    return () => i18n.off('languageChanged', syncPushTexts)
  }, [status.available])

  // Aparelho na conta enquanto as notificações estão ligadas; fora dela
  // quando a pessoa desliga.
  useEffect(() => {
    if (!signedIn || !status.available) return
    let cancelled = false
    if (!status.enabled) {
      setDevice(undefined)
      unregisterDeviceIfRegistered(tokenRef.current, accountSub).catch((err) =>
        console.warn('[ffcom] push: aparelho não saiu da conta', err),
      )
      return
    }
    const register = (token: string) =>
      registerDevice(tokenRef.current, accountSub, token)
        .then(() => {
          if (!cancelled) setDevice(token)
        })
        .catch((err) => console.warn('[ffcom] push: aparelho não registrado', err))
    getPushToken()
      .then((token) => {
        if (!cancelled) void register(token)
      })
      .catch((err) => console.warn('[ffcom] push: sem token do FCM', err))
    const stop = onPushToken((token) => void register(token))
    return () => {
      cancelled = true
      stop()
    }
  }, [signedIn, status.available, status.enabled, accountSub])

  // Grants: a cada servidor que entra na lista (e quando o token muda).
  const addressesKey = useMemo(() => [...new Set(servers.map((s) => s.baseUrl))].sort().join('\n'), [servers])
  useEffect(() => {
    if (!device || !addressesKey) return
    void syncGrants(tokenRef.current, accountSub, device, addressesKey.split('\n'))
  }, [device, addressesKey, accountSub])

  // Silêncios da conta, para os menus.
  useEffect(() => {
    if (!signedIn || !status.available) {
      setMutes([])
      return
    }
    let cancelled = false
    fetchPushMutes(tokenRef.current)
      .then((list) => {
        if (!cancelled) setMutes(list)
      })
      .catch((err) => console.warn('[ffcom] push: silêncios não carregaram', err))
    return () => {
      cancelled = true
    }
  }, [signedIn, status.available, addressesKey])

  const isMuted = useCallback(
    (serverAddress: string, channelId?: string) =>
      mutes.some((m) => m.serverAddress === serverAddress && (m.channelId ?? undefined) === channelId),
    [mutes],
  )
  const setMuted = useCallback(async (serverAddress: string, channelId: string | undefined, muted: boolean) => {
    await setPushMute(tokenRef.current, { serverAddress, channelId }, muted)
    setMutes((prev) => {
      const rest = prev.filter((m) => !(m.serverAddress === serverAddress && (m.channelId ?? undefined) === channelId))
      return muted ? [...rest, { serverAddress, channelId }] : rest
    })
  }, [])

  const acceptPermission = useCallback(() => {
    setStatus((prev) => ({ ...prev, asked: true }))
    void requestPushPermission().then(refreshStatus)
  }, [refreshStatus])
  const declinePermission = useCallback(() => {
    setStatus((prev) => ({ ...prev, asked: true }))
    dismissPushPermission()
  }, [])

  return {
    available: status.available,
    askPermission: signedIn && status.available && !status.enabled && !status.asked,
    acceptPermission,
    declinePermission,
    mutes: useMemo(() => ({ isMuted, setMuted }), [isMuted, setMuted]),
  }
}
