import { useEffect, useMemo, useState } from 'react'
import { ServerRail } from './components/ServerRail'
import { ChannelSidebar } from './components/ChannelSidebar'
import { MainPanel } from './components/MainPanel'
import { MemberList } from './components/MemberList'
import { LoginScreen } from './components/LoginScreen'
import { AddServerDialog } from './components/AddServerDialog'
import { FriendsView } from './components/FriendsView'
import { AddFriendDialog } from './components/AddFriendDialog'
import { useAuth } from './auth/AuthProvider'
import { useServerStructure } from './hooks/useServerStructure'
import { useKnownServers } from './hooks/useKnownServers'
import { useFriends } from './hooks/useFriends'
import type { Member } from './types'
import './App.css'

const NO_MEMBERS: Member[] = []

function App() {
  const { status, accessToken } = useAuth()
  const { servers, addServer } = useKnownServers(accessToken ?? '')
  const { friends, createInvite, redeemInvite } = useFriends(accessToken ?? '')
  const [selectedServerId, setSelectedServerId] = useState<string>()
  const [showFriends, setShowFriends] = useState(false)
  const [showAddServer, setShowAddServer] = useState(false)
  const [showAddFriend, setShowAddFriend] = useState(false)

  useEffect(() => {
    if (selectedServerId && servers.some((s) => s.id === selectedServerId)) return
    setSelectedServerId(servers[0]?.id)
  }, [servers, selectedServerId])

  const server = servers.find((s) => s.id === selectedServerId)
  // Lista de membros por servidor (server-channel) ainda não tem API real
  // (diferente da lista de amigos/presença, que já vem de server-central via
  // useFriends) — ver TODO.md, "Sistema de permissões/roles por servidor e
  // por canal".
  const members = NO_MEMBERS

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
        onSelectServer={(id) => {
          setShowFriends(false)
          setSelectedServerId(id)
        }}
        onSelectFriends={() => setShowFriends(true)}
        onAddServer={() => setShowAddServer(true)}
      />
      {showFriends ? (
        <FriendsView friends={friends} onAddFriend={() => setShowAddFriend(true)} />
      ) : server ? (
        <>
          <ChannelSidebar
            server={server}
            categories={categories}
            selectedChannelId={selectedChannelId}
            onSelectChannel={setSelectedChannelId}
          />
          <MainPanel channel={channel} serverBaseUrl={server.baseUrl} />
          <MemberList members={members} />
        </>
      ) : (
        <div className="empty-state">
          <p>Nenhum servidor ainda. Adicione um pelo botão "+" na barra lateral.</p>
        </div>
      )}
      {showAddServer && (
        <AddServerDialog onAdd={addServer} onClose={() => setShowAddServer(false)} />
      )}
      {showAddFriend && (
        <AddFriendDialog
          onCreateInvite={createInvite}
          onRedeemInvite={redeemInvite}
          onClose={() => setShowAddFriend(false)}
        />
      )}
    </div>
  )
}

export default App
