export type ChannelType = 'text' | 'voice' | 'forum'

export interface KnownServer {
  id: string
  name: string
  initials: string
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
