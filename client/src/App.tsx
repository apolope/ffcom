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
import { useAuth } from './auth/AuthProvider'
import { useServerStructure } from './hooks/useServerStructure'
import { useKnownServers } from './hooks/useKnownServers'
import { useFriends } from './hooks/useFriends'
import { useE2EKeys } from './hooks/useE2EKeys'
import { useMe } from './hooks/useMe'
import { useMyProfile } from './hooks/useMyProfile'
import { useServerMembers } from './hooks/useServerMembers'
import { useUnread } from './hooks/useUnread'
import { createServerInvite } from './lib/serverChannelApi'
import { markRead } from './lib/unread'
import { PERMISSIONS, hasPermission } from './lib/permissions'
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

  const { categories } = useServerStructure(server?.baseUrl ?? '', accessToken ?? '')

  const [selectedChannelId, setSelectedChannelId] = useState<string>()
  useEffect(() => {
    setSelectedChannelId(categories[0]?.channels[0]?.id)
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

export default App
