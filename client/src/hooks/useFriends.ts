import { useCallback, useEffect, useRef, useState } from 'react'
import {
  acceptFriendRequest,
  createFriendInvite,
  decodePresenceSocketFrame,
  deleteFriendRequest,
  fetchFriendRequests,
  fetchFriends,
  fetchPresenceSnapshot,
  openPresenceSocket,
  presenceStatusOf,
  redeemFriendInvite,
  sendFriendRequest,
  type FriendRequest,
  type RemoteFriend,
} from '../lib/serverCentralApi'
import type { Friend, PresenceStatus } from '../types'

export type FriendsStatus = 'loading' | 'ready' | 'error'

// Avisos que chegam pelo WebSocket e merecem uma mensagem na tela (App.tsx
// decide o texto): pedido recebido, ou pedido aceito pelo outro lado.
export type FriendEvent =
  | { kind: 'request-received'; request: FriendRequest }
  | { kind: 'accepted'; accountId: string; displayName?: string }

interface UseFriendsResult {
  friends: Friend[]
  status: FriendsStatus
  error: string | undefined
  createInvite: () => Promise<string>
  redeemInvite: (code: string) => Promise<void>
  // Pedidos pendentes (ver docs/architecture.md, "Decisão: pedido de
  // amizade pela lista de membros").
  incomingRequests: FriendRequest[]
  outgoingRequests: FriendRequest[]
  sendRequest: (accountId: string) => Promise<'pending' | 'accepted'>
  acceptRequest: (id: string) => Promise<void>
  // Recusa um pedido recebido ou cancela um enviado.
  removeRequest: (id: string) => Promise<void>
  // Conexão WebSocket de presença, exposta para hooks/useDirectMessages.ts
  // anexar um listener extra nela (ver docs/architecture.md, "Decisão: DMs
  // entregues no mesmo WebSocket de presença") em vez de abrir uma segunda
  // conexão.
  socket: WebSocket | null
}

function toFriend(remote: RemoteFriend, status: PresenceStatus): Friend {
  return {
    accountId: remote.accountId,
    displayName: remote.displayName ?? remote.accountId,
    avatarUrl: remote.avatarUrl,
    online: status !== 'offline',
    status,
    e2ePublicKey: remote.e2ePublicKey,
    lastMessageAt: remote.lastMessageAt,
  }
}

// Carrega a lista de amigos (server-central) e mantém o status de presença
// (online, ocupado, ausente ou offline) atualizado: um snapshot inicial via GET /api/presence, depois eventos ao
// vivo via GET /api/presence/ws (ver docs/architecture.md, "Decisão: gateway
// de presença em server-central").
export function useFriends(accessToken: string, onEvent?: (event: FriendEvent) => void): UseFriendsResult {
  const [remoteFriends, setRemoteFriends] = useState<RemoteFriend[]>([])
  const [incomingRequests, setIncomingRequests] = useState<FriendRequest[]>([])
  const [outgoingRequests, setOutgoingRequests] = useState<FriendRequest[]>([])
  const [statusById, setStatusById] = useState<Map<string, PresenceStatus>>(new Map())
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
    return Promise.all([
      fetchFriends(accessToken),
      fetchPresenceSnapshot(accessToken),
      // server-central anterior aos pedidos não tem a rota: sem pedidos.
      fetchFriendRequests(accessToken).catch(() => ({ incoming: [], outgoing: [] })),
    ])
      .then(([friends, presence, requests]) => {
        setRemoteFriends(friends)
        setIncomingRequests(requests.incoming)
        setOutgoingRequests(requests.outgoing)
        setStatusById(new Map(presence.map((p) => [p.accountId, presenceStatusOf(p)])))
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

  // Refs para o listener do WebSocket, que só é recriado quando o token muda.
  const loadRef = useRef(load)
  const onEventRef = useRef(onEvent)
  const outgoingRef = useRef(outgoingRequests)
  useEffect(() => {
    loadRef.current = load
    onEventRef.current = onEvent
    outgoingRef.current = outgoingRequests
  }, [load, onEvent, outgoingRequests])

  useEffect(() => {
    if (!accessToken) return

    const ws = openPresenceSocket(accessToken)
    setSocket(ws)
    ws.onmessage = (event) => {
      const frame = decodePresenceSocketFrame(String(event.data))
      if (!frame) return
      if (frame.type === 'presence.update') {
        setStatusById((prev) => new Map(prev).set(frame.accountId, presenceStatusOf(frame)))
      } else if (frame.type === 'dm.created') {
        // Atualiza lastMessageAt do amigo em memória em vez de esperar o
        // próximo load() — o mesmo evento já chega aqui independente de qual
        // conversa está aberta (ver docs/architecture.md, "Decisão: DMs
        // entregues no mesmo WebSocket de presença"), então dá pra alimentar
        // o indicador de não lida (ver hooks/useUnread.ts) sem polling.
        const { senderId, recipientId, createdAt } = frame.message
        setRemoteFriends((prev) =>
          prev.map((f) =>
            f.accountId === senderId || f.accountId === recipientId
              ? { ...f, lastMessageAt: createdAt }
              : f,
          ),
        )
      } else if (frame.type === 'friend.request') {
        setIncomingRequests((prev) => [frame.request, ...prev.filter((r) => r.id !== frame.request.id)])
        onEventRef.current?.({ kind: 'request-received', request: frame.request })
      } else if (frame.type === 'friend.request.removed') {
        setIncomingRequests((prev) => prev.filter((r) => r.id !== frame.id))
        setOutgoingRequests((prev) => prev.filter((r) => r.id !== frame.id))
      } else if (frame.type === 'friend.accepted') {
        // O mesmo frame chega a quem aceitou (outras abas inclusive); só
        // avisa quem mandou o pedido, que ainda o tinha como enviado.
        const wasOutgoing = outgoingRef.current.some((r) => r.accountId === frame.accountId)
        setOutgoingRequests((prev) => prev.filter((r) => r.accountId !== frame.accountId))
        setIncomingRequests((prev) => prev.filter((r) => r.accountId !== frame.accountId))
        if (wasOutgoing) {
          onEventRef.current?.({ kind: 'accepted', accountId: frame.accountId, displayName: frame.displayName })
        }
        // Amigo novo: lista, chave de E2E e presença vêm do servidor.
        void loadRef.current()
      }
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

  const sendRequest = useCallback(
    async (accountId: string) => {
      const result = await sendFriendRequest(accessToken, accountId)
      if (result.status === 'accepted') {
        setIncomingRequests((prev) => prev.filter((r) => r.accountId !== accountId))
        await load()
      } else {
        setOutgoingRequests((prev) => [result.request, ...prev.filter((r) => r.id !== result.request.id)])
      }
      return result.status
    },
    [accessToken, load],
  )

  const acceptRequest = useCallback(
    async (id: string) => {
      await acceptFriendRequest(accessToken, id)
      setIncomingRequests((prev) => prev.filter((r) => r.id !== id))
      await load()
    },
    [accessToken, load],
  )

  const removeRequest = useCallback(
    async (id: string) => {
      await deleteFriendRequest(accessToken, id)
      setIncomingRequests((prev) => prev.filter((r) => r.id !== id))
      setOutgoingRequests((prev) => prev.filter((r) => r.id !== id))
    },
    [accessToken],
  )

  const friends = remoteFriends.map((f) => toFriend(f, statusById.get(f.accountId) ?? 'offline'))

  return {
    friends,
    status,
    error,
    createInvite,
    redeemInvite,
    incomingRequests,
    outgoingRequests,
    sendRequest,
    acceptRequest,
    removeRequest,
    socket,
  }
}
