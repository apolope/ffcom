export type ChannelType = 'text' | 'voice' | 'forum'

export interface KnownServer {
  id: string
  name: string
  initials: string
  // Endereço base do server-channel desta comunidade (REST + WebSocket),
  // vindo do diretório de server-channel conhecidos em server-central (ver
  // hooks/useKnownServers.ts).
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

// Amigo (via server-central, ver hooks/useFriends.ts). displayName cai para
// o accountId quando a conta ainda não preencheu o perfil (ver
// ProfileStore.GetManyByAccountIDs em server-central).
export interface Friend {
  accountId: string
  displayName: string
  avatarUrl?: string
  online: boolean
}
