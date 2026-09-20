import { useCallback, useEffect, useState } from 'react'
import {
  createFriendInvite,
  decodePresenceFrame,
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
}

function toFriend(remote: RemoteFriend, online: boolean): Friend {
  return {
    accountId: remote.accountId,
    displayName: remote.displayName ?? remote.accountId,
    avatarUrl: remote.avatarUrl,
    online,
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

  const load = useCallback(() => {
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

    const socket = openPresenceSocket(accessToken)
    socket.onmessage = (event) => {
      const frame = decodePresenceFrame(String(event.data))
      if (!frame) return
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

    return () => socket.close()
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

  return { friends, status, error, createInvite, redeemInvite }
}
