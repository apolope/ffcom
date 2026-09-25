import { Fragment, useState, type DragEvent } from 'react'
import type { Category, ChannelType, KnownServer, Member } from '../types'
import { UNCATEGORIZED_ID, type ChannelOrderGroup, type VoiceParticipant } from '../lib/serverChannelApi'
import type { StructurePermissions } from '../lib/permissions'
import './ChannelSidebar.css'
import { AvatarWithStatus, MemberAvatar } from './AvatarWithStatus'

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
  // Administração da estrutura, por ação (ver lib/permissions.ts e
  // components/StructureDialogs.tsx). categoryId ausente = sem categoria.
  structure: StructurePermissions
  // Overwrite de canal por role, só com ManageRoles ou dono (ver
  // components/ChannelPermissionsDialog.tsx).
  canManageRoles: boolean
  onEditChannelPermissions: (channelId: string) => void
  onCreateCategory: () => void
  onEditCategory: (categoryId: string) => void
  onCreateChannel: (categoryId: string | undefined) => void
  onEditChannel: (channelId: string) => void
  // Nova ordem completa das categorias reais depois de arrastar uma.
  onReorderCategories: (categoryIds: string[]) => void
  // Nova ordem dos canais das categorias afetadas por um arraste (a de
  // destino e, se mudou de categoria, a de origem).
  onReorderChannels: (groups: ChannelOrderGroup[]) => void
}

// O que está sendo arrastado e onde cairia. Canal solto no cabeçalho ou no
// vão de uma categoria (channelId ausente) vai para o fim dela.
type DragItem = { kind: 'category'; id: string } | { kind: 'channel'; id: string; fromCategoryId: string }
type DropTarget =
  | { kind: 'category'; id: string; after: boolean }
  | { kind: 'channel'; categoryId: string; channelId?: string; after: boolean }

function sameDrop(a: DropTarget | undefined, b: DropTarget) {
  return JSON.stringify(a) === JSON.stringify(b)
}

// Metade de baixo do elemento = soltar depois dele.
function isAfter(event: DragEvent<HTMLElement>) {
  const rect = event.currentTarget.getBoundingClientRect()
  return event.clientY > rect.top + rect.height / 2
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
  structure,
  canManageRoles,
  onEditChannelPermissions,
  onCreateCategory,
  onEditCategory,
  onCreateChannel,
  onEditChannel,
  onReorderCategories,
  onReorderChannels,
}: ChannelSidebarProps) {
  const memberById = new Map(members.map((m) => [m.id, m]))
  const canEditChannel = structure.rename || structure.reorderChannels || structure.deleteChannels
  const canEditCategory = structure.rename || structure.deleteCategories

  // Arrastar e soltar (HTML5). Categoria cai antes ou depois de outra; a
  // sintética "Canais" não se move nem recebe categoria (fica no topo), mas
  // recebe canal (= tirar da categoria). Canal cai antes ou depois de outro
  // canal, de qualquer categoria, ou no fim de uma categoria.
  const [dragging, setDragging] = useState<DragItem>()
  const [dropTarget, setDropTarget] = useState<DropTarget>()

  function updateDrop(event: DragEvent<HTMLElement>, target: DropTarget) {
    event.preventDefault()
    event.dataTransfer.dropEffect = 'move'
    if (!sameDrop(dropTarget, target)) setDropTarget(target)
  }

  function handleCategoryDragOver(event: DragEvent<HTMLDivElement>, categoryId: string) {
    if (dragging?.kind === 'category' && categoryId !== UNCATEGORIZED_ID) {
      updateDrop(event, { kind: 'category', id: categoryId, after: isAfter(event) })
    } else if (dragging?.kind === 'channel') {
      updateDrop(event, { kind: 'channel', categoryId, after: true })
    }
  }

  function handleChannelDragOver(event: DragEvent<HTMLLIElement>, categoryId: string, channelId: string) {
    if (dragging?.kind !== 'channel') return
    // Sem isto, o dragover da categoria em volta trocaria o alvo para "fim
    // da categoria".
    event.stopPropagation()
    updateDrop(event, { kind: 'channel', categoryId, channelId, after: isAfter(event) })
  }

  function endDrag() {
    setDragging(undefined)
    setDropTarget(undefined)
  }

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault()
    if (dragging?.kind === 'category' && dropTarget?.kind === 'category' && dropTarget.id !== dragging.id) {
      const current = categories.filter((c) => c.id !== UNCATEGORIZED_ID).map((c) => c.id)
      const order = current.filter((id) => id !== dragging.id)
      order.splice(order.indexOf(dropTarget.id) + (dropTarget.after ? 1 : 0), 0, dragging.id)
      if (order.some((id, i) => id !== current[i])) onReorderCategories(order)
    }
    if (dragging?.kind === 'channel' && dropTarget?.kind === 'channel' && dropTarget.channelId !== dragging.id) {
      const groups = channelDropGroups(categories, dragging, dropTarget)
      if (groups) onReorderChannels(groups)
    }
    endDrag()
  }

  return (
    <nav className="channel-sidebar" aria-label="Canais">
      <div className="server-name">
        <span>{server.name}</span>
      </div>
      <div className="category-list">
        {structure.createChannels && categories.length === 0 && (
          <p className="structure-empty-hint">
            Servidor sem canais. Crie uma categoria abaixo ou{' '}
            <button type="button" className="structure-inline-link" onClick={() => onCreateChannel(undefined)}>
              um canal sem categoria
            </button>
            .
          </p>
        )}
        {categories.map((category) => {
          const draggable = structure.reorderCategories && category.id !== UNCATEGORIZED_ID
          let dropClass = ''
          if (dropTarget?.kind === 'category' && dropTarget.id === category.id && dropTarget.id !== dragging?.id) {
            dropClass = dropTarget.after ? ' drop-after' : ' drop-before'
          } else if (dropTarget?.kind === 'channel' && dropTarget.categoryId === category.id && !dropTarget.channelId) {
            dropClass = ' drop-into'
          }
          return (
            <div
              className={`category${dragging?.id === category.id ? ' dragging' : ''}${dropClass}`}
              key={category.id}
              onDragOver={(event) => handleCategoryDragOver(event, category.id)}
              onDrop={handleDrop}
            >
              <div
                className={draggable ? 'category-name draggable' : 'category-name'}
                draggable={draggable}
                title={draggable ? 'Arraste para reordenar' : undefined}
                onDragStart={(event) => {
                  event.dataTransfer.effectAllowed = 'move'
                  // Firefox só inicia o arraste com algum dado no dataTransfer.
                  event.dataTransfer.setData('text/plain', category.id)
                  setDragging({ kind: 'category', id: category.id })
                }}
                onDragEnd={endDrag}
              >
                <span className="category-title">{category.name}</span>
                {canEditCategory && category.id !== UNCATEGORIZED_ID && (
                  <span className="structure-actions">
                    <button
                      type="button"
                      title="Editar categoria"
                      aria-label={`Editar categoria ${category.name}`}
                      onClick={() => onEditCategory(category.id)}
                    >
                      ✎
                    </button>
                  </span>
                )}
                {structure.createChannels && (
                  <button
                    type="button"
                    className="category-add-channel"
                    title="Novo canal nesta categoria"
                    aria-label={`Novo canal em ${category.name}`}
                    onClick={() => onCreateChannel(category.id === UNCATEGORIZED_ID ? undefined : category.id)}
                  >
                    +
                  </button>
                )}
              </div>
              <ul className="category-channels">
                {category.channels.map((channel) => {
                  let rowClass = 'channel-row'
                  if (dragging?.id === channel.id) rowClass += ' dragging'
                  if (dropTarget?.kind === 'channel' && dropTarget.channelId === channel.id && channel.id !== dragging?.id) {
                    rowClass += dropTarget.after ? ' drop-after' : ' drop-before'
                  }
                  return (
                    <Fragment key={channel.id}>
                      <li
                        className={rowClass}
                        draggable={structure.reorderChannels}
                        onDragStart={(event) => {
                          event.dataTransfer.effectAllowed = 'move'
                          event.dataTransfer.setData('text/plain', channel.id)
                          setDragging({ kind: 'channel', id: channel.id, fromCategoryId: category.id })
                        }}
                        onDragEnd={endDrag}
                        onDragOver={(event) => handleChannelDragOver(event, category.id, channel.id)}
                      >
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
                        {(canManageRoles || canEditChannel) && (
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
                            {canEditChannel && (
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
                              <li key={p.memberId} className="sidebar-voice-participant">
                                <VoiceParticipantAvatar
                                  member={memberById.get(p.memberId)}
                                  fallbackName={p.name || p.memberId.slice(0, 8)}
                                />
                                {memberById.get(p.memberId)?.nickname ?? (p.name || p.memberId.slice(0, 8))}
                                {p.memberId === selfMemberId && <span className="voice-participant-self">(você)</span>}
                              </li>
                            ))}
                          </ul>
                        </li>
                      )}
                    </Fragment>
                  )
                })}
              </ul>
            </div>
          )
        })}
        {structure.createCategories && (
          <button
            type="button"
            className="create-category-card"
            aria-label="Criar nova categoria"
            onClick={onCreateCategory}
          >
            <span aria-hidden="true">+</span>
            <span className="create-category-label" aria-hidden="true">
              Criar nova categoria
            </span>
          </button>
        )}
      </div>
    </nav>
  )
}

// Grupos a gravar depois de soltar um canal: a categoria de destino com o
// canal na posição nova e, se ele mudou de categoria, a de origem sem ele.
// undefined quando nada muda. A sintética "Canais" vira categoryId null.
function channelDropGroups(
  categories: Category[],
  dragging: { id: string; fromCategoryId: string },
  drop: { categoryId: string; channelId?: string; after: boolean },
): ChannelOrderGroup[] | undefined {
  const source = categories.find((c) => c.id === dragging.fromCategoryId)
  const target = categories.find((c) => c.id === drop.categoryId)
  if (!source || !target) return undefined
  const apiId = (id: string) => (id === UNCATEGORIZED_ID ? null : id)

  const before = target.channels.map((c) => c.id)
  const ids = before.filter((id) => id !== dragging.id)
  const index = drop.channelId ? ids.indexOf(drop.channelId) + (drop.after ? 1 : 0) : ids.length
  ids.splice(index, 0, dragging.id)

  if (source.id === target.id) {
    return ids.some((id, i) => id !== before[i]) ? [{ categoryId: apiId(target.id), ids }] : undefined
  }
  return [
    { categoryId: apiId(target.id), ids },
    { categoryId: apiId(source.id), ids: source.channels.map((c) => c.id).filter((id) => id !== dragging.id) },
  ]
}

// Quem está na sala de voz: avatar e status do membro, ou só a inicial para
// alguém que ainda não aparece na lista de membros (entrou há pouco).
function VoiceParticipantAvatar({ member, fallbackName }: { member: Member | undefined; fallbackName: string }) {
  if (member) return <MemberAvatar member={member} size={20} />
  return <AvatarWithStatus displayName={fallbackName} status="offline" size={20} />
}
