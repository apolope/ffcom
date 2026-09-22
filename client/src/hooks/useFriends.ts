import { useCallback, useEffect, useState } from 'react'
import {
  createFriendInvite,
  decodePresenceSocketFrame,
  fetchFriends,
  fetchPresenceSnapshot,
  openPresenceSocket,
  redeemFriendInvite,
  type RemoteFriend,
} from '../lib/serverCentralApi'
import type { Friend } from '../types'

export type FriendsStatus = 'loading' | 'ready' | 'error'

interface UseFriendsResult {
  friends: Friend[]
  status: FriendsStatus
  error: string | undefined
  createInvite: () => Promise<string>
  redeemInvite: (code: string) => Promise<void>
  // Conexão WebSocket de presença, exposta para hooks/useDirectMessages.ts
  // anexar um listener extra nela (ver docs/architecture.md, "Decisão: DMs
  // entregues no mesmo WebSocket de presença") em vez de abrir uma segunda
  // conexão.
  socket: WebSocket | null
}

function toFriend(remote: RemoteFriend, online: boolean): Friend {
  return {
    accountId: remote.accountId,
    displayName: remote.displayName ?? remote.accountId,
    avatarUrl: remote.avatarUrl,
    online,
    e2ePublicKey: remote.e2ePublicKey,
  }
}

// Carrega a lista de amigos (server-central) e mantém o status online/offline
// atualizado: um snapshot inicial via GET /api/presence, depois eventos ao
// vivo via GET /api/presence/ws (ver docs/architecture.md, "Decisão: gateway
// de presença em server-central").
export function useFriends(accessToken: string): UseFriendsResult {
  const [remoteFriends, setRemoteFriends] = useState<RemoteFriend[]>([])
  const [onlineIds, setOnlineIds] = useState<Set<string>>(new Set())
  const [status, setStatus] = useState<FriendsStatus>('loading')
  const [error, setError] = useState<string>()
  const [socket, setSocket] = useState<WebSocket | null>(null)

  const load = useCallback(() => {
    // accessToken só existe depois que o login OIDC termina (ver
    // AuthProvider) — sem essa guarda, o primeiro render (ainda em
    // 'loading' no App) já dispara fetch com token vazio, gerando 401
    // visível no console antes do token real chegar.
    if (!accessToken) return Promise.resolve()
    setStatus('loading')
    setError(undefined)
    return Promise.all([fetchFriends(accessToken), fetchPresenceSnapshot(accessToken)])
      .then(([friends, presence]) => {
        setRemoteFriends(friends)
        setOnlineIds(new Set(presence.filter((p) => p.online).map((p) => p.accountId)))
        setStatus('ready')
      })
      .catch((err) => {
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar amigos')
      })
  }, [accessToken])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!accessToken) return

    const ws = openPresenceSocket(accessToken)
    setSocket(ws)
    ws.onmessage = (event) => {
      const frame = decodePresenceSocketFrame(String(event.data))
      if (!frame || frame.type !== 'presence.update') return
      setOnlineIds((prev) => {
        const next = new Set(prev)
        if (frame.online) {
          next.add(frame.accountId)
        } else {
          next.delete(frame.accountId)
        }
        return next
      })
    }

    return () => {
      ws.close()
      setSocket(null)
    }
  }, [accessToken])

  const createInvite = useCallback(async () => {
    const invite = await createFriendInvite(accessToken)
    return invite.code
  }, [accessToken])

  const redeemInvite = useCallback(
    async (code: string) => {
      await redeemFriendInvite(accessToken, code)
      await load()
    },
    [accessToken, load],
  )

  const friends = remoteFriends.map((f) => toFriend(f, onlineIds.has(f.accountId)))

  return { friends, status, error, createInvite, redeemInvite, socket }
}
