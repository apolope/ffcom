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

// Membro de um server-channel (ver hooks/useServerMembers.ts). Não há
// presença/online por membro de server-channel ainda (diferente da lista de
// amigos, que vem de server-central com presença via WebSocket) — só voz
// tem um conceito de "quem está no canal agora", ver VoiceChannelView.
export interface Member {
  id: string
  nickname: string
  isOwner: boolean
  roleIds: string[]
}

export interface Role {
  id: string
  name: string
  color?: string
  permissions: number
  position: number
  isDefault: boolean
}

// Amigo (via server-central, ver hooks/useFriends.ts). displayName cai para
// o accountId quando a conta ainda não preencheu o perfil (ver
// ProfileStore.GetManyByAccountIDs em server-central).
export interface Friend {
  accountId: string
  displayName: string
  avatarUrl?: string
  online: boolean
  // Chave pública de E2E do amigo (base64), se já publicada -- ver
  // crypto/e2e.ts e docs/architecture.md, "Decisão: criptografia
  // ponta-a-ponta em DMs". Ausente = ainda não dá pra enviar DM cifrada.
  e2ePublicKey?: string
}
