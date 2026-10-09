import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChosenStatus, KnownServer, PresenceStatus } from '../types'
import type { Language } from '../i18n'
import type { MyProfile } from '../lib/serverCentralApi'
import { AvatarWithStatus } from './AvatarWithStatus'
import { statusLabel } from './PresenceContext'
import { MutedIcon } from './MutedIcon'
import { ServerMenu, type ServerMenuActions } from './ServerMenu'
import { StatusMenu, type AccountIdentity } from './StatusMenu'
import { UpdateButton } from './UpdateButton'
import { useScrollEdges } from '../hooks/useScrollEdges'
import type { UpdateKind } from '../hooks/useAppUpdate'
import './ServerRail.css'

interface ServerRailProps {
  servers: KnownServer[]
  selectedServerId: string | undefined
  friendsSelected: boolean
  // Indicador agregado de não lida (ver docs/architecture.md, "Decisão:
  // indicador de não lida"): servidores com canal não lido e se há DM não
  // lida. App.tsx só acende o que a pessoa não está vendo agora.
  unreadServerIds: Set<string>
  friendsUnread: boolean
  // Versão nova do client esperando (hooks/useAppUpdate.ts) e de quem é.
  updateReady: boolean
  updateKind: UpdateKind | undefined
  onUpdate: () => void
  myProfile: MyProfile | undefined
  // Dados do Authentik e do servidor aberto para o cabeçalho do menu do
  // avatar (nome e avatar vêm de myProfile quando houver).
  account: Omit<AccountIdentity, 'displayName' | 'avatarUrl'> & { profileName?: string }
  // Como a pessoa aparece para os amigos agora (escolha + ociosidade).
  myStatus: PresenceStatus
  onSetStatus: (status: ChosenStatus) => void
  onSetLanguage: (language: Language) => void
  onSelectServer: (serverId: string) => void
  // Ordem nova completa do rail depois de arrastar um ícone.
  onReorderServers: (serverIds: string[]) => void
  // Ações do menu de contexto do servidor selecionado (o botão direito
  // seleciona o servidor antes de abrir o menu).
  serverMenuActions: ServerMenuActions
  // Servidor com notificações silenciadas (push do app Android): 🔕 no canto
  // do ícone. Ausente onde não há push.
  isServerMuted?: (server: KnownServer) => boolean
  onSelectFriends: () => void
  onAddServer: () => void
  onOpenMyAvatar: () => void
  // Ausente enquanto o perfil de server-central não carregou.
  onEditDisplayName?: () => void
  // Ausente fora de um servidor (apelido é por server-channel).
  onEditNickname?: () => void
  onSignOut: () => void
}

export function ServerRail({
  servers,
  selectedServerId,
  friendsSelected,
  unreadServerIds,
  friendsUnread,
  updateReady,
  updateKind,
  onUpdate,
  myProfile,
  account,
  myStatus,
  onSetStatus,
  onSetLanguage,
  onSelectServer,
  onReorderServers,
  serverMenuActions,
  isServerMuted,
  onSelectFriends,
  onAddServer,
  onOpenMyAvatar,
  onEditDisplayName,
  onEditNickname,
  onSignOut,
}: ServerRailProps) {
  const { t } = useTranslation()
  // Âncora do menu de status (undefined = fechado).
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement>()
  const closeMenu = useCallback(() => setMenuAnchor(undefined), [])
  const chosen = myProfile?.status ?? 'online'
  const { profileName, ...accountRest } = account
  const displayName = myProfile?.displayName ?? profileName ?? myProfile?.oidcSubject ?? '?'
  const [serverMenu, setServerMenu] = useState<{ anchor: HTMLElement; server: KnownServer }>()

  // Arrastar ícone de servidor (HTML5): qual está sendo arrastado e se cai
  // antes ou depois de qual.
  const [draggingId, setDraggingId] = useState<string>()
  const [dropTarget, setDropTarget] = useState<{ id: string; after: boolean }>()
  const endDrag = () => {
    setDraggingId(undefined)
    setDropTarget(undefined)
  }
  const handleDrop = () => {
    if (draggingId && dropTarget && dropTarget.id !== draggingId) {
      const current = servers.map((s) => s.id)
      const order = current.filter((id) => id !== draggingId)
      order.splice(order.indexOf(dropTarget.id) + (dropTarget.after ? 1 : 0), 0, draggingId)
      if (order.some((id, i) => id !== current[i])) onReorderServers(order)
    }
    endDrag()
  }
  const closeServerMenu = useCallback(() => setServerMenu(undefined), [])

  // Setas de "tem mais servidores" sobre a lista rolável: cada uma só existe
  // enquanto houver para onde rolar naquela direção, então some ao chegar na
  // ponta e não cobre o primeiro nem o último ícone.
  const [scrollRef, edges, scrollEl] = useScrollEdges()
  const scrollToEdge = (edge: 'top' | 'bottom') => {
    const el = scrollEl
    if (!el) return
    el.scrollTo({ top: edge === 'top' ? 0 : el.scrollHeight, behavior: 'smooth' })
  }

  return (
    // Três faixas: Amigos fixo no topo, servidores rolando no meio e
    // adicionar servidor, conta e sair fixos embaixo, para nunca saírem da
    // tela com muitos servidores.
    <nav className="server-rail" aria-label={t('server.rail.label')}>
      <button
        type="button"
        className={friendsSelected ? 'server-icon active' : 'server-icon'}
        onClick={onSelectFriends}
        title={friendsUnread ? t('server.rail.friendsUnread') : t('server.rail.friends')}
      >
        {friendsUnread && <span className="rail-unread-pill" aria-hidden="true" />}
        <svg viewBox="0 0 24 24" width="22" height="22" fill="currentColor" aria-hidden="true">
          <path d="M12 3 2 12h3v8h6v-6h2v6h6v-8h3L12 3z" />
        </svg>
      </button>
      <div
        className={['rail-scroll-area', edges.up && 'more-up', edges.down && 'more-down'].filter(Boolean).join(' ')}
      >
        {edges.up && (
          <button
            type="button"
            className="rail-scroll-hint up"
            title={t('server.rail.scrollTop')}
            aria-label={t('server.rail.scrollTopLabel')}
            onClick={() => scrollToEdge('top')}
          >
            <DoubleChevron />
          </button>
        )}
        <div ref={scrollRef} className="rail-scroll scroll-hidden">
          <ul>
            {servers.map((server) => (
              <li
                key={server.id}
                className={
                  [
                    draggingId === server.id && 'dragging',
                    dropTarget?.id === server.id && dropTarget.id !== draggingId && (dropTarget.after ? 'drop-after' : 'drop-before'),
                  ]
                    .filter(Boolean)
                    .join(' ') || undefined
                }
                draggable
                onDragStart={(event) => {
                  event.dataTransfer.effectAllowed = 'move'
                  // Firefox só inicia o arraste com algum dado no dataTransfer.
                  event.dataTransfer.setData('text/plain', server.id)
                  setDraggingId(server.id)
                }}
                onDragEnd={endDrag}
                onDragOver={(event) => {
                  if (!draggingId) return
                  event.preventDefault()
                  event.dataTransfer.dropEffect = 'move'
                  const rect = event.currentTarget.getBoundingClientRect()
                  const after = event.clientY > rect.top + rect.height / 2
                  if (dropTarget?.id !== server.id || dropTarget.after !== after) setDropTarget({ id: server.id, after })
                }}
                onDrop={(event) => {
                  event.preventDefault()
                  handleDrop()
                }}
              >
                <button
                  type="button"
                  // Com Amigos aberto, selectedServerId continua apontando para o
                  // último servidor (App.tsx volta para ele), mas o ativo é a casa.
                  className={
                    !friendsSelected && server.id === selectedServerId ? 'server-icon active' : 'server-icon'
                  }
                  onClick={() => onSelectServer(server.id)}
                  onContextMenu={(event) => {
                    event.preventDefault()
                    const button = event.currentTarget
                    onSelectServer(server.id)
                    setServerMenu({ anchor: button, server })
                  }}
                  title={unreadServerIds.has(server.id) ? t('server.rail.serverUnread', { name: server.name }) : server.name}
                >
                  {unreadServerIds.has(server.id) && <span className="rail-unread-pill" aria-hidden="true" />}
                  {server.initials}
                  {isServerMuted?.(server) && <MutedIcon variant="badge" />}
                </button>
              </li>
            ))}
          </ul>
          {serverMenu && serverMenu.server.id === selectedServerId && (
            <ServerMenu
              anchor={serverMenu.anchor}
              serverName={serverMenu.server.name}
              actions={serverMenuActions}
              onClose={closeServerMenu}
            />
          )}
        </div>
        {edges.down && (
          <button
            type="button"
            className="rail-scroll-hint down"
            title={t('server.rail.scrollBottom')}
            aria-label={t('server.rail.scrollBottomLabel')}
            onClick={() => scrollToEdge('bottom')}
          >
            <DoubleChevron />
          </button>
        )}
      </div>
      <div className="rail-footer">
        <button
          type="button"
          className="server-icon add-server"
          title={t('server.rail.addServer')}
          onClick={onAddServer}
        >
          +
        </button>
        {updateReady && <UpdateButton kind={updateKind ?? 'web'} onUpdate={onUpdate} />}
        <button
          type="button"
          className="account-button"
          title={t('server.rail.myStatus', { status: statusLabel(t, chosen) })}
          aria-haspopup="menu"
          aria-expanded={!!menuAnchor}
          onClick={(event) => {
            const button = event.currentTarget
            setMenuAnchor((open) => (open ? undefined : button))
          }}
        >
          <AvatarWithStatus
            avatarUrl={myProfile?.avatarUrl}
            displayName={displayName}
            status={myStatus}
            size={44}
          />
        </button>
        {menuAnchor && (
          <StatusMenu
            anchor={menuAnchor}
            identity={{
              ...accountRest,
              displayName,
              avatarUrl: myProfile?.avatarUrl,
              username: accountRest.username !== displayName ? accountRest.username : undefined,
            }}
            chosen={chosen}
            onChoose={onSetStatus}
            onChooseLanguage={onSetLanguage}
            onEditAvatar={onOpenMyAvatar}
            onEditDisplayName={onEditDisplayName}
            onEditNickname={onEditNickname}
            onClose={closeMenu}
          />
        )}
        <button type="button" className="sign-out-button" title={t('auth.signOut')} aria-label={t('auth.signOut')} onClick={onSignOut}>
          <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" aria-hidden="true">
            <path d="M10 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h5v-2H5V5h5V3zm6.6 4.6-1.4 1.4 2 2H9v2h8.2l-2 2 1.4 1.4L21 12l-4.4-4.4z" />
          </svg>
        </button>
      </div>
    </nav>
  )
}

// Duas setas para baixo; a variante "up" é girada no CSS.
function DoubleChevron() {
  return (
    <svg
      viewBox="0 0 24 24"
      width="16"
      height="16"
      fill="none"
      stroke="currentColor"
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m7 6 5 5 5-5" />
      <path d="m7 13 5 5 5-5" />
    </svg>
  )
}
