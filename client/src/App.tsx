import { useCallback, useEffect, useMemo, useRef, useState, type TouchEvent as ReactTouchEvent } from 'react'
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
import { DisplayNameDialog } from './components/DisplayNameDialog'
import { AvatarDialog } from './components/AvatarDialog'
import { InstallPermissionDialog } from './components/InstallPermissionDialog'
import { PushPermissionDialog } from './components/PushPermissionDialog'
import { CategoryDialog, ChannelDialog } from './components/StructureDialogs'
import { ChannelPermissionsDialog } from './components/ChannelPermissionsDialog'
import { PresenceContext, visibleOwnStatus, type PresenceContextValue } from './components/PresenceContext'
import { MOBILE_QUERY, MobileNavContext } from './components/MobileNavContext'
import { MobileNavButton } from './components/MobileNavButtons'
import { useAuth } from './auth/AuthProvider'
import { useServerStructure } from './hooks/useServerStructure'
import { useServersUnread } from './hooks/useServersUnread'
import { useAppUpdate } from './hooks/useAppUpdate'
import { useAccountsBySubject } from './hooks/useAccountsBySubject'
import { useIdle } from './hooks/useIdle'
import { useKnownServers } from './hooks/useKnownServers'
import { useAndroidPush } from './hooks/useAndroidPush'
import { useFriends, type FriendEvent } from './hooks/useFriends'
import { useE2EKeys } from './hooks/useE2EKeys'
import { E2EKeyPanel } from './components/E2EKeyPanel'
import { NotMemberPanel } from './components/NotMemberPanel'
import { useMe } from './hooks/useMe'
import { useMediaQuery } from './hooks/useMediaQuery'
import { useMyProfile } from './hooks/useMyProfile'
import { useTranslation } from 'react-i18next'
import i18n from './i18n'
import { errorMessage } from './lib/apiError'
import { useServerMembers } from './hooks/useServerMembers'
import { useUnread } from './hooks/useUnread'
import { useVoiceParticipants } from './hooks/useVoiceParticipants'
import { NotificationsContext, useNotificationCenter } from './hooks/useNotificationCenter'
import { NotificationStack } from './components/NotificationStack'
import { VoiceSessionProvider } from './components/VoiceSessionProvider'
import { VoiceConnectionBar } from './components/VoiceConnectionBar'
import type { VoiceTarget } from './hooks/useVoiceChannel'
import {
  UNCATEGORIZED_ID,
  createCategory,
  createChannel,
  createServerInvite,
  deleteCategory,
  deleteChannel,
  deleteChannelOverwrite,
  fetchChannelOverwrites,
  joinServer,
  moveVoiceParticipant,
  setChannelOverwrite,
  updateCategory,
  updateChannel,
} from './lib/serverChannelApi'
import { friendRequestName, sendPresenceIdleFrame } from './lib/serverCentralApi'
import { markRead } from './lib/unread'
import type { ParsedInvite } from './lib/inviteLink'
import { onPendingInvite, takePendingInvite } from './lib/pendingInvite'
import { onPushOpen, setPushActive, unregisterPush, type PushOpen } from './lib/androidPush'
import { NO_STRUCTURE_PERMISSIONS, PERMISSIONS, hasPermission, structurePermissionsOf } from './lib/permissions'
import type { Category } from './types'
import './App.css'

function App() {
  const { status, user, accessToken, redirecting, signOut } = useAuth()
  const { t } = useTranslation()
  // `sub` do OIDC: chave de E2E e cursores de não lida em localStorage são
  // por conta, não por navegador (ver crypto/e2e.ts, lib/unread.ts).
  const accountSub = user?.profile.sub ?? ''
  const { updateReady, updateKind, applyUpdate, applyUpdateSilently, installPermission } = useAppUpdate()
  // Sem sessão não há chamada de voz para derrubar: aplica a versão nova
  // sozinha, menos no meio do redirecionamento ao Authentik (recarregar ali
  // cancelaria a saída ou o login). Logado, só pelo botão do ServerRail. O
  // APK do Android fica sempre para o botão (abre o instalador na tela).
  useEffect(() => {
    if (applyUpdateSilently && status === 'signed-out' && !redirecting) applyUpdateSilently()
  }, [applyUpdateSilently, status, redirecting])
  const {
    profile: myProfile,
    uploadAvatar,
    removeAvatar,
    setDisplayName: setMyDisplayName,
    setStatus: setMyStatus,
    setLanguage: setMyLanguage,
  } = useMyProfile(accessToken ?? '')
  // Mensagens na tela (components/NotificationStack.tsx); desligadas com o
  // status "Ocupado" escolhido pela própria pessoa.
  const { notifications, notify, dismiss: dismissNotification } = useNotificationCenter(myProfile?.status === 'busy')
  const onServerOrderSaveError = useCallback(
    () => notify(i18n.t('notifications.serverOrderSaveFailed'), 'error'),
    [notify],
  )
  const {
    servers,
    status: serversStatus,
    addServer,
    removeServer,
    saveOrder: saveServerOrder,
  } = useKnownServers(accessToken ?? '', onServerOrderSaveError)
  // Notificações push, só no app Android (ver hooks/useAndroidPush.ts e
  // docs/architecture.md, "Decisão: notificações push no app Android (fase
  // 6, client)"). Fora dele push.available é false e nada aparece.
  const push = useAndroidPush(accessToken ?? '', accountSub, servers)
  const serverIds = useMemo(() => servers.map((s) => s.id), [servers])
  const [selectedFriendId, setSelectedFriendId] = useState<string>()
  // Pedido de amizade recebido ou aceito chega pelo WebSocket de presença,
  // com a pessoa em qualquer tela: vira aviso no canto (ver
  // docs/architecture.md, "Decisão: pedido de amizade pela lista de membros").
  const onFriendEvent = useCallback(
    (event: FriendEvent) => {
      if (event.kind === 'removed') {
        // Desfeita pelo outro lado com a DM aberta: volta para a lista.
        setSelectedFriendId((prev) => (prev === event.accountId ? undefined : prev))
      } else if (event.kind === 'request-received') {
        notify(i18n.t('notifications.friendRequestReceived', { name: friendRequestName(event.request) }))
      } else {
        notify(i18n.t('notifications.friendRequestAccepted', { name: friendRequestName(event) }), 'success')
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
    removeFriend,
    socket: presenceSocket,
  } = useFriends(accessToken ?? '', onFriendEvent)
  const e2eKeys = useE2EKeys(accessToken ?? '', accountSub)
  const myE2EKeyPair = e2eKeys.keyPair
  const [selectedServerId, setSelectedServerId] = useState<string>()
  const [showFriends, setShowFriends] = useState(false)
  const [showAddServer, setShowAddServer] = useState(false)
  // Convite da rota /convite que abriu o "Adicionar servidor" já preenchido.
  const [addServerInvite, setAddServerInvite] = useState<ParsedInvite>()
  const [showInviteServer, setShowInviteServer] = useState(false)
  const [showManageRoles, setShowManageRoles] = useState(false)
  const [showAddFriend, setShowAddFriend] = useState(false)
  const [showEditNickname, setShowEditNickname] = useState(false)
  const [showEditDisplayName, setShowEditDisplayName] = useState(false)
  const [showMyAvatar, setShowMyAvatar] = useState(false)
  // Diálogos de estrutura: undefined = fechado; id ausente = criar.
  const [categoryDialog, setCategoryDialog] = useState<{ id?: string }>()
  const [channelDialog, setChannelDialog] = useState<{ id?: string; categoryId?: string }>()
  const [permissionsChannelId, setPermissionsChannelId] = useState<string>()
  // Gaveta aberta no layout móvel (docs/architecture.md, "Decisão: layout
  // móvel com gavetas"); no computador é ignorada.
  const isMobile = useMediaQuery(MOBILE_QUERY)
  const [mobileDrawer, setMobileDrawer] = useState<'nav' | 'members'>()
  const swipeStart = useRef<{ x: number; y: number }>(undefined)

  // "Voltar" do Android fecha a gaveta em vez de sair da página: abrir
  // empilha um estado no histórico, e o popstate dele fecha a gaveta. Fechar
  // pela interface (toque, gesto, escolher um canal) desempilha esse estado,
  // para o próximo "voltar" não parar numa entrada vazia. A gaveta forçada
  // (nada aberto no painel) não entra: não há o que fechar.
  const drawerOpen = isMobile && !!mobileDrawer
  const drawerInHistory = useRef(false)
  useEffect(() => {
    if (drawerOpen) {
      if (!drawerInHistory.current) {
        window.history.pushState({ ffcomDrawer: true }, '')
        drawerInHistory.current = true
      }
      const onPopState = () => {
        drawerInHistory.current = false
        setMobileDrawer(undefined)
      }
      window.addEventListener('popstate', onPopState)
      return () => window.removeEventListener('popstate', onPopState)
    }
    if (drawerInHistory.current) {
      drawerInHistory.current = false
      window.history.back()
    }
  }, [drawerOpen])

  // Convite aberto pela rota /convite (lib/pendingInvite.ts): guardado até
  // haver sessão (inclusive através do login) e mostrado no "Adicionar
  // servidor" já preenchido. Chegando outro com a página aberta (App Link no
  // Android), abre na hora. Ver docs/architecture.md, "Decisão: convites
  // pelo domínio do app (fase 4)".
  useEffect(() => {
    if (status !== 'signed-in') return
    const openPendingInvite = () => {
      const invite = takePendingInvite()
      if (!invite) return
      setAddServerInvite(invite)
      setShowAddServer(true)
    }
    openPendingInvite()
    return onPendingInvite(openPendingInvite)
  }, [status])

  const selectedFriend = friends.find((f) => f.accountId === selectedFriendId)

  // Ações do botão direito na lista de membros. Erros (pedido duplicado,
  // conta sumiu) voltam do servidor como texto e viram aviso.
  const memberFriendActions = useMemo<MemberFriendActions>(() => {
    const friendIds = new Set(friends.map((f) => f.accountId))
    const incomingByAccount = new Map(incomingRequests.map((r) => [r.accountId, r]))
    const outgoingIds = new Set(outgoingRequests.map((r) => r.accountId))
    const fail = (err: unknown) =>
      notify(errorMessage(err, i18n.t('notifications.friendshipUpdateFailed')), 'error')
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
                ? i18n.t('notifications.nowFriends', { name: nickname })
                : i18n.t('notifications.friendRequestSent', { name: nickname }),
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
  // Nome gravado em cada servidor como nome exibido de quem não escolheu
  // apelido (ver hooks/useMe.ts): o nome de exibição da conta, ou o do
  // perfil do Authentik quando não há um escolhido. Espera o perfil de
  // server-central carregar, para não gravar o do Authentik e trocar logo
  // depois. A lista de membros recarrega quando ele é gravado;
  // useServerMembers vem depois porque depende das permissões de me, daí o
  // ref.
  const authentikName = (user?.profile.name || user?.profile.preferred_username)?.trim() || undefined
  const profileName = myProfile ? (myProfile.customDisplayName ?? authentikName) : undefined
  const refreshMembersRef = useRef<() => Promise<void>>(undefined)
  const onProfileNameSaved = useCallback(() => void refreshMembersRef.current?.(), [])
  const {
    me,
    notMember,
    reload: reloadMe,
    setNickname,
  } = useMe(server?.baseUrl ?? '', accessToken ?? '', profileName, onProfileNameSaved)
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
    updateRole,
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
    () => notify(i18n.t('notifications.structureOrderSaveFailed'), 'error'),
    [notify],
  )
  const {
    categories,
    refresh: refreshStructure,
    saveCategoryOrder,
    saveChannelOrder,
    // 403 num poll: pode ter sido expulso com o servidor aberto, e só
    // /api/me diz com certeza (ver NotMemberPanel).
  } = useServerStructure(server?.baseUrl ?? '', accessToken ?? '', onStructureSaveError, reloadMe)
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
  // Pela permissão base: um overwrite de canal pode mudar isso sala a sala,
  // e o servidor confere nas duas salas ao mover.
  const canMoveMembers = me ? hasPermission(me.permissions, PERMISSIONS.MoveMembers) || !!me.isOwner : false
  const moveToVoiceChannel = useCallback(
    (memberId: string, channelId: string) => {
      if (!serverBaseUrl || !accessToken) return
      moveVoiceParticipant(serverBaseUrl, accessToken, memberId, channelId).catch((err) =>
        notify(errorMessage(err, i18n.t('notifications.moveMemberFailed')), 'error'),
      )
    },
    [serverBaseUrl, accessToken, notify],
  )

  const [selectedChannelId, setSelectedChannelId] = useState<string>()
  // Canal a abrir quando a estrutura do servidor escolhido chegar: a barra
  // "Conectado em" leva ao canal da chamada mesmo em outro servidor, e até lá
  // categories ainda é a do servidor anterior.
  const pendingChannelIdRef = useRef<string>(undefined)
  // categories muda a cada repoll de useServerStructure (20s), não só ao
  // trocar de servidor: manter o canal selecionado se ele ainda existe, e só
  // cair no primeiro canal quando ele sumiu (servidor novo, canal apagado).
  // Resetar sempre tirava a pessoa do canal a cada poll.
  useEffect(() => {
    const pending = pendingChannelIdRef.current
    if (pending && categories.some((category) => category.channels.some((c) => c.id === pending))) {
      pendingChannelIdRef.current = undefined
      setSelectedChannelId(pending)
      return
    }
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

  const notMemberPanel = server && (
    <NotMemberPanel
      serverName={server.name}
      onJoin={async (inviteCode) => {
        await joinServer(server.baseUrl, accessToken ?? '', inviteCode)
        reloadMe()
        refreshStructure()
        void refreshMembersRef.current?.()
      }}
      onRemove={() => removeServer(server.id)}
    />
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

  // O que está na tela, para o push não avisar dele com o app na frente e
  // tirar a notificação de quem acabou de abrir.
  const activeServerAddress = server?.baseUrl
  useEffect(() => {
    if (status !== 'signed-in') setPushActive(undefined)
    else if (showFriends) setPushActive(selectedFriendId ? { dmAccountId: selectedFriendId } : undefined)
    else if (activeServerAddress && selectedChannelId)
      setPushActive({ serverAddress: activeServerAddress, channelId: selectedChannelId })
    else setPushActive(undefined)
  }, [status, showFriends, selectedFriendId, activeServerAddress, selectedChannelId])

  // Toque numa notificação: abre o canal, a DM ou a tela de amigos. Na
  // abertura a frio a lista de servidores ainda está chegando, então o
  // pedido espera por ela.
  const [pushOpen, setPushOpen] = useState<PushOpen>()
  useEffect(() => {
    if (status !== 'signed-in') return
    return onPushOpen(setPushOpen)
  }, [status])
  useEffect(() => {
    if (!pushOpen) return
    if (pushOpen.type === 'channel_message') {
      const target = servers.find((s) => s.baseUrl === pushOpen.serverAddress)
      if (!target) {
        // Lista carregada e o servidor não está nela (saiu em outro
        // aparelho): ignora.
        if (serversStatus !== 'loading') setPushOpen(undefined)
        return
      }
      setShowFriends(false)
      if (target.id === selectedServerId) {
        setSelectedChannelId(pushOpen.channelId)
      } else {
        pendingChannelIdRef.current = pushOpen.channelId
        setSelectedServerId(target.id)
      }
    } else {
      setShowFriends(true)
      setSelectedFriendId(pushOpen.type === 'dm' ? pushOpen.accountId : undefined)
    }
    setMobileDrawer(undefined)
    setPushOpen(undefined)
  }, [pushOpen, servers, serversStatus, selectedServerId])

  const togglePushMute = (serverAddress: string, channelId?: string) => {
    const muted = push.mutes.isMuted(serverAddress, channelId)
    push.mutes
      .setMuted(serverAddress, channelId, !muted)
      .catch((err: unknown) => notify(errorMessage(err, i18n.t('push.mute.saveFailed')), 'error'))
  }

  if (status === 'loading') {
    return null
  }

  // Sem nada para mostrar no painel principal, a gaveta de servidores e
  // canais fica aberta no celular: é por ela que se escolhe o que abrir.
  const navForced = showFriends ? !!myE2EKeyPair && !selectedFriend : !server || !channel
  const drawer = !isMobile ? undefined : navForced ? 'nav' : mobileDrawer
  const closeDrawer = () => setMobileDrawer(undefined)

  const openVoiceChannel = (target: VoiceTarget) => {
    setShowFriends(false)
    if (target.serverId === selectedServerId) {
      setSelectedChannelId(target.channelId)
    } else {
      pendingChannelIdRef.current = target.channelId
      setSelectedServerId(target.serverId)
    }
    closeDrawer()
  }

  // Arrastar na horizontal abre e fecha as gavetas. Começa em qualquer ponto
  // (as bordas da tela são o gesto de voltar do Android) e ignora diálogos e
  // campos de texto, onde arrastar tem outro sentido (recorte do avatar,
  // seleção de texto).
  const onTouchStart = (e: ReactTouchEvent) => {
    const target = e.target as Element
    swipeStart.current =
      !isMobile || e.touches.length > 1 || target.closest('.dialog-overlay, input, textarea, video, [data-no-swipe]')
        ? undefined
        : { x: e.touches[0].clientX, y: e.touches[0].clientY }
  }
  const onTouchEnd = (e: ReactTouchEvent) => {
    const start = swipeStart.current
    swipeStart.current = undefined
    if (!start) return
    const dx = e.changedTouches[0].clientX - start.x
    const dy = e.changedTouches[0].clientY - start.y
    if (Math.abs(dx) < 60 || Math.abs(dx) < 1.5 * Math.abs(dy)) return
    if (drawer === 'nav') {
      if (dx < 0 && !navForced) closeDrawer()
    } else if (drawer === 'members') {
      if (dx > 0) closeDrawer()
    } else if (dx > 0) {
      setMobileDrawer('nav')
    } else if (!showFriends && server) {
      setMobileDrawer('members')
    }
  }

  if (status === 'signed-out') {
    return <LoginScreen />
  }

  return (
    <VoiceSessionProvider serverIds={serverIds}>
      <PresenceContext.Provider value={presence}>
        <NotificationsContext.Provider value={notify}>
          <MobileNavContext.Provider
            value={{
              isMobile,
              openNav: () => setMobileDrawer('nav'),
              openMembers: !showFriends && server ? () => setMobileDrawer('members') : undefined,
            }}
          >
            <div
              className={isMobile ? 'app-shell mobile' : 'app-shell'}
              data-drawer={drawer}
              onTouchStart={onTouchStart}
              onTouchEnd={onTouchEnd}
            >
              <div className="nav-drawer">
                <ServerRail
                  servers={servers}
                  selectedServerId={selectedServerId}
                  friendsSelected={showFriends}
                  unreadServerIds={unreadServerIds}
                  friendsUnread={!showFriends && (unreadFriendIds.size > 0 || incomingRequests.length > 0)}
                  updateReady={updateReady}
                  updateKind={updateKind}
                  onUpdate={applyUpdate}
                  myProfile={myProfile}
                  account={{
                    profileName: authentikName,
                    username: user?.profile.preferred_username,
                    email: user?.profile.email,
                    nickname: showFriends ? undefined : me?.nickname,
                    serverName: server?.name,
                  }}
                  myStatus={ownStatus}
                  onSetStatus={(next) => {
                    setMyStatus(next).catch(() => {
                      /* setStatus já desfez a troca otimista */
                    })
                  }}
                  onSetLanguage={(next) => {
                    setMyLanguage(next).catch(() => notify(i18n.t('settings.languageSaveFailed'), 'error'))
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
                    notifications:
                      push.available && server
                        ? { muted: push.mutes.isMuted(server.baseUrl), onToggle: () => togglePushMute(server.baseUrl) }
                        : undefined,
                  }}
                  onReorderServers={saveServerOrder}
                  onSelectFriends={() => setShowFriends(true)}
                  onAddServer={() => setShowAddServer(true)}
                  onOpenMyAvatar={() => setShowMyAvatar(true)}
                  onEditDisplayName={myProfile ? () => setShowEditDisplayName(true) : undefined}
                  onEditNickname={!showFriends && server && me ? () => setShowEditNickname(true) : undefined}
                  onSignOut={() => {
                    // No app Android, tira antes o aparelho e os grants da
                    // conta (melhor esforço, com prazo); no resto é só sair.
                    void unregisterPush(accessToken ?? '', accountSub).finally(signOut)
                  }}
                />
                {/* A barra da chamada fica no pé da coluna, com amigos ou
                    canais na frente. */}
                <div className="sidebar-column">
                  {showFriends ? (
                    <FriendsView
                      friends={friends}
                      selectedFriendId={selectedFriendId}
                      unreadFriendIds={unreadFriendIds}
                      onSelectFriend={(id) => {
                        setSelectedFriendId(id)
                        closeDrawer()
                      }}
                      onAddFriend={() => setShowAddFriend(true)}
                      incomingRequests={incomingRequests}
                      outgoingRequests={outgoingRequests}
                      onAcceptRequest={(id) => {
                        acceptFriendRequest(id).catch((err: unknown) =>
                          notify(errorMessage(err, t('notifications.acceptRequestFailed')), 'error'),
                        )
                      }}
                      onRemoveRequest={(id) => {
                        removeFriendRequest(id).catch((err: unknown) =>
                          notify(errorMessage(err, t('notifications.removeRequestFailed')), 'error'),
                        )
                      }}
                      onRemoveFriend={async (id) => {
                        await removeFriend(id)
                        setSelectedFriendId((prev) => (prev === id ? undefined : prev))
                      }}
                    />
                  ) : server ? (
                    <ChannelSidebar
                      server={server}
                      // Fora do servidor, a estrutura do último poll bem-sucedido
                      // não vale mais.
                      categories={notMember ? [] : categories}
                      selectedChannelId={selectedChannelId}
                      unreadChannelIds={unreadChannelIds}
                      voiceParticipants={voiceParticipants}
                      canMoveMembers={canMoveMembers}
                      onMoveVoiceParticipant={moveToVoiceChannel}
                      members={members}
                      selfMemberId={me?.memberId}
                      onSelectChannel={(id) => {
                        setSelectedChannelId(id)
                        closeDrawer()
                      }}
                      structure={structurePermissions}
                      canManageRoles={canManageRoles}
                      onEditChannelPermissions={setPermissionsChannelId}
                      onCreateCategory={() => setCategoryDialog({})}
                      onEditCategory={(id) => setCategoryDialog({ id })}
                      onCreateChannel={(categoryId) => setChannelDialog({ categoryId })}
                      onEditChannel={(id) => setChannelDialog({ id })}
                      onReorderCategories={saveCategoryOrder}
                      onReorderChannels={saveChannelOrder}
                      channelNotifications={
                        push.available
                          ? {
                              isMuted: (channelId) => push.mutes.isMuted(server.baseUrl, channelId),
                              onToggle: (channelId) => togglePushMute(server.baseUrl, channelId),
                            }
                          : undefined
                      }
                    />
                  ) : null}
                  <VoiceConnectionBar onOpen={openVoiceChannel} />
                </div>
              </div>
              {drawer && !navForced && <div className="drawer-backdrop" onClick={closeDrawer} />}
              {showFriends ? (
                !myE2EKeyPair ? (
                  isMobile ? (
                    // No celular a tela da chave precisa de um cabeçalho com o ☰
                    // para voltar à lista de amigos e aos servidores.
                    <section className="main-panel">
                      <header className="channel-header">
                        <MobileNavButton />
                        <span className="channel-title">{t('dm.title')}</span>
                      </header>
                      <E2EKeyPanel e2e={e2eKeys} />
                    </section>
                  ) : (
                    <E2EKeyPanel e2e={e2eKeys} />
                  )
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
                    <p>{t('friends.selectPrompt')}</p>
                  </div>
                )
              ) : server && notMember ? (
                isMobile ? (
                  // Mesmo caso da chave de E2E: no celular o painel precisa do ☰
                  // para voltar aos servidores.
                  <section className="main-panel">
                    <header className="channel-header">
                      <MobileNavButton />
                      <span className="channel-title">{server.name}</span>
                    </header>
                    {notMemberPanel}
                  </section>
                ) : (
                  notMemberPanel
                )
              ) : server ? (
                <>
                  <MainPanel
                    channel={channel}
                    server={server}
                    members={members}
                    canModerateMessages={canModerateMessages}
                  />
                  <MemberList members={members} roles={roles} friendActions={memberFriendActions} />
                </>
              ) : (
                <div className="empty-state">
                  <p>{t('server.noneYet')}</p>
                </div>
              )}
              {showAddServer && (
                <AddServerDialog
                  key={addServerInvite ? `${addServerInvite.address} ${addServerInvite.inviteCode}` : 'manual'}
                  onAdd={addServer}
                  initialInvite={addServerInvite}
                  onClose={() => {
                    setShowAddServer(false)
                    setAddServerInvite(undefined)
                  }}
                />
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
                  onUpdateRole={updateRole}
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
              {showEditDisplayName && (
                <DisplayNameDialog
                  currentName={myProfile?.customDisplayName}
                  authentikName={authentikName}
                  onSave={setMyDisplayName}
                  onClose={() => setShowEditDisplayName(false)}
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
              {push.askPermission && (
                <PushPermissionDialog onAllow={push.acceptPermission} onDecline={push.declinePermission} />
              )}
              {installPermission && (
                <InstallPermissionDialog onAllow={installPermission.allow} onClose={installPermission.dismiss} />
              )}
            </div>
          </MobileNavContext.Provider>
          <NotificationStack notifications={notifications} onDismiss={dismissNotification} />
        </NotificationsContext.Provider>
      </PresenceContext.Provider>
    </VoiceSessionProvider>
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
