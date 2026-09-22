// Cliente HTTP para a API de server-central: diretório de server-channel
// conhecidos pela conta autenticada. Ver docs/architecture.md, "Decisão:
// descoberta de server-channel" — sem descoberta automática, só convite ou
// endereço adicionado manualmente.

// Instância oficial única do server-central (ver docs/architecture.md,
// contexto: "server-central (instância única oficial)"). Domínio de
// produção ainda não provisionado (ver TODO.md); em dev local aponta para o
// docker-compose de referência (SERVER_CENTRAL_PORT=8081).
export const SERVER_CENTRAL_URL: string =
  import.meta.env.VITE_SERVER_CENTRAL_URL ?? 'http://localhost:8081'

export interface RemoteKnownServer {
  id: string
  address: string
  name: string
  iconUrl?: string
  addedAt: string
}

async function parseJsonOrThrow<T>(res: Response): Promise<T> {
  if (!res.ok) {
    throw new Error(`server-central: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

// Conta autenticada + perfil (GET /api/me em server-central) -- não confundir
// com hooks/useMe.ts, que é o "me" de um server-channel específico (apelido,
// permissões). displayName/avatarUrl aqui vêm de profiles, preenchido só
// depois do primeiro upload de avatar (ver internal/httpapi/avatar.go em
// server-central, docs/architecture.md "Decisão: upload de avatar de conta").
export interface MyProfile {
  accountId: string
  oidcSubject: string
  createdAt: string
  displayName?: string
  avatarUrl?: string
}

export async function fetchMyProfile(accessToken: string): Promise<MyProfile> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/me`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  return parseJsonOrThrow<MyProfile>(res)
}

export async function uploadMyAvatar(accessToken: string, file: File): Promise<MyProfile> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/me/avatar`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
    body: form,
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `server-central: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<MyProfile>
}

export async function deleteMyAvatar(accessToken: string): Promise<MyProfile> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/me/avatar`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `server-central: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<MyProfile>
}

// GET /api/avatars/{accountId} — precisa do mesmo Bearer token de qualquer
// outra rota deste servidor, por isso um <img src="..."> direto não
// funciona -- o client busca como Blob e gera uma object URL local (ver
// components/UserAvatar.tsx). avatarUrl já vem como o path relativo
// ("/api/avatars/{id}") devolvido pelo servidor em MyProfile/Friend.
export async function fetchAvatarBlob(avatarUrl: string, accessToken: string): Promise<Blob> {
  const res = await fetch(`${SERVER_CENTRAL_URL}${avatarUrl}`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) {
    throw new Error(`server-central: ${res.status} ${res.statusText}`)
  }
  return res.blob()
}

export async function fetchKnownServers(accessToken: string): Promise<RemoteKnownServer[]> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/servers`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ servers: RemoteKnownServer[] }>(res)
  return body.servers
}

export async function addKnownServer(
  accessToken: string,
  address: string,
  name: string,
): Promise<RemoteKnownServer> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/servers`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ address, name }),
  })
  return parseJsonOrThrow<RemoteKnownServer>(res)
}

export async function removeKnownServer(accessToken: string, id: string): Promise<void> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/servers/${id}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) {
    throw new Error(`server-central: ${res.status} ${res.statusText}`)
  }
}

// Amigos + presença (ver docs/architecture.md, "Decisão: adicionar amigos
// via convite" e "Decisão: gateway de presença em server-central").

export interface RemoteFriend {
  accountId: string
  displayName?: string
  avatarUrl?: string
  // Chave pública de E2E (NaCl box, base64) do dispositivo ativo do amigo,
  // se já publicada -- ver docs/architecture.md, "Decisão: criptografia
  // ponta-a-ponta em DMs". Ausente = amigo ainda não usou DMs em nenhum
  // dispositivo, não dá pra enviar mensagem cifrada para ele ainda.
  e2ePublicKey?: string
  // Ver docs/architecture.md, "Decisão: indicador de não lida".
  lastMessageAt?: string
}

export interface FriendPresence {
  accountId: string
  online: boolean
}

export interface FriendInvite {
  code: string
  createdAt: string
}

export async function fetchFriends(accessToken: string): Promise<RemoteFriend[]> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/friends`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ friends: RemoteFriend[] }>(res)
  return body.friends
}

// setMyE2EPublicKey publica a chave pública de E2E deste dispositivo (ver
// hooks/useE2EKeys.ts) -- sobrescreve qualquer chave publicada antes por
// outro dispositivo desta mesma conta.
export async function setMyE2EPublicKey(accessToken: string, publicKeyB64: string): Promise<void> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/me/e2e-public-key`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ publicKey: publicKeyB64 }),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `server-central: ${res.status} ${res.statusText}`)
  }
}

export async function fetchPresenceSnapshot(accessToken: string): Promise<FriendPresence[]> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/presence`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ friends: FriendPresence[] }>(res)
  return body.friends
}

export async function createFriendInvite(accessToken: string): Promise<FriendInvite> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/friends/invites`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  return parseJsonOrThrow<FriendInvite>(res)
}

export async function redeemFriendInvite(accessToken: string, code: string): Promise<void> {
  const res = await fetch(
    `${SERVER_CENTRAL_URL}/api/friends/invites/${encodeURIComponent(code)}/redeem`,
    { method: 'POST', headers: { Authorization: `Bearer ${accessToken}` } },
  )
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `server-central: ${res.status} ${res.statusText}`)
  }
}

function toPresenceWebSocketUrl(): string {
  const url = new URL(`${SERVER_CENTRAL_URL}/api/presence/ws`)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

// Mesmo mecanismo de auth via subprotocolo usado no WebSocket de canal de
// texto (ver serverChannelApi.ts e docs/architecture.md) — a API WebSocket
// do navegador não permite header Authorization no handshake.
export function openPresenceSocket(accessToken: string): WebSocket {
  return new WebSocket(toPresenceWebSocketUrl(), ['access_token', accessToken])
}

// DMs (ver docs/architecture.md, "Decisão: DMs entregues no mesmo WebSocket
// de presença"): mesma conexão de openPresenceSocket acima, sem socket
// dedicado — o client anexa um listener extra a esse mesmo WebSocket (ver
// hooks/useDirectMessages.ts).

// ciphertext/nonce (base64) são opacos ao server-central -- criptografia
// ponta-a-ponta, ver docs/architecture.md, "Decisão: criptografia
// ponta-a-ponta em DMs". Decifrados no client via crypto/e2e.ts antes de
// virar texto exibível (ver hooks/useDirectMessages.ts).
export interface RemoteDirectMessage {
  id: string
  senderId: string
  recipientId: string
  ciphertext: string
  nonce: string
  createdAt: string
  editedAt?: string
}

export async function fetchDirectMessages(
  accessToken: string,
  otherAccountId: string,
  limit = 50,
): Promise<RemoteDirectMessage[]> {
  const res = await fetch(
    `${SERVER_CENTRAL_URL}/api/dms/${encodeURIComponent(otherAccountId)}/messages?limit=${limit}`,
    { headers: { Authorization: `Bearer ${accessToken}` } },
  )
  const body = await parseJsonOrThrow<{ messages: RemoteDirectMessage[] }>(res)
  return body.messages.slice().reverse()
}

export function sendDirectMessageFrame(socket: WebSocket, recipientId: string, ciphertext: string, nonce: string): void {
  socket.send(JSON.stringify({ type: 'dm.create', recipientId, ciphertext, nonce }))
}

type PresenceSocketFrame =
  | { type: 'presence.update'; accountId: string; online: boolean }
  | { type: 'dm.created'; message: RemoteDirectMessage }
  | { type: 'error'; error: string }

export function decodePresenceSocketFrame(raw: string): PresenceSocketFrame | null {
  try {
    const parsed = JSON.parse(raw) as { type?: string }
    if (
      parsed.type === 'presence.update' ||
      parsed.type === 'dm.created' ||
      parsed.type === 'error'
    ) {
      return parsed as PresenceSocketFrame
    }
    return null
  } catch {
    return null
  }
}
