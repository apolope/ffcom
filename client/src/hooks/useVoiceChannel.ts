import { useCallback, useEffect, useRef, useState } from 'react'
import { Room, RoomEvent, Track, type LocalParticipant, type RemoteParticipant } from 'livekit-client'
import { fetchVoiceToken } from '../lib/serverChannelApi'

export type VoiceChannelStatus = 'idle' | 'connecting' | 'connected' | 'error'

export interface VoiceParticipant {
  identity: string
  name: string
  isLocal: boolean
  micEnabled: boolean
  screenSharing: boolean
}

interface UseVoiceChannelResult {
  status: VoiceChannelStatus
  error: string | undefined
  participants: VoiceParticipant[]
  micEnabled: boolean
  screenSharing: boolean
  screenShareContainerRef: (node: HTMLDivElement | null) => void
  join: () => void
  leave: () => void
  toggleMic: () => void
  toggleScreenShare: () => void
}

function toParticipant(p: LocalParticipant | RemoteParticipant): VoiceParticipant {
  return {
    identity: p.identity,
    name: p.name || p.identity,
    isLocal: p.isLocal,
    micEnabled: p.isMicrophoneEnabled,
    screenSharing: p.isScreenShareEnabled,
  }
}

// Conecta a um canal de voz via LiveKit (ver docs/architecture.md, "Decisão:
// integração de voz com LiveKit"). Cada canal tem sua própria sala LiveKit
// (nome = id do canal); entrar busca um token novo em
// POST /api/channels/{id}/voice/token a cada tentativa, em vez de cachear.
export function useVoiceChannel(
  baseUrl: string,
  channelId: string,
  accessToken: string,
): UseVoiceChannelResult {
  const roomRef = useRef<Room | undefined>(undefined)
  const audioElsRef = useRef<Set<HTMLMediaElement>>(new Set())
  const screenShareContainerElRef = useRef<HTMLDivElement | null>(null)
  const screenShareTilesRef = useRef<Map<string, HTMLDivElement>>(new Map())
  const [status, setStatus] = useState<VoiceChannelStatus>('idle')
  const [error, setError] = useState<string>()
  const [participants, setParticipants] = useState<VoiceParticipant[]>([])
  const [micEnabled, setMicEnabled] = useState(false)

  const cleanupAudioEls = useCallback(() => {
    audioElsRef.current.forEach((el) => el.remove())
    audioElsRef.current.clear()
  }, [])

  const cleanupScreenShareTiles = useCallback(() => {
    screenShareTilesRef.current.forEach((tile) => tile.remove())
    screenShareTilesRef.current.clear()
  }, [])

  // Callback ref: a UI monta/desmonta a <div> de destino ao longo do ciclo de
  // vida do componente, mas as tiles de compartilhamento de tela são
  // inseridas de forma imperativa (mesmo padrão já usado para os elementos de
  // áudio), então guardamos o nó atual para anexar/remover tiles depois.
  const screenShareContainerRef = useCallback((node: HTMLDivElement | null) => {
    screenShareContainerElRef.current = node
    if (node) {
      screenShareTilesRef.current.forEach((tile) => node.appendChild(tile))
    }
  }, [])

  const refreshParticipants = useCallback((room: Room) => {
    const all = [room.localParticipant, ...Array.from(room.remoteParticipants.values())]
    setParticipants(all.map(toParticipant))
  }, [])

  const disconnect = useCallback(
    (room: Room | undefined) => {
      room?.disconnect()
      cleanupAudioEls()
      cleanupScreenShareTiles()
      roomRef.current = undefined
      setStatus('idle')
      setParticipants([])
      setMicEnabled(false)
    },
    [cleanupAudioEls, cleanupScreenShareTiles],
  )

  // Sair do canal de voz ao trocar de canal ou desmontar o componente —
  // nunca deixar uma sala LiveKit conectada em segundo plano sem UI.
  useEffect(() => {
    return () => disconnect(roomRef.current)
  }, [channelId, disconnect])

  const join = useCallback(async () => {
    if (roomRef.current) return
    setStatus('connecting')
    setError(undefined)
    try {
      const { token, url } = await fetchVoiceToken(baseUrl, channelId, accessToken)
      const room = new Room()
      roomRef.current = room

      room.on(RoomEvent.ParticipantConnected, () => refreshParticipants(room))
      room.on(RoomEvent.ParticipantDisconnected, () => refreshParticipants(room))
      room.on(RoomEvent.LocalTrackPublished, () => refreshParticipants(room))
      room.on(RoomEvent.LocalTrackUnpublished, () => refreshParticipants(room))
      room.on(RoomEvent.TrackSubscribed, (track, publication, participant) => {
        if (track.kind === Track.Kind.Audio) {
          const el = track.attach()
          document.body.appendChild(el)
          audioElsRef.current.add(el)
        } else if (track.source === Track.Source.ScreenShare) {
          const video = track.attach() as HTMLVideoElement
          video.autoplay = true
          video.playsInline = true
          const label = document.createElement('span')
          label.className = 'screen-share-label'
          label.textContent = participant.name || participant.identity
          const tile = document.createElement('div')
          tile.className = 'screen-share-tile'
          tile.appendChild(video)
          tile.appendChild(label)
          screenShareTilesRef.current.set(publication.trackSid, tile)
          screenShareContainerElRef.current?.appendChild(tile)
        }
        refreshParticipants(room)
      })
      room.on(RoomEvent.TrackUnsubscribed, (track, publication) => {
        if (track.source === Track.Source.ScreenShare) {
          screenShareTilesRef.current.get(publication.trackSid)?.remove()
          screenShareTilesRef.current.delete(publication.trackSid)
        }
        track.detach().forEach((el) => {
          audioElsRef.current.delete(el)
          el.remove()
        })
        refreshParticipants(room)
      })
      room.on(RoomEvent.Disconnected, () => disconnect(room))

      await room.connect(url, token)
      await room.localParticipant.setMicrophoneEnabled(true)
      setMicEnabled(true)
      setStatus('connected')
      refreshParticipants(room)
    } catch (err) {
      disconnect(roomRef.current)
      setStatus('error')
      setError(err instanceof Error ? err.message : 'falha ao conectar à voz')
    }
  }, [baseUrl, channelId, accessToken, refreshParticipants, disconnect])

  const leave = useCallback(() => disconnect(roomRef.current), [disconnect])

  const toggleMic = useCallback(() => {
    const room = roomRef.current
    if (!room) return
    const next = !room.localParticipant.isMicrophoneEnabled
    room.localParticipant.setMicrophoneEnabled(next).then(() => {
      setMicEnabled(next)
      refreshParticipants(room)
    })
  }, [refreshParticipants])

  // setScreenShareEnabled(true) abre o seletor nativo do navegador
  // (getDisplayMedia); rejeitar essa promise ao cancelar o seletor não é um
  // erro real do canal de voz, só a desistência do usuário.
  const toggleScreenShare = useCallback(async () => {
    const room = roomRef.current
    if (!room) return
    const next = !room.localParticipant.isScreenShareEnabled
    try {
      await room.localParticipant.setScreenShareEnabled(next)
    } catch {
      return
    }
    refreshParticipants(room)
  }, [refreshParticipants])

  const screenSharing = participants.some((p) => p.isLocal && p.screenSharing)

  return {
    status,
    error,
    participants,
    micEnabled,
    screenSharing,
    screenShareContainerRef,
    join,
    leave,
    toggleMic,
    toggleScreenShare,
  }
}
