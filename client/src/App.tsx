import { useEffect, useMemo, useState } from 'react'
import { ServerRail } from './components/ServerRail'
import { ChannelSidebar } from './components/ChannelSidebar'
import { MainPanel } from './components/MainPanel'
import { MemberList } from './components/MemberList'
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
import { useAuth } from './auth/AuthProvider'
import { useServerStructure } from './hooks/useServerStructure'
import { useKnownServers } from './hooks/useKnownServers'
import { useFriends } from './hooks/useFriends'
import { useE2EKeys } from './hooks/useE2EKeys'
import { useMe } from './hooks/useMe'
import { useMyProfile } from './hooks/useMyProfile'
import { useServerMembers } from './hooks/useServerMembers'
import { useUnread } from './hooks/useUnread'
import {
  UNCATEGORIZED_ID,
  createCategory,
  createChannel,
  createServerInvite,
  deleteCategory,
  deleteChannel,
  updateCategory,
  updateChannel,
} from './lib/serverChannelApi'
import { markRead } from './lib/unread'
import { PERMISSIONS, hasPermission } from './lib/permissions'
import type { Category } from './types'
import './App.css'

function App() {
  const { status, accessToken } = useAuth()
  const { servers, addServer } = useKnownServers(accessToken ?? '')
  const { friends, createInvite, redeemInvite, socket: presenceSocket } = useFriends(accessToken ?? '')
  const { keyPair: myE2EKeyPair } = useE2EKeys(accessToken ?? '')
  const { profile: myProfile, uploadAvatar, removeAvatar } = useMyProfile(accessToken ?? '')
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

  const selectedFriend = friends.find((f) => f.accountId === selectedFriendId)

  useEffect(() => {
    if (selectedServerId && servers.some((s) => s.id === selectedServerId)) return
    setSelectedServerId(servers[0]?.id)
  }, [servers, selectedServerId])

  const server = servers.find((s) => s.id === selectedServerId)
  const { me, setNickname } = useMe(server?.baseUrl ?? '', accessToken ?? '')
  const canManageRoles = me ? hasPermission(me.permissions, PERMISSIONS.ManageRoles) || !!me.isOwner : false
  const canKick = me ? hasPermission(me.permissions, PERMISSIONS.KickMembers) || !!me.isOwner : false
  const canBan = me ? hasPermission(me.permissions, PERMISSIONS.BanMembers) || !!me.isOwner : false
  const canOpenMemberAdmin = canManageRoles || canKick || canBan
  const canManageChannels = me ? hasPermission(me.permissions, PERMISSIONS.ManageChannels) || !!me.isOwner : false
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

  const { categories, refresh: refreshStructure } = useServerStructure(server?.baseUrl ?? '', accessToken ?? '')
  const realCategories = useMemo(() => categories.filter((c) => c.id !== UNCATEGORIZED_ID), [categories])

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
    markRead('channel', selectedChannelId)
    return () => markRead('channel', selectedChannelId)
  }, [selectedChannelId])

  useEffect(() => {
    if (!selectedFriendId) return
    markRead('dm', selectedFriendId)
    return () => markRead('dm', selectedFriendId)
  }, [selectedFriendId])

  const unreadChannelIds = useUnread(
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
    'dm',
    useMemo(() => friends.map((f) => ({ id: f.accountId, lastMessageAt: f.lastMessageAt })), [friends]),
    showFriends ? selectedFriendId : undefined,
  )

  if (status === 'loading') {
    return null
  }

  if (status === 'signed-out') {
    return <LoginScreen />
  }

  return (
    <div className="app-shell">
      <ServerRail
        servers={servers}
        selectedServerId={selectedServerId}
        friendsSelected={showFriends}
        myProfile={myProfile}
        onSelectServer={(id) => {
          setShowFriends(false)
          setSelectedServerId(id)
        }}
        onSelectFriends={() => setShowFriends(true)}
        onAddServer={() => setShowAddServer(true)}
        onOpenMyAvatar={() => setShowMyAvatar(true)}
      />
      {showFriends ? (
        <>
          <FriendsView
            friends={friends}
            selectedFriendId={selectedFriendId}
            unreadFriendIds={unreadFriendIds}
            onSelectFriend={setSelectedFriendId}
            onAddFriend={() => setShowAddFriend(true)}
          />
          {selectedFriend ? (
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
            onSelectChannel={setSelectedChannelId}
            onInvite={() => setShowInviteServer(true)}
            canManageMembers={canOpenMemberAdmin}
            onManageRoles={() => setShowManageRoles(true)}
            onEditNickname={() => setShowEditNickname(true)}
            canManageChannels={canManageChannels}
            onCreateCategory={() => setCategoryDialog({})}
            onEditCategory={(id) => setCategoryDialog({ id })}
            onCreateChannel={(categoryId) => setChannelDialog({ categoryId })}
            onEditChannel={(id) => setChannelDialog({ id })}
          />
          <MainPanel channel={channel} serverBaseUrl={server.baseUrl} />
          <MemberList members={members} roles={roles} />
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
          onSave={async (name) => {
            if (categoryDialog.id) {
              await updateCategory(server.baseUrl, accessToken ?? '', categoryDialog.id, { name })
            } else {
              await createCategory(server.baseUrl, accessToken ?? '', name)
            }
            refreshStructure()
          }}
          onDelete={async () => {
            if (!categoryDialog.id) return
            await deleteCategory(server.baseUrl, accessToken ?? '', categoryDialog.id)
            refreshStructure()
          }}
          onClose={() => setCategoryDialog(undefined)}
        />
      )}
      {channelDialog && server && (
        <ChannelDialog
          channel={findChannelForDialog(categories, channelDialog.id)}
          initialCategoryId={channelDialog.categoryId}
          categories={realCategories}
          onSave={async ({ name, type, categoryId }) => {
            if (channelDialog.id) {
              await updateChannel(server.baseUrl, accessToken ?? '', channelDialog.id, {
                name,
                categoryId: categoryId ?? null,
              })
            } else {
              const created = await createChannel(server.baseUrl, accessToken ?? '', { name, type, categoryId })
              setSelectedChannelId(created.id)
            }
            refreshStructure()
          }}
          onDelete={async () => {
            if (!channelDialog.id) return
            await deleteChannel(server.baseUrl, accessToken ?? '', channelDialog.id)
            refreshStructure()
          }}
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
