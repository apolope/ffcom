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
  // Timestamp da mensagem mais recente do canal (ISO 8601), se houver
  // alguma -- usado só para o indicador de não lida (ver
  // lib/unread.ts/hooks/useUnread.ts), comparado contra um cursor "última
  // leitura" guardado local ao dispositivo. Não é uma garantia de entrega
  // nem reflete atividade de voz.
  lastMessageAt?: string
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

// Banimento de um server-channel (ver hooks/useServerMembers.ts,
// docs/architecture.md "Decisão: kick/ban de membro"). displayName cai para
// o oidcSubject cru quando a pessoa nunca chegou a definir um apelido (ou
// nunca chegou a ser membro — banimento preventivo).
export interface Ban {
  oidcSubject: string
  displayName: string
  reason?: string
  createdAt: string
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
  // Timestamp da DM mais recente trocada com este amigo (ISO 8601), em
  // qualquer sentido -- mesmo uso do lastMessageAt de Channel, ver
  // hooks/useUnread.ts.
  lastMessageAt?: string
}
