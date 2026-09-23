import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Room,
  RoomEvent,
  Track,
  type LocalParticipant,
  type Participant,
  type RemoteParticipant,
  type TrackPublication,
} from 'livekit-client'
import { fetchVoiceToken } from '../lib/serverChannelApi'

export type VoiceChannelStatus = 'idle' | 'connecting' | 'connected' | 'error'

export interface VoiceParticipant {
  identity: string
  name: string
  isLocal: boolean
  micEnabled: boolean
  cameraEnabled: boolean
  screenSharing: boolean
}

interface UseVoiceChannelResult {
  status: VoiceChannelStatus
  error: string | undefined
  participants: VoiceParticipant[]
  micEnabled: boolean
  cameraEnabled: boolean
  cameraError: string | undefined
  screenSharing: boolean
  videoContainerRef: (node: HTMLDivElement | null) => void
  join: () => void
  leave: () => void
  toggleMic: () => void
  toggleCamera: () => void
  toggleScreenShare: () => void
}

function toParticipant(p: LocalParticipant | RemoteParticipant): VoiceParticipant {
  return {
    identity: p.identity,
    name: p.name || p.identity,
    isLocal: p.isLocal,
    micEnabled: p.isMicrophoneEnabled,
    cameraEnabled: p.isCameraEnabled,
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
  const videoContainerElRef = useRef<HTMLDivElement | null>(null)
  const videoTilesRef = useRef<Map<string, HTMLDivElement>>(new Map())
  const [status, setStatus] = useState<VoiceChannelStatus>('idle')
  const [error, setError] = useState<string>()
  const [participants, setParticipants] = useState<VoiceParticipant[]>([])
  const [micEnabled, setMicEnabled] = useState(false)
  const [cameraError, setCameraError] = useState<string>()

  const cleanupAudioEls = useCallback(() => {
    audioElsRef.current.forEach((el) => el.remove())
    audioElsRef.current.clear()
  }, [])

  const cleanupVideoTiles = useCallback(() => {
    videoTilesRef.current.forEach((tile) => tile.remove())
    videoTilesRef.current.clear()
  }, [])

  // Callback ref: a UI monta/desmonta a <div> de destino ao longo do ciclo de
  // vida do componente, mas as tiles de vídeo (câmera e tela) são inseridas
  // de forma imperativa (mesmo padrão já usado para os elementos de áudio),
  // então guardamos o nó atual para anexar/remover tiles depois.
  const videoContainerRef = useCallback((node: HTMLDivElement | null) => {
    videoContainerElRef.current = node
    if (node) {
      videoTilesRef.current.forEach((tile) => node.appendChild(tile))
    }
  }, [])

  // Uma tile por track de vídeo (câmera ou tela), chaveada pelo trackSid.
  // A câmera local também ganha tile (espelhada via classe "local") para a
  // pessoa se ver; a tela local continua sem preview, ver
  // docs/architecture.md, "Decisão: câmera no canal de voz".
  const addVideoTile = useCallback((publication: TrackPublication, participant: Participant) => {
    const track = publication.track
    if (!track || videoTilesRef.current.has(publication.trackSid)) return
    const video = track.attach() as HTMLVideoElement
    video.autoplay = true
    video.playsInline = true
    video.muted = true
    const name = participant.name || participant.identity
    const label = document.createElement('span')
    label.className = 'video-tile-label'
    if (participant.isLocal) {
      label.textContent = `${name} (você)`
    } else if (publication.source === Track.Source.ScreenShare) {
      label.textContent = `${name} (tela)`
    } else {
      label.textContent = name
    }
    const tile = document.createElement('div')
    tile.className = participant.isLocal ? 'video-tile local' : 'video-tile'
    if (publication.isMuted) tile.classList.add('muted')
    tile.appendChild(video)
    tile.appendChild(label)
    videoTilesRef.current.set(publication.trackSid, tile)
    videoContainerElRef.current?.appendChild(tile)
  }, [])

  const removeVideoTile = useCallback((trackSid: string) => {
    videoTilesRef.current.get(trackSid)?.remove()
    videoTilesRef.current.delete(trackSid)
  }, [])

  const refreshParticipants = useCallback((room: Room) => {
    const all = [room.localParticipant, ...Array.from(room.remoteParticipants.values())]
    setParticipants(all.map(toParticipant))
  }, [])

  const disconnect = useCallback(
    (room: Room | undefined) => {
      room?.disconnect()
      cleanupAudioEls()
      cleanupVideoTiles()
      roomRef.current = undefined
      setStatus('idle')
      setParticipants([])
      setMicEnabled(false)
      setCameraError(undefined)
    },
    [cleanupAudioEls, cleanupVideoTiles],
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
      room.on(RoomEvent.LocalTrackPublished, (publication, participant) => {
        if (publication.source === Track.Source.Camera) {
          addVideoTile(publication, participant)
        }
        refreshParticipants(room)
      })
      room.on(RoomEvent.LocalTrackUnpublished, (publication) => {
        removeVideoTile(publication.trackSid)
        refreshParticipants(room)
      })
      room.on(RoomEvent.TrackSubscribed, (track, publication, participant) => {
        if (track.kind === Track.Kind.Audio) {
          const el = track.attach()
          document.body.appendChild(el)
          audioElsRef.current.add(el)
        } else if (track.kind === Track.Kind.Video) {
          addVideoTile(publication, participant)
        }
        refreshParticipants(room)
      })
      room.on(RoomEvent.TrackUnsubscribed, (track, publication) => {
        removeVideoTile(publication.trackSid)
        track.detach().forEach((el) => {
          audioElsRef.current.delete(el)
          el.remove()
        })
        refreshParticipants(room)
      })
      // setCameraEnabled(false) não despublica a track, só a silencia (mute),
      // e religar só tira o mute: a tile é escondida/mostrada aqui em vez de
      // removida, senão ficaria congelada no último quadro. Dispara tanto
      // para a câmera local quanto para as remotas.
      room.on(RoomEvent.TrackMuted, (publication) => {
        videoTilesRef.current.get(publication.trackSid)?.classList.add('muted')
        refreshParticipants(room)
      })
      room.on(RoomEvent.TrackUnmuted, (publication) => {
        videoTilesRef.current.get(publication.trackSid)?.classList.remove('muted')
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
  }, [baseUrl, channelId, accessToken, refreshParticipants, disconnect, addVideoTile, removeVideoTile])

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

  // Diferente do seletor de tela (ver toggleScreenShare), recusar a permissão
  // da câmera ou não ter câmera é uma falha que a pessoa precisa ver: vira
  // cameraError, sem derrubar o canal de voz (status continua 'connected').
  const toggleCamera = useCallback(async () => {
    const room = roomRef.current
    if (!room) return
    const next = !room.localParticipant.isCameraEnabled
    setCameraError(undefined)
    try {
      await room.localParticipant.setCameraEnabled(next)
    } catch (err) {
      setCameraError(cameraErrorMessage(err))
    }
    refreshParticipants(room)
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
  const cameraEnabled = participants.some((p) => p.isLocal && p.cameraEnabled)

  return {
    status,
    error,
    participants,
    micEnabled,
    cameraEnabled,
    cameraError,
    screenSharing,
    videoContainerRef,
    join,
    leave,
    toggleMic,
    toggleCamera,
    toggleScreenShare,
  }
}

function cameraErrorMessage(err: unknown): string {
  const name = err instanceof Error ? err.name : ''
  if (name === 'NotAllowedError') return 'Permissão da câmera negada pelo navegador.'
  if (name === 'NotFoundError' || name === 'OverconstrainedError') return 'Nenhuma câmera encontrada.'
  if (name === 'NotReadableError') return 'A câmera está em uso por outro aplicativo.'
  return err instanceof Error ? err.message : 'falha ao ligar a câmera'
}
