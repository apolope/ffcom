export type ChannelType = 'text' | 'voice' | 'forum'

export interface KnownServer {
  id: string
  name: string
  initials: string
  // Endereço base do server-channel desta comunidade (REST + WebSocket).
  // Preenchimento manual por enquanto — a tela de "adicionar servidor via
  // IP/DNS" (TODO.md) ainda não existe.
  baseUrl: string
}

export interface Channel {
  id: string
  name: string
  type: ChannelType
}

export interface Category {
  id: string
  name: string
  channels: Channel[]
}

export interface Member {
  id: string
  nickname: string
  online: boolean
}

export interface ServerDetail {
  categories: Category[]
  members: Member[]
}
