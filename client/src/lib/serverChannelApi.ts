// Cliente HTTP/WebSocket para a API de um server-channel: estrutura do
// servidor (categorias/canais) e canal de texto (histórico + WebSocket). Ver
// docs/architecture.md, "Decisão: canal de texto em server-channel" — REST
// para operações CRUD/stateless, WebSocket para tempo real.

import type { Category, ChannelType } from '../types'

export interface RemoteCategory {
  id: string
  name: string
  position: number
  createdAt: string
}

export interface RemoteChannel {
  id: string
  categoryId?: string
  name: string
  type: ChannelType
  position: number
  createdAt: string
  // Ver docs/architecture.md, "Decisão: indicador de não lida".
  lastMessageAt?: string
}

// RemoteAttachment.url é relativo e exige o mesmo Bearer token de qualquer
// outra rota de server-channel — não dá pra usar direto num <img src> ou
// link de download, ver fetchAttachmentBlob abaixo. Ver docs/architecture.md,
// "Decisão: upload de anexo em mensagem".
export interface RemoteAttachment {
  id: string
  filename: string
  contentType: string
  sizeBytes: number
  url: string
}

export interface ChannelMessage {
  id: string
  channelId: string
  // threadId vem preenchido só quando a mensagem é um post dentro de uma
  // thread de canal forum (ver docs/architecture.md, "Canal forum:
  // threads/posts"); nulo para mensagem de canal de texto.
  threadId?: string
  authorMemberId: string
  content: string
  createdAt: string
  editedAt?: string
  // Só preenchido em mensagem de canal de texto (canal forum fora do
  // escopo, mesmo critério de edição/exclusão de mensagem).
  attachments?: RemoteAttachment[]
}

export interface RemoteThread {
  id: string
  channelId: string
  title: string
  authorMemberId: string
  createdAt: string
}

export interface Me {
  memberId: string
  oidcSubject: string
  nickname?: string
  joinedAt: string
  isOwner?: boolean
  // Permissão base efetiva (roles + role default), sem overwrites de canal —
  // ver docs/architecture.md, "Sistema de permissões/roles por servidor e
  // por canal". Usada só para decidir se mostra UI de administração; a
  // permissão de fato é sempre reforçada pelo servidor em cada rota.
  permissions: number
  roleIds?: string[]
}

async function parseJsonOrThrow<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(`${res.status} ${res.statusText}${text ? `: ${text}` : ''}`)
  }
  return res.json() as Promise<T>
}

export async function fetchMe(baseUrl: string, accessToken: string): Promise<Me> {
  const res = await fetch(`${baseUrl}/api/me`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  return parseJsonOrThrow<Me>(res)
}

// PATCH /api/me — define (ou limpa, passando undefined) o apelido exibido
// neste server-channel. Ver docs/architecture.md, endpoint novo em
// server-channel/internal/httpapi/me.go.
export async function updateMyNickname(
  baseUrl: string,
  accessToken: string,
  nickname: string | undefined,
): Promise<Me> {
  const res = await fetch(`${baseUrl}/api/me`, {
    method: 'PATCH',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ nickname: nickname ?? null }),
  })
  return parseJsonOrThrow<Me>(res)
}

export interface JoinResult {
  memberId: string
  founder?: boolean
}

// POST /api/join — precisa ser chamado antes de qualquer outra rota deste
// server-channel (ver docs/architecture.md, "Convites obrigatórios para
// entrar em server-channel"). Sem `code`, só funciona se ninguém ainda for
// membro (bootstrap do self-host) ou se o "sub" já tiver entrado antes.
export async function joinServer(
  baseUrl: string,
  accessToken: string,
  code?: string,
): Promise<JoinResult> {
  const res = await fetch(`${baseUrl}/api/join`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(code ? { code } : {}),
  })
  return parseJsonOrThrow<JoinResult>(res)
}

export interface RemoteInvite {
  id: string
  code: string
  createdByMemberId: string
  maxUses?: number
  uses: number
  expiresAt?: string
  createdAt: string
}

// POST /api/invites — gera um código de convite para este server-channel
// (qualquer membro pode gerar, ver docs/architecture.md).
export async function createServerInvite(
  baseUrl: string,
  accessToken: string,
): Promise<RemoteInvite> {
  const res = await fetch(`${baseUrl}/api/invites`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({}),
  })
  return parseJsonOrThrow<RemoteInvite>(res)
}

export async function fetchCategories(
  baseUrl: string,
  accessToken: string,
): Promise<RemoteCategory[]> {
  const res = await fetch(`${baseUrl}/api/categories`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ categories: RemoteCategory[] }>(res)
  return body.categories
}

export async function fetchChannels(
  baseUrl: string,
  accessToken: string,
): Promise<RemoteChannel[]> {
  const res = await fetch(`${baseUrl}/api/channels`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ channels: RemoteChannel[] }>(res)
  return body.channels
}

export interface RemoteMember {
  id: string
  nickname?: string
  joinedAt: string
  isOwner?: boolean
  roleIds?: string[]
}

// GET /api/members — lista os membros deste server-channel com suas roles
// atribuídas (ver docs/architecture.md, "Sistema de permissões/roles por
// servidor e por canal").
export async function fetchMembers(baseUrl: string, accessToken: string): Promise<RemoteMember[]> {
  const res = await fetch(`${baseUrl}/api/members`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ members: RemoteMember[] }>(res)
  return body.members
}

export interface RemoteRole {
  id: string
  name: string
  color?: string
  permissions: number
  position: number
  isDefault: boolean
  createdAt: string
}

export async function fetchRoles(baseUrl: string, accessToken: string): Promise<RemoteRole[]> {
  const res = await fetch(`${baseUrl}/api/roles`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ roles: RemoteRole[] }>(res)
  return body.roles
}

async function postJsonOrThrow<T>(
  baseUrl: string,
  path: string,
  accessToken: string,
  method: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(`${baseUrl}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${accessToken}`,
      'Content-Type': 'application/json',
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  return parseJsonOrThrow<T>(res)
}

export function createRole(
  baseUrl: string,
  accessToken: string,
  role: { name: string; color?: string; permissions: number; position: number },
): Promise<RemoteRole> {
  return postJsonOrThrow(baseUrl, '/api/roles', accessToken, 'POST', role)
}

export function deleteRole(baseUrl: string, accessToken: string, roleId: string): Promise<void> {
  return postJsonOrThrow(baseUrl, `/api/roles/${roleId}`, accessToken, 'DELETE')
}

export function assignRole(
  baseUrl: string,
  accessToken: string,
  memberId: string,
  roleId: string,
): Promise<void> {
  return postJsonOrThrow(baseUrl, `/api/members/${memberId}/roles/${roleId}`, accessToken, 'POST')
}

export function removeRole(
  baseUrl: string,
  accessToken: string,
  memberId: string,
  roleId: string,
): Promise<void> {
  return postJsonOrThrow(baseUrl, `/api/members/${memberId}/roles/${roleId}`, accessToken, 'DELETE')
}

// POST /api/members/{memberId}/kick — expulsa um membro (requer
// KickMembers). Quem for expulso pode voltar com um convite novo, ver
// docs/architecture.md, "Decisão: kick/ban de membro".
export function kickMember(baseUrl: string, accessToken: string, memberId: string): Promise<void> {
  return postJsonOrThrow(baseUrl, `/api/members/${memberId}/kick`, accessToken, 'POST')
}

// POST /api/members/{memberId}/ban — expulsa um membro e bloqueia
// reentrada até um unban (requer BanMembers).
export function banMember(
  baseUrl: string,
  accessToken: string,
  memberId: string,
  reason?: string,
): Promise<void> {
  return postJsonOrThrow(baseUrl, `/api/members/${memberId}/ban`, accessToken, 'POST', reason ? { reason } : {})
}

export interface RemoteBan {
  oidcSubject: string
  bannedByMemberId?: string
  reason?: string
  createdAt: string
  lastNickname?: string
}

// GET /api/bans — lista banimentos ativos (requer BanMembers), para a UI
// de administração poder revogar.
export async function fetchBans(baseUrl: string, accessToken: string): Promise<RemoteBan[]> {
  const res = await fetch(`${baseUrl}/api/bans`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ bans: RemoteBan[] }>(res)
  return body.bans
}

// DELETE /api/bans/{oidcSubject} — revoga um banimento (requer BanMembers).
export function unbanMember(baseUrl: string, accessToken: string, oidcSubject: string): Promise<void> {
  return postJsonOrThrow(baseUrl, `/api/bans/${encodeURIComponent(oidcSubject)}`, accessToken, 'DELETE')
}

// Criar/renomear/mover/apagar categoria e canal (requer ManageChannels) --
// ver docs/architecture.md, "Decisão: gerenciar categorias e canais".
export function createCategory(baseUrl: string, accessToken: string, name: string): Promise<RemoteCategory> {
  return postJsonOrThrow(baseUrl, '/api/categories', accessToken, 'POST', { name })
}

export function updateCategory(
  baseUrl: string,
  accessToken: string,
  categoryId: string,
  changes: { name?: string; position?: number },
): Promise<RemoteCategory> {
  return postJsonOrThrow(baseUrl, `/api/categories/${categoryId}`, accessToken, 'PATCH', changes)
}

// Os canais da categoria apagada continuam existindo, sem categoria.
export function deleteCategory(baseUrl: string, accessToken: string, categoryId: string): Promise<void> {
  return postJsonOrThrow(baseUrl, `/api/categories/${categoryId}`, accessToken, 'DELETE')
}

export function createChannel(
  baseUrl: string,
  accessToken: string,
  channel: { name: string; type: ChannelType; categoryId?: string },
): Promise<RemoteChannel> {
  return postJsonOrThrow(baseUrl, '/api/channels', accessToken, 'POST', channel)
}

// categoryId: null tira o canal da categoria; ausente mantém a atual.
export function updateChannel(
  baseUrl: string,
  accessToken: string,
  channelId: string,
  changes: { name?: string; categoryId?: string | null; position?: number },
): Promise<RemoteChannel> {
  return postJsonOrThrow(baseUrl, `/api/channels/${channelId}`, accessToken, 'PATCH', changes)
}

// Apaga o canal com todas as mensagens, threads e anexos.
export function deleteChannel(baseUrl: string, accessToken: string, channelId: string): Promise<void> {
  return postJsonOrThrow(baseUrl, `/api/channels/${channelId}`, accessToken, 'DELETE')
}

// Id da categoria sintética "Canais" que agrupa os canais sem categoria (ver
// groupIntoCategories). Não existe no servidor: não dá pra renomear nem
// apagar, e criar canal nela significa criar sem categoryId.
export const UNCATEGORIZED_ID = 'uncategorized'

// Agrupa categorias e canais crus da API em Category[] (formato usado pelo
// client), preservando a ordem por posição já aplicada pelo servidor. Canais
// sem categoria (categoryId nulo) vão para uma categoria sintética no topo,
// só quando existir pelo menos um.
export function groupIntoCategories(
  categories: RemoteCategory[],
  channels: RemoteChannel[],
): Category[] {
  const byCategory = new Map<string, Category>(
    categories.map((c) => [c.id, { id: c.id, name: c.name, channels: [] }]),
  )

  const uncategorized: Category = { id: UNCATEGORIZED_ID, name: 'Canais', channels: [] }
  let hasUncategorized = false

  for (const channel of channels) {
    const target = channel.categoryId ? byCategory.get(channel.categoryId) : undefined
    const entry = {
      id: channel.id,
      name: channel.name,
      type: channel.type,
      lastMessageAt: channel.lastMessageAt,
    }
    if (target) {
      target.channels.push(entry)
    } else {
      uncategorized.channels.push(entry)
      hasUncategorized = true
    }
  }

  const grouped = categories.map((c) => byCategory.get(c.id)!)
  return hasUncategorized ? [uncategorized, ...grouped] : grouped
}

// Histórico devolvido pelo servidor vem mais recente primeiro (keyset por
// created_at); esta função já devolve em ordem cronológica (mais antiga
// primeiro), pronta para renderizar de cima para baixo.
export async function fetchChannelHistory(
  baseUrl: string,
  channelId: string,
  accessToken: string,
  limit = 50,
): Promise<ChannelMessage[]> {
  const res = await fetch(
    `${baseUrl}/api/channels/${channelId}/messages?limit=${limit}`,
    { headers: { Authorization: `Bearer ${accessToken}` } },
  )
  const body = await parseJsonOrThrow<{ messages: ChannelMessage[] }>(res)
  return body.messages.slice().reverse()
}

// GET /api/channels/{id}/threads — lista as threads de um canal forum, mais
// recentes primeiro (mesma ordem devolvida pelo servidor).
export async function fetchForumThreads(
  baseUrl: string,
  channelId: string,
  accessToken: string,
): Promise<RemoteThread[]> {
  const res = await fetch(`${baseUrl}/api/channels/${channelId}/threads`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  const body = await parseJsonOrThrow<{ threads: RemoteThread[] }>(res)
  return body.threads
}

// GET /api/threads/{id}/messages — histórico de posts de uma thread, na
// mesma ordem cronológica já usada por fetchChannelHistory.
export async function fetchThreadMessages(
  baseUrl: string,
  threadId: string,
  accessToken: string,
  limit = 50,
): Promise<ChannelMessage[]> {
  const res = await fetch(
    `${baseUrl}/api/threads/${threadId}/messages?limit=${limit}`,
    { headers: { Authorization: `Bearer ${accessToken}` } },
  )
  const body = await parseJsonOrThrow<{ messages: ChannelMessage[] }>(res)
  return body.messages.slice().reverse()
}

export interface VoiceToken {
  token: string
  roomName: string
  // Endereço público do LiveKit desta instância de server-channel — pode
  // ser diferente de baseUrl (ver docs/architecture.md, "Decisão:
  // integração de voz com LiveKit"), por isso vem sempre do servidor em vez
  // de ser derivado no client.
  url: string
}

// POST /api/channels/{id}/voice/token — pede um access token de LiveKit
// novo a cada tentativa de entrar num canal de voz (ver
// hooks/useVoiceChannel.ts). Não há cache: o token tem TTL curto e o custo
// de pedir de novo é uma requisição HTTP local ao server-channel.
export async function fetchVoiceToken(
  baseUrl: string,
  channelId: string,
  accessToken: string,
): Promise<VoiceToken> {
  const res = await fetch(`${baseUrl}/api/channels/${channelId}/voice/token`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  return parseJsonOrThrow<VoiceToken>(res)
}

function toWebSocketUrl(baseUrl: string, channelId: string): string {
  const url = new URL(`${baseUrl}/api/channels/${channelId}/ws`)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url.toString()
}

// A API WebSocket do navegador não permite header Authorization no
// handshake; o token vai no subprotocolo "access_token, <token>" (ver
// docs/architecture.md e internal/httpapi/channel_ws.go em server-channel).
export function openChannelSocket(
  baseUrl: string,
  channelId: string,
  accessToken: string,
): WebSocket {
  return new WebSocket(toWebSocketUrl(baseUrl, channelId), ['access_token', accessToken])
}

export function sendCreateMessage(socket: WebSocket, content: string): void {
  socket.send(JSON.stringify({ type: 'message.create', content }))
}

// POST /api/channels/{id}/messages — só usada quando a mensagem tem um
// anexo (multipart/form-data); o handshake de WebSocket não tem como
// carregar um arquivo, então mensagem só-texto continua indo por
// sendCreateMessage. O broadcast ("message.created") chega pela mesma
// conexão de WebSocket já aberta (ver docs/architecture.md, "Decisão:
// upload de anexo em mensagem") — o retorno desta função só serve pra saber
// se o upload falhou, não precisa adicionar a mensagem ao estado local.
export async function sendMessageWithAttachment(
  baseUrl: string,
  channelId: string,
  accessToken: string,
  content: string,
  file: File,
): Promise<ChannelMessage> {
  const form = new FormData()
  if (content) form.set('content', content)
  form.set('file', file)
  const res = await fetch(`${baseUrl}/api/channels/${channelId}/messages`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
    body: form,
  })
  return parseJsonOrThrow<ChannelMessage>(res)
}

// GET /api/attachments/{id} — precisa do mesmo Bearer token de qualquer
// outra rota, então não dá pra apontar um <img src> ou link direto pra lá;
// o client busca como Blob e gera uma object URL local (ver
// components/MessageAttachment.tsx).
export async function fetchAttachmentBlob(
  baseUrl: string,
  attachment: RemoteAttachment,
  accessToken: string,
): Promise<Blob> {
  const res = await fetch(`${baseUrl}${attachment.url}`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`)
  }
  return res.blob()
}

export function sendUpdateMessage(socket: WebSocket, id: string, content: string): void {
  socket.send(JSON.stringify({ type: 'message.update', id, content }))
}

export function sendDeleteMessage(socket: WebSocket, id: string): void {
  socket.send(JSON.stringify({ type: 'message.delete', id }))
}

// Frames de canal forum, mesma conexão de WebSocket do canal (ver
// docs/architecture.md, "Canal forum: threads/posts").
export function sendCreateThread(socket: WebSocket, title: string, content: string): void {
  socket.send(JSON.stringify({ type: 'thread.create', title, content }))
}

export function sendCreatePost(socket: WebSocket, threadId: string, content: string): void {
  socket.send(JSON.stringify({ type: 'post.create', threadId, content }))
}

interface MessageCreatedFrame {
  type: 'message.created'
  message: ChannelMessage
}

interface MessageUpdatedFrame {
  type: 'message.updated'
  message: ChannelMessage
}

interface MessageDeletedFrame {
  type: 'message.deleted'
  id: string
  channelId: string
}

interface ThreadCreatedFrame {
  type: 'thread.created'
  thread: RemoteThread
  message: ChannelMessage
}

interface PostCreatedFrame {
  type: 'post.created'
  message: ChannelMessage
}

interface ErrorFrame {
  type: 'error'
  error: string
}

export type ChannelSocketFrame =
  | MessageCreatedFrame
  | MessageUpdatedFrame
  | MessageDeletedFrame
  | ThreadCreatedFrame
  | PostCreatedFrame
  | ErrorFrame

const CHANNEL_SOCKET_FRAME_TYPES = new Set([
  'message.created',
  'message.updated',
  'message.deleted',
  'thread.created',
  'post.created',
  'error',
])

export function decodeChannelSocketFrame(raw: string): ChannelSocketFrame | null {
  try {
    const parsed = JSON.parse(raw) as { type?: string }
    if (parsed.type && CHANNEL_SOCKET_FRAME_TYPES.has(parsed.type)) {
      return parsed as ChannelSocketFrame
    }
    return null
  } catch {
    return null
  }
}
