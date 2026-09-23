import { Fragment } from 'react'
import type { Category, ChannelType, KnownServer, Member } from '../types'
import { UNCATEGORIZED_ID, type VoiceParticipant } from '../lib/serverChannelApi'
import './ChannelSidebar.css'

const CHANNEL_ICON: Record<ChannelType, string> = {
  text: '#',
  voice: '🔊',
  forum: '💬',
}

interface ChannelSidebarProps {
  server: KnownServer
  categories: Category[]
  selectedChannelId: string | undefined
  // Ver hooks/useUnread.ts e docs/architecture.md, "Decisão: indicador de
  // não lida".
  unreadChannelIds: Set<string>
  // Quem está em cada sala de voz, por id do canal (hooks/useVoiceParticipants.ts).
  // members resolve o apelido atual; selfMemberId marca "você".
  voiceParticipants: Record<string, VoiceParticipant[]>
  members: Member[]
  selfMemberId: string | undefined
  onSelectChannel: (channelId: string) => void
  onInvite: () => void
  // Só com CreateInvites/ManageInvites ou dono; sem o bit o POST daria 403.
  canCreateInvites: boolean
  canManageMembers: boolean
  onManageRoles: () => void
  onEditNickname: () => void
  // Administração da estrutura, só com ManageChannels ou dono (ver
  // components/StructureDialogs.tsx). categoryId ausente = sem categoria.
  canManageChannels: boolean
  // Overwrite de canal por role, só com ManageRoles ou dono (ver
  // components/ChannelPermissionsDialog.tsx).
  canManageRoles: boolean
  onEditChannelPermissions: (channelId: string) => void
  onCreateCategory: () => void
  onEditCategory: (categoryId: string) => void
  onCreateChannel: (categoryId: string | undefined) => void
  onEditChannel: (channelId: string) => void
}

export function ChannelSidebar({
  server,
  categories,
  selectedChannelId,
  unreadChannelIds,
  voiceParticipants,
  members,
  selfMemberId,
  onSelectChannel,
  onInvite,
  canCreateInvites,
  canManageMembers,
  onManageRoles,
  onEditNickname,
  canManageChannels,
  canManageRoles,
  onEditChannelPermissions,
  onCreateCategory,
  onEditCategory,
  onCreateChannel,
  onEditChannel,
}: ChannelSidebarProps) {
  const nicknameById = new Map(members.map((m) => [m.id, m.nickname]))

  return (
    <nav className="channel-sidebar" aria-label="Canais">
      <div className="server-name">
        <span>{server.name}</span>
        <div className="server-name-actions">
          {canManageChannels && (
            <button type="button" className="invite-button" onClick={onCreateCategory}>
              + Categoria
            </button>
          )}
          {canManageMembers && (
            <button type="button" className="invite-button" onClick={onManageRoles}>
              Membros
            </button>
          )}
          <button type="button" className="invite-button" onClick={onEditNickname}>
            Apelido
          </button>
          {canCreateInvites && (
            <button type="button" className="invite-button" onClick={onInvite}>
              Convidar
            </button>
          )}
        </div>
      </div>
      <div className="category-list">
        {canManageChannels && categories.length === 0 && (
          <p className="structure-empty-hint">
            Servidor sem canais. Crie uma categoria em "+ Categoria" ou{' '}
            <button type="button" className="structure-inline-link" onClick={() => onCreateChannel(undefined)}>
              um canal sem categoria
            </button>
            .
          </p>
        )}
        {categories.map((category) => (
          <div className="category" key={category.id}>
            <div className="category-name">
              <span>{category.name}</span>
              {canManageChannels && (
                <span className="structure-actions">
                  <button
                    type="button"
                    title="Novo canal nesta categoria"
                    aria-label={`Novo canal em ${category.name}`}
                    onClick={() => onCreateChannel(category.id === UNCATEGORIZED_ID ? undefined : category.id)}
                  >
                    +
                  </button>
                  {category.id !== UNCATEGORIZED_ID && (
                    <button
                      type="button"
                      title="Editar categoria"
                      aria-label={`Editar categoria ${category.name}`}
                      onClick={() => onEditCategory(category.id)}
                    >
                      ✎
                    </button>
                  )}
                </span>
              )}
            </div>
            <ul>
              {category.channels.map((channel) => (
                <Fragment key={channel.id}>
                  <li className="channel-row">
                    <button
                      type="button"
                      className={
                        channel.id === selectedChannelId
                          ? 'channel-item active'
                          : 'channel-item'
                      }
                      onClick={() => onSelectChannel(channel.id)}
                    >
                      <span className="channel-icon">
                        {CHANNEL_ICON[channel.type]}
                      </span>
                      {channel.name}
                      {unreadChannelIds.has(channel.id) && (
                        <span className="unread-dot" aria-label="mensagens não lidas" />
                      )}
                    </button>
                    {(canManageRoles || canManageChannels) && (
                      <span className="channel-row-actions">
                        {canManageRoles && (
                          <button
                            type="button"
                            className="channel-edit-button"
                            title="Permissões do canal"
                            aria-label={`Permissões do canal ${channel.name}`}
                            onClick={() => onEditChannelPermissions(channel.id)}
                          >
                            🔒
                          </button>
                        )}
                        {canManageChannels && (
                          <button
                            type="button"
                            className="channel-edit-button"
                            title="Editar canal"
                            aria-label={`Editar canal ${channel.name}`}
                            onClick={() => onEditChannel(channel.id)}
                          >
                            ✎
                          </button>
                        )}
                      </span>
                    )}
                  </li>
                  {channel.type === 'voice' && voiceParticipants[channel.id] && (
                    <li>
                      <ul className="voice-participants" aria-label={`Na sala ${channel.name}`}>
                        {voiceParticipants[channel.id].map((p) => (
                          <li key={p.memberId} className="voice-participant">
                            <span className="voice-participant-dot" aria-hidden="true" />
                            {nicknameById.get(p.memberId) ?? (p.name || p.memberId.slice(0, 8))}
                            {p.memberId === selfMemberId && <span className="voice-participant-self">(você)</span>}
                          </li>
                        ))}
                      </ul>
                    </li>
                  )}
                </Fragment>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </nav>
  )
}
