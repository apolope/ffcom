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
}

export interface ChannelMessage {
  id: string
  channelId: string
  authorMemberId: string
  content: string
  createdAt: string
  editedAt?: string
}

export interface Me {
  memberId: string
  oidcSubject: string
  nickname?: string
  joinedAt: string
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

const UNCATEGORIZED_ID = 'uncategorized'

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
    const entry = { id: channel.id, name: channel.name, type: channel.type }
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

interface MessageCreatedFrame {
  type: 'message.created'
  message: ChannelMessage
}

interface ErrorFrame {
  type: 'error'
  error: string
}

export type ChannelSocketFrame = MessageCreatedFrame | ErrorFrame

export function decodeChannelSocketFrame(raw: string): ChannelSocketFrame | null {
  try {
    const parsed = JSON.parse(raw) as { type?: string }
    if (parsed.type === 'message.created' || parsed.type === 'error') {
      return parsed as ChannelSocketFrame
    }
    return null
  } catch {
    return null
  }
}
