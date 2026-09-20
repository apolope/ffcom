// Cliente HTTP/WebSocket para a API de canal de texto de um server-channel.
// Ver docs/architecture.md, "Decisão: canal de texto em server-channel" —
// REST para histórico paginado, WebSocket para tempo real.

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
