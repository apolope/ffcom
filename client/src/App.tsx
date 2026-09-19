import { useMemo, useState } from 'react'
import { ServerRail } from './components/ServerRail'
import { ChannelSidebar } from './components/ChannelSidebar'
import { MainPanel } from './components/MainPanel'
import { MemberList } from './components/MemberList'
import { mockServerDetails, mockServers } from './data/mockData'
import './App.css'

function App() {
  const [selectedServerId, setSelectedServerId] = useState(mockServers[0].id)

  const server = mockServers.find((s) => s.id === selectedServerId)!
  const detail = mockServerDetails[selectedServerId]

  const firstChannelId = detail.categories[0]?.channels[0]?.id
  const [selectedChannelId, setSelectedChannelId] = useState(firstChannelId)

  const channel = useMemo(
    () =>
      detail.categories
        .flatMap((category) => category.channels)
        .find((c) => c.id === selectedChannelId),
    [detail, selectedChannelId],
  )

  function handleSelectServer(serverId: string) {
    setSelectedServerId(serverId)
    setSelectedChannelId(mockServerDetails[serverId].categories[0]?.channels[0]?.id)
  }

  return (
    <div className="app-shell">
      <ServerRail
        servers={mockServers}
        selectedServerId={selectedServerId}
        onSelectServer={handleSelectServer}
      />
      <ChannelSidebar
        server={server}
        categories={detail.categories}
        selectedChannelId={selectedChannelId}
        onSelectChannel={setSelectedChannelId}
      />
      <MainPanel channel={channel} />
      <MemberList members={detail.members} />
    </div>
  )
}

export default App
