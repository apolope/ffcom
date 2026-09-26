import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ServerRail } from './components/ServerRail'
import { ChannelSidebar } from './components/ChannelSidebar'
import { MainPanel } from './components/MainPanel'
import { MemberList, type MemberFriendActions } from './components/MemberList'
import { LoginScreen } from './components/LoginScreen'
import { AddServerDialog } from './components/AddServerDialog'
import { InviteServerDialog } from './components/InviteServerDialog'
import { ManageRolesDialog } from './components/ManageRolesDialog'
import { FriendsView } from './components/FriendsView'
import { AddFriendDialog } from './components/AddFriendDialog'
import { DirectMessageView } from './components/DirectMessageView'
import { NicknameDialog } from './components/NicknameDialog'
import { AvatarDialog } from './components/AvatarDialog'
import { CategoryDialog, ChannelDialog } from './components/StructureDialogs'
import { ChannelPermissionsDialog } from './components/ChannelPermissionsDialog'
import { PresenceContext, visibleOwnStatus, type PresenceContextValue } from './components/PresenceContext'
import { useAuth } from './auth/AuthProvider'
import { useServerStructure } from './hooks/useServerStructure'
import { useServersUnread } from './hooks/useServersUnread'
import { useAppUpdate } from './hooks/useAppUpdate'
import { useAccountsBySubject } from './hooks/useAccountsBySubject'
import { useIdle } from './hooks/useIdle'
import { useKnownServers } from './hooks/useKnownServers'
import { useFriends, type FriendEvent } from './hooks/useFriends'
import { useE2EKeys } from './hooks/useE2EKeys'
import { E2EKeyPanel } from './components/E2EKeyPanel'
import { useMe } from './hooks/useMe'
import { useMyProfile } from './hooks/useMyProfile'
import { useServerMembers } from './hooks/useServerMembers'
import { useUnread } from './hooks/useUnread'
import { useVoiceParticipants } from './hooks/useVoiceParticipants'
import { NotificationsContext, useNotificationCenter } from './hooks/useNotificationCenter'
import { NotificationStack } from './components/NotificationStack'
import {
  UNCATEGORIZED_ID,
  createCategory,
  createChannel,
  createServerInvite,
  deleteCategory,
  deleteChannel,
  deleteChannelOverwrite,
  fetchChannelOverwrites,
  setChannelOverwrite,
  updateCategory,
  updateChannel,
} from './lib/serverChannelApi'
import { friendRequestName, sendPresenceIdleFrame } from './lib/serverCentralApi'
import { markRead } from './lib/unread'
import { NO_STRUCTURE_PERMISSIONS, PERMISSIONS, hasPermission, structurePermissionsOf } from './lib/permissions'
import type { Category } from './types'
import './App.css'

function App() {
  const { status, user, accessToken, signOut } = useAuth()
  // `sub` do OIDC: chave de E2E e cursores de não lida em localStorage são
  // por conta, não por navegador (ver crypto/e2e.ts, lib/unread.ts).
  const accountSub = user?.profile.sub ?? ''
  const { updateReady, applyUpdate } = useAppUpdate()
  // Sem sessão não há chamada de voz para derrubar: aplica a versão nova
  // sozinha. Logado, só pelo botão do ServerRail.
  useEffect(() => {
    if (updateReady && status === 'signed-out') applyUpdate()
  }, [updateReady, status, applyUpdate])
  const { profile: myProfile, uploadAvatar, removeAvatar, setStatus: setMyStatus } = useMyProfile(accessToken ?? '')
  // Mensagens na tela (components/NotificationStack.tsx); desligadas com o
  // status "Ocupado" escolhido pela própria pessoa.
  const { notifications, notify, dismiss: dismissNotification } = useNotificationCenter(myProfile?.status === 'busy')
  const onServerOrderSaveError = useCallback(
    () => notify('Falha ao salvar a ordem dos servidores. Tentando de novo…', 'error'),
    [notify],
  )
  const { servers, addServer, saveOrder: saveServerOrder } = useKnownServers(accessToken ?? '', onServerOrderSaveError)
  // Pedido de amizade recebido ou aceito chega pelo WebSocket de presença,
  // com a pessoa em qualquer tela: vira aviso no canto (ver
  // docs/architecture.md, "Decisão: pedido de amizade pela lista de membros").
  const onFriendEvent = useCallback(
    (event: FriendEvent) => {
      if (event.kind === 'request-received') {
        notify(`${friendRequestName(event.request)} te enviou um pedido de amizade. Veja em Amigos.`)
      } else {
        notify(`${friendRequestName(event)} aceitou seu pedido de amizade.`, 'success')
      }
    },
    [notify],
  )
  const {
    friends,
    createInvite,
    redeemInvite,
    incomingRequests,
    outgoingRequests,
    sendRequest: sendFriendRequest,
    acceptRequest: acceptFriendRequest,
    removeRequest: removeFriendRequest,
    socket: presenceSocket,
  } = useFriends(accessToken ?? '', onFriendEvent)
  const e2eKeys = useE2EKeys(accessToken ?? '', accountSub)
  const myE2EKeyPair = e2eKeys.keyPair
  const [selectedServerId, setSelectedServerId] = useState<string>()
  const [showFriends, setShowFriends] = useState(false)
  const [showAddServer, setShowAddServer] = useState(false)
  const [showInviteServer, setShowInviteServer] = useState(false)
  const [showManageRoles, setShowManageRoles] = useState(false)
  const [showAddFriend, setShowAddFriend] = useState(false)
  const [showEditNickname, setShowEditNickname] = useState(false)
  const [showMyAvatar, setShowMyAvatar] = useState(false)
  const [selectedFriendId, setSelectedFriendId] = useState<string>()
  // Diálogos de estrutura: undefined = fechado; id ausente = criar.
  const [categoryDialog, setCategoryDialog] = useState<{ id?: string }>()
  const [channelDialog, setChannelDialog] = useState<{ id?: string; categoryId?: string }>()
  const [permissionsChannelId, setPermissionsChannelId] = useState<string>()

  const selectedFriend = friends.find((f) => f.accountId === selectedFriendId)

  // Ações do botão direito na lista de membros. Erros (pedido duplicado,
  // conta sumiu) voltam do servidor como texto e viram aviso.
  const memberFriendActions = useMemo<MemberFriendActions>(() => {
    const friendIds = new Set(friends.map((f) => f.accountId))
    const incomingByAccount = new Map(incomingRequests.map((r) => [r.accountId, r]))
    const outgoingIds = new Set(outgoingRequests.map((r) => r.accountId))
    const fail = (err: unknown) =>
      notify(err instanceof Error ? err.message : 'Falha ao atualizar a amizade.', 'error')
    return {
      relationOf: (accountId) => {
        if (accountId === myProfile?.accountId) return 'self'
        if (friendIds.has(accountId)) return 'friend'
        if (incomingByAccount.has(accountId)) return 'incoming'
        if (outgoingIds.has(accountId)) return 'outgoing'
        return 'none'
      },
      onAddFriend: (accountId, nickname) => {
        sendFriendRequest(accountId)
          .then((result) =>
            notify(
              result === 'accepted'
                ? `Você e ${nickname} agora são amigos.`
                : `Pedido de amizade enviado para ${nickname}.`,
              'success',
            ),
          )
          .catch(fail)
      },
      onAcceptRequest: (accountId) => {
        const request = incomingByAccount.get(accountId)
        if (request) acceptFriendRequest(request.id).catch(fail)
      },
      onDeclineRequest: (accountId) => {
        const request = incomingByAccount.get(accountId)
        if (request) removeFriendRequest(request.id).catch(fail)
      },
      onMessage: (accountId) => {
        setShowFriends(true)
        setSelectedFriendId(accountId)
      },
    }
  }, [
    friends,
    incomingRequests,
    outgoingRequests,
    myProfile?.accountId,
    notify,
    sendFriendRequest,
    acceptFriendRequest,
    removeFriendRequest,
  ])

  useEffect(() => {
    if (selectedServerId && servers.some((s) => s.id === selectedServerId)) return
    setSelectedServerId(servers[0]?.id)
  }, [servers, selectedServerId])

  const server = servers.find((s) => s.id === selectedServerId)
  // Nome do perfil do Authentik, gravado em cada servidor como nome exibido
  // de quem não escolheu apelido (ver hooks/useMe.ts). A lista de membros
  // recarrega quando ele é gravado; useServerMembers vem depois porque
  // depende das permissões de me, daí o ref.
  const profileName = (user?.profile.name || user?.profile.preferred_username)?.trim() || undefined
  const refreshMembersRef = useRef<() => Promise<void>>(undefined)
  const onProfileNameSaved = useCallback(() => void refreshMembersRef.current?.(), [])
  const { me, setNickname } = useMe(server?.baseUrl ?? '', accessToken ?? '', profileName, onProfileNameSaved)
  const canManageRoles = me ? hasPermission(me.permissions, PERMISSIONS.ManageRoles) || !!me.isOwner : false
  const canKick = me ? hasPermission(me.permissions, PERMISSIONS.KickMembers) || !!me.isOwner : false
  const canBan = me ? hasPermission(me.permissions, PERMISSIONS.BanMembers) || !!me.isOwner : false
  const canOpenMemberAdmin = canManageRoles || canKick || canBan
  // Mesmo critério do POST /api/invites em server-channel: CreateInvites ou
  // ManageInvites (quem gerencia também cria).
  const canCreateInvites = me
    ? hasPermission(me.permissions, PERMISSIONS.CreateInvites | PERMISSIONS.ManageInvites) || !!me.isOwner
    : false
  const structurePermissions = me ? structurePermissionsOf(me.permissions, !!me.isOwner) : NO_STRUCTURE_PERMISSIONS
  // Mesmo critério de handleIncomingMessageDelete em server-channel: só
  // Administrator apaga mensagem alheia (não há bit dedicado a mensagens).
  const canModerateMessages = me ? hasPermission(me.permissions, PERMISSIONS.Administrator) || !!me.isOwner : false
  const {
    members,
    roles,
    bans,
    createRole,
    deleteRole,
    assignRole,
    removeRole,
    kickMember,
    banMember,
    unbanMember,
    refresh: refreshMembers,
  } = useServerMembers(server?.baseUrl ?? '', accessToken ?? '', canBan)
  useEffect(() => {
    refreshMembersRef.current = refreshMembers
  }, [refreshMembers])

  // Status de presença e avatar nas listas (ver docs/architecture.md,
  // "Decisão: status de presença e avatar nas listas de membros"). A
  // ociosidade vai para server-central pela conexão de presença, que decide
  // o "ausente" que os amigos veem; numa conexão nova, o estado atual é
  // reenviado assim que ela abre.
  const idle = useIdle()
  useEffect(() => {
    if (!presenceSocket) return
    const send = () => sendPresenceIdleFrame(presenceSocket, idle)
    if (presenceSocket.readyState === WebSocket.OPEN) {
      send()
      return
    }
    presenceSocket.addEventListener('open', send, { once: true })
    return () => presenceSocket.removeEventListener('open', send)
  }, [presenceSocket, idle])
  const memberSubjects = useMemo(
    () => members.flatMap((m) => (m.oidcSubject ? [m.oidcSubject] : [])),
    [members],
  )
  const accountsBySubject = useAccountsBySubject(accessToken ?? '', memberSubjects)
  const ownStatus = visibleOwnStatus(myProfile?.status, idle)
  const presence = useMemo<PresenceContextValue>(() => {
    const friendStatus = new Map(friends.map((f) => [f.accountId, f.status]))
    return {
      statusOf: (accountId) => {
        if (!accountId) return 'offline'
        if (accountId === myProfile?.accountId) return ownStatus
        return friendStatus.get(accountId) ?? 'offline'
      },
      // A própria conta vem do perfil, para o avatar novo aparecer na hora.
      accountOf: (oidcSubject) => {
        if (!oidcSubject) return undefined
        if (myProfile && oidcSubject === myProfile.oidcSubject) return myProfile
        return accountsBySubject.get(oidcSubject)
      },
    }
  }, [friends, myProfile, ownStatus, accountsBySubject])

  const onStructureSaveError = useCallback(
    () => notify('Falha ao salvar a ordem das categorias e canais. Tentando de novo…', 'error'),
    [notify],
  )
  const {
    categories,
    refresh: refreshStructure,
    saveCategoryOrder,
    saveChannelOrder,
  } = useServerStructure(server?.baseUrl ?? '', accessToken ?? '', onStructureSaveError)
  const realCategories = useMemo(() => categories.filter((c) => c.id !== UNCATEGORIZED_ID), [categories])
  const serverBaseUrl = server?.baseUrl
  // Estável por canal: ChannelPermissionsDialog recarrega quando muda.
  const loadChannelOverwrites = useCallback(
    () => fetchChannelOverwrites(serverBaseUrl ?? '', accessToken ?? '', permissionsChannelId ?? ''),
    [serverBaseUrl, accessToken, permissionsChannelId],
  )
  const permissionsChannel = findChannelForDialog(categories, permissionsChannelId)
  const hasVoiceChannel = categories.some((category) => category.channels.some((c) => c.type === 'voice'))
  const voiceParticipants = useVoiceParticipants(server?.baseUrl ?? '', accessToken ?? '', hasVoiceChannel)

  const [selectedChannelId, setSelectedChannelId] = useState<string>()
  // categories muda a cada repoll de useServerStructure (20s), não só ao
  // trocar de servidor: manter o canal selecionado se ele ainda existe, e só
  // cair no primeiro canal quando ele sumiu (servidor novo, canal apagado).
  // Resetar sempre tirava a pessoa do canal a cada poll, e num canal de voz
  // isso desmontava VoiceChannelView e derrubava a chamada.
  useEffect(() => {
    setSelectedChannelId((prev) => {
      const all = categories.flatMap((category) => category.channels)
      return prev && all.some((c) => c.id === prev) ? prev : all[0]?.id
    })
  }, [categories])

  const channel = useMemo(
    () =>
      categories
        .flatMap((category) => category.channels)
        .find((c) => c.id === selectedChannelId),
    [categories, selectedChannelId],
  )

  // Marca o canal como lido ao entrar nele e de novo ao sair (cursor "agora"
  // -- ver lib/unread.ts) para não deixar mensagens vistas enquanto o canal
  // estava aberto acusando "não lida" depois. Independe de showFriends: só
  // useUnread abaixo decide se o canal selecionado conta como ativo agora.
  useEffect(() => {
    if (!selectedChannelId) return
    markRead(accountSub, 'channel', selectedChannelId)
    return () => markRead(accountSub, 'channel', selectedChannelId)
  }, [accountSub, selectedChannelId])

  useEffect(() => {
    if (!selectedFriendId) return
    markRead(accountSub, 'dm', selectedFriendId)
    return () => markRead(accountSub, 'dm', selectedFriendId)
  }, [accountSub, selectedFriendId])

  const unreadChannelIds = useUnread(
    accountSub,
    'channel',
    useMemo(
      () =>
        categories
          .flatMap((category) => category.channels)
          .map((c) => ({ id: c.id, lastMessageAt: c.lastMessageAt })),
      [categories],
    ),
    showFriends ? undefined : selectedChannelId,
  )

  const unreadFriendIds = useUnread(
    accountSub,
    'dm',
    useMemo(() => friends.map((f) => ({ id: f.accountId, lastMessageAt: f.lastMessageAt })), [friends]),
    showFriends ? selectedFriendId : undefined,
  )

  // Agregado do ServerRail: só acende o que a pessoa não está vendo. O
  // servidor aberto conta pelo unreadChannelIds acima (já sem o canal
  // selecionado) e só quando a tela de Amigos está na frente; os demais
  // vêm de useServersUnread. DMs, idem, só fora da tela de Amigos.
  const backgroundUnreadServerIds = useServersUnread(servers, selectedServerId, accessToken ?? '', accountSub)
  const openServerUnread = showFriends && unreadChannelIds.size > 0
  const unreadServerIds = useMemo(() => {
    if (!openServerUnread || !selectedServerId) return backgroundUnreadServerIds
    return new Set([...backgroundUnreadServerIds, selectedServerId])
  }, [backgroundUnreadServerIds, openServerUnread, selectedServerId])

  if (status === 'loading') {
    return null
  }

  if (status === 'signed-out') {
    return <LoginScreen />
  }

  return (
    <PresenceContext.Provider value={presence}>
      <NotificationsContext.Provider value={notify}>
        <div className="app-shell">
          <ServerRail
            servers={servers}
            selectedServerId={selectedServerId}
            friendsSelected={showFriends}
            unreadServerIds={unreadServerIds}
            friendsUnread={!showFriends && (unreadFriendIds.size > 0 || incomingRequests.length > 0)}
            updateReady={updateReady}
            onUpdate={applyUpdate}
            myProfile={myProfile}
            myStatus={ownStatus}
            onSetStatus={(next) => {
              setMyStatus(next).catch(() => {
                /* setStatus já desfez a troca otimista */
              })
            }}
            onSelectServer={(id) => {
              setShowFriends(false)
              setSelectedServerId(id)
            }}
            serverMenuActions={{
              loading: !me,
              // Sem o bit o POST de convite daria 403 (CreateInvites/ManageInvites ou dono).
              onInvite: canCreateInvites ? () => setShowInviteServer(true) : undefined,
              onManageMembers: canOpenMemberAdmin ? () => setShowManageRoles(true) : undefined,
            }}
            onReorderServers={saveServerOrder}
          onSelectFriends={() => setShowFriends(true)}
            onAddServer={() => setShowAddServer(true)}
            onOpenMyAvatar={() => setShowMyAvatar(true)}
            onEditNickname={!showFriends && server && me ? () => setShowEditNickname(true) : undefined}
            onSignOut={signOut}
          />
          {showFriends ? (
            <>
              <FriendsView
                friends={friends}
                selectedFriendId={selectedFriendId}
                unreadFriendIds={unreadFriendIds}
                onSelectFriend={setSelectedFriendId}
                onAddFriend={() => setShowAddFriend(true)}
                incomingRequests={incomingRequests}
                outgoingRequests={outgoingRequests}
                onAcceptRequest={(id) => {
                  acceptFriendRequest(id).catch((err: unknown) =>
                    notify(err instanceof Error ? err.message : 'Falha ao aceitar o pedido.', 'error'),
                  )
                }}
                onRemoveRequest={(id) => {
                  removeFriendRequest(id).catch((err: unknown) =>
                    notify(err instanceof Error ? err.message : 'Falha ao remover o pedido.', 'error'),
                  )
                }}
              />
              {!myE2EKeyPair ? (
                <E2EKeyPanel e2e={e2eKeys} />
              ) : selectedFriend ? (
                <DirectMessageView
                  key={selectedFriend.accountId}
                  peer={selectedFriend}
                  accessToken={accessToken ?? ''}
                  socket={presenceSocket}
                  myKeyPair={myE2EKeyPair}
                />
              ) : (
                <div className="empty-state">
                  <p>Selecione um amigo para conversar.</p>
                </div>
              )}
            </>
          ) : server ? (
            <>
              <ChannelSidebar
                server={server}
                categories={categories}
                selectedChannelId={selectedChannelId}
                unreadChannelIds={unreadChannelIds}
                voiceParticipants={voiceParticipants}
                members={members}
                selfMemberId={me?.memberId}
                onSelectChannel={setSelectedChannelId}
                structure={structurePermissions}
                canManageRoles={canManageRoles}
                onEditChannelPermissions={setPermissionsChannelId}
                onCreateCategory={() => setCategoryDialog({})}
                onEditCategory={(id) => setCategoryDialog({ id })}
                onCreateChannel={(categoryId) => setChannelDialog({ categoryId })}
                onEditChannel={(id) => setChannelDialog({ id })}
                onReorderCategories={saveCategoryOrder}
                onReorderChannels={saveChannelOrder}
              />
              <MainPanel channel={channel} serverBaseUrl={server.baseUrl} canModerateMessages={canModerateMessages} />
              <MemberList members={members} roles={roles} friendActions={memberFriendActions} />
            </>
          ) : (
            <div className="empty-state">
              <p>Nenhum servidor ainda. Adicione um pelo botão "+" na barra lateral.</p>
            </div>
          )}
          {showAddServer && (
            <AddServerDialog onAdd={addServer} onClose={() => setShowAddServer(false)} />
          )}
          {showInviteServer && server && (
            <InviteServerDialog
              serverName={server.name}
              serverBaseUrl={server.baseUrl}
              onCreateInvite={async () => {
                const invite = await createServerInvite(server.baseUrl, accessToken ?? '')
                return invite.code
              }}
              onClose={() => setShowInviteServer(false)}
            />
          )}
          {showManageRoles && (
            <ManageRolesDialog
              members={members}
              roles={roles}
              bans={bans}
              currentMemberId={me?.memberId}
              canManageRoles={canManageRoles}
              canKick={canKick}
              canBan={canBan}
              onCreateRole={createRole}
              onDeleteRole={deleteRole}
              onAssignRole={assignRole}
              onRemoveRole={removeRole}
              onKick={kickMember}
              onBan={banMember}
              onUnban={unbanMember}
              onClose={() => setShowManageRoles(false)}
            />
          )}
          {categoryDialog && server && (
            <CategoryDialog
              category={realCategories.find((c) => c.id === categoryDialog.id)}
              canRename={structurePermissions.rename}
              onSave={async (name) => {
                if (categoryDialog.id) {
                  await updateCategory(server.baseUrl, accessToken ?? '', categoryDialog.id, { name })
                } else {
                  await createCategory(server.baseUrl, accessToken ?? '', name)
                }
                refreshStructure()
              }}
              onDelete={
                structurePermissions.deleteCategories
                  ? async () => {
                      if (!categoryDialog.id) return
                      await deleteCategory(server.baseUrl, accessToken ?? '', categoryDialog.id)
                      refreshStructure()
                    }
                  : undefined
              }
              onClose={() => setCategoryDialog(undefined)}
            />
          )}
          {permissionsChannel && server && (
            <ChannelPermissionsDialog
              key={permissionsChannel.id}
              channel={permissionsChannel}
              roles={roles}
              myPermissions={me?.permissions ?? 0}
              isOwner={!!me?.isOwner}
              onLoad={loadChannelOverwrites}
              onSet={async (roleId, overwrite) => {
                await setChannelOverwrite(server.baseUrl, accessToken ?? '', permissionsChannel.id, roleId, overwrite)
              }}
              onDelete={(roleId) => deleteChannelOverwrite(server.baseUrl, accessToken ?? '', permissionsChannel.id, roleId)}
              onSaved={refreshStructure}
              onClose={() => setPermissionsChannelId(undefined)}
            />
          )}
          {channelDialog && server && (
            <ChannelDialog
              channel={findChannelForDialog(categories, channelDialog.id)}
              initialCategoryId={channelDialog.categoryId}
              categories={realCategories}
              canRename={structurePermissions.rename}
              canMove={structurePermissions.reorderChannels}
              onSave={async ({ name, type, categoryId }) => {
                const existing = findChannelForDialog(categories, channelDialog.id)
                if (existing) {
                  // Só manda o que mudou: renomear e mover exigem permissões
                  // diferentes, e mandar o nome igual pediria ManageChannels à toa.
                  const changes: { name?: string; categoryId?: string | null } = {}
                  if (name !== existing.name) changes.name = name
                  if (categoryId !== existing.categoryId) changes.categoryId = categoryId ?? null
                  if (Object.keys(changes).length === 0) return
                  await updateChannel(server.baseUrl, accessToken ?? '', existing.id, changes)
                } else {
                  const created = await createChannel(server.baseUrl, accessToken ?? '', { name, type, categoryId })
                  setSelectedChannelId(created.id)
                }
                refreshStructure()
              }}
              onDelete={
                structurePermissions.deleteChannels
                  ? async () => {
                      if (!channelDialog.id) return
                      await deleteChannel(server.baseUrl, accessToken ?? '', channelDialog.id)
                      refreshStructure()
                    }
                  : undefined
              }
              onClose={() => setChannelDialog(undefined)}
            />
          )}
          {showAddFriend && (
            <AddFriendDialog
              onCreateInvite={createInvite}
              onRedeemInvite={redeemInvite}
              onClose={() => setShowAddFriend(false)}
            />
          )}
          {showEditNickname && (
            <NicknameDialog
              currentNickname={me?.nickname}
              onSave={async (nickname) => {
                await setNickname(nickname)
                await refreshMembers()
              }}
              onClose={() => setShowEditNickname(false)}
            />
          )}
          {showMyAvatar && (
            <AvatarDialog
              profile={myProfile}
              onUpload={uploadAvatar}
              onRemove={removeAvatar}
              onClose={() => setShowMyAvatar(false)}
            />
          )}
        </div>
        <NotificationStack notifications={notifications} onDismiss={dismissNotification} />
      </NotificationsContext.Provider>
    </PresenceContext.Provider>
  )
}

// Canal em edição com a categoria real dele (a sintética "Canais" vira
// "sem categoria"), no formato esperado por ChannelDialog.
function findChannelForDialog(categories: Category[], channelId: string | undefined) {
  if (!channelId) return undefined
  for (const category of categories) {
    const found = category.channels.find((c) => c.id === channelId)
    if (found) {
      return { ...found, categoryId: category.id === UNCATEGORIZED_ID ? undefined : category.id }
    }
  }
  return undefined
}

export default App
