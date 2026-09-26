import type { ChosenStatus, PresenceStatus } from '../types'

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
  // Status escolhido. Ausente em server-central anterior ao campo.
  status?: ChosenStatus
  createdAt: string
  // Chave pública de E2E publicada pela conta (base64), null se nenhuma.
  // Ausente só em server-central anterior ao campo -- ver
  // hooks/useE2EKeys.ts, que depende dessa diferença.
  e2ePublicKey?: string | null
  // Se a conta já tem backup da chave com frase de recuperação. Ausente em
  // server-central anterior ao campo.
  hasE2EKeyBackup?: boolean
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

// Grava a ordem do rail (arrastar os ícones). ids precisa ser a lista
// completa da conta; se mudou em outra aba ou dispositivo, 409.
export async function reorderKnownServers(accessToken: string, ids: string[]): Promise<void> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/servers/order`, {
    method: 'PUT',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ ids }),
  })
  if (!res.ok) {
    throw new Error(`server-central: ${res.status} ${res.statusText}`)
  }
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
  // Ausente em server-central anterior ao status: aí só vale online.
  status?: PresenceStatus
  online: boolean
}

// Status como os outros veem, a partir de um frame ou snapshot de presença,
// com o online antigo como reserva.
export function presenceStatusOf(p: { status?: PresenceStatus; online: boolean }): PresenceStatus {
  return p.status ?? (p.online ? 'online' : 'offline')
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

export interface E2EKeyBackup {
  publicKey: string
  backup: string
}

// Erro de PUT /api/me/e2e-key-backup quando a conta já tem backup (outro
// dispositivo criou antes) -- hooks/useE2EKeys.ts passa a pedir a frase.
export class E2EKeyBackupConflictError extends Error {}

// fetchMyE2EKeyBackup busca a chave pública da conta e o backup cifrado da
// chave privada (crypto/keyBackup.ts), para desbloquear com a frase.
export async function fetchMyE2EKeyBackup(accessToken: string): Promise<E2EKeyBackup> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/me/e2e-key-backup`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  return parseJsonOrThrow<E2EKeyBackup>(res)
}

// setMyE2EKeyBackup grava a chave pública da conta junto com o backup
// cifrado. replace troca a chave de uma conta que já tem backup ("esqueci a
// frase"); sem ele, lança E2EKeyBackupConflictError se já houver backup.
export async function setMyE2EKeyBackup(accessToken: string, backup: E2EKeyBackup, replace: boolean): Promise<void> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/me/e2e-key-backup`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...backup, replace }),
  })
  if (res.status === 409) throw new E2EKeyBackupConflictError('a conta já tem frase de recuperação')
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

// Pedidos de amizade feitos pela lista de membros (ver docs/architecture.md,
// "Decisão: pedido de amizade pela lista de membros"). accountId/nome/avatar
// são sempre do outro lado do pedido.
export interface FriendRequest {
  id: string
  accountId: string
  displayName?: string
  avatarUrl?: string
  createdAt: string
}

export interface FriendRequests {
  incoming: FriendRequest[]
  outgoing: FriendRequest[]
}

// Nome de quem está do outro lado de um pedido. Sem perfil em server-central
// (a pessoa nunca definiu avatar), sobra o começo do id, mesmo recurso das
// listas de membros.
export function friendRequestName(request: { accountId: string; displayName?: string }): string {
  return request.displayName ?? request.accountId.slice(0, 8)
}

async function throwWithBody(res: Response): Promise<void> {
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text.trim() || `server-central: ${res.status} ${res.statusText}`)
  }
}

export async function fetchFriendRequests(accessToken: string): Promise<FriendRequests> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/friends/requests`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  return parseJsonOrThrow<FriendRequests>(res)
}

// status 'accepted' quando o outro lado já tinha mandado um pedido: vira
// amizade na hora.
export async function sendFriendRequest(
  accessToken: string,
  accountId: string,
): Promise<{ status: 'pending' | 'accepted'; request: FriendRequest }> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/friends/requests`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ accountId }),
  })
  await throwWithBody(res)
  return res.json()
}

export async function acceptFriendRequest(accessToken: string, id: string): Promise<void> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/friends/requests/${encodeURIComponent(id)}/accept`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  await throwWithBody(res)
}

// Recusa (pedido recebido) ou cancela (pedido enviado).
export async function deleteFriendRequest(accessToken: string, id: string): Promise<void> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/friends/requests/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  await throwWithBody(res)
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

// Avisa server-central que a pessoa ficou ociosa (ou voltou) nesta conexão;
// ver hooks/useIdle.ts.
export function sendPresenceIdleFrame(socket: WebSocket, idle: boolean): void {
  if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'presence.idle', idle }))
}

type PresenceSocketFrame =
  | { type: 'presence.update'; accountId: string; status?: PresenceStatus; online: boolean }
  | { type: 'dm.created'; message: RemoteDirectMessage }
  | { type: 'friend.request'; request: FriendRequest }
  | { type: 'friend.request.removed'; id: string }
  | { type: 'friend.accepted'; requestId?: string; accountId: string; displayName?: string }
  | { type: 'error'; error: string }

export function decodePresenceSocketFrame(raw: string): PresenceSocketFrame | null {
  try {
    const parsed = JSON.parse(raw) as { type?: string }
    if (
      parsed.type === 'presence.update' ||
      parsed.type === 'dm.created' ||
      parsed.type === 'friend.request' ||
      parsed.type === 'friend.request.removed' ||
      parsed.type === 'friend.accepted' ||
      parsed.type === 'error'
    ) {
      return parsed as PresenceSocketFrame
    }
    return null
  } catch {
    return null
  }
}

// PUT /api/me/status — grava o status escolhido (ver docs/architecture.md,
// "Decisão: status de presença e avatar nas listas de membros").
export async function setMyStatus(accessToken: string, status: ChosenStatus): Promise<void> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/me/status`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `server-central: ${res.status} ${res.statusText}`)
  }
}

// Conta de server-central por trás de um membro de server-channel, achada
// pelo oidcSubject (a chave que os dois servidores têm em comum).
export interface AccountSummary {
  oidcSubject: string
  accountId: string
  displayName?: string
  avatarUrl?: string
}

// POST /api/accounts/lookup — subjects sem conta em server-central (nunca
// logaram lá) simplesmente não voltam.
export async function lookupAccounts(accessToken: string, subjects: string[]): Promise<AccountSummary[]> {
  const res = await fetch(`${SERVER_CENTRAL_URL}/api/accounts/lookup`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ subjects }),
  })
  const body = await parseJsonOrThrow<{ accounts: AccountSummary[] }>(res)
  return body.accounts
}
