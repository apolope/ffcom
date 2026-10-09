import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ConnectionState,
  Room,
  RoomEvent,
  Track,
  createLocalAudioTrack,
  type AudioCaptureOptions,
  type LocalAudioTrack,
  type LocalParticipant,
  type Participant,
  type RemoteAudioTrack,
  type RemoteParticipant,
  type TrackPublication,
} from 'livekit-client'
import i18n from '../i18n'
import { LocalizedError, useErrorText, type DisplayError } from '../lib/apiError'
import { playMicToggleSound, playPushToTalkSound, primeMicToggleSound } from '../lib/micToggleSound'
import { playPresenceSound } from '../lib/presenceSound'
import { participantAudioOf, type ParticipantAudioMap } from '../lib/participantAudio'
import { fetchVoiceToken } from '../lib/serverChannelApi'
import { setVoiceConnected } from '../lib/voiceActivity'
import { onCallNotificationAction, syncCallNotification } from '../lib/androidCallService'

// Atualiza os textos de uma tile de vídeo (montada fora do React) no idioma
// ativo; chamado ao criar a tile e de novo a cada troca de idioma.
const tileTextUpdaters = new WeakMap<HTMLElement, () => void>()

export type VoiceChannelStatus = 'idle' | 'connecting' | 'connected' | 'error'

export interface VoiceParticipant {
  identity: string
  name: string
  isLocal: boolean
  micEnabled: boolean
  cameraEnabled: boolean
  screenSharing: boolean
  screenShareAudio: boolean
  // Falando agora (detecção de voz do LiveKit; microfone fechado nunca fala).
  speaking: boolean
}

// O canal de voz da chamada, com o que a barra "Conectado em" precisa para
// mostrar e voltar a ele de qualquer tela.
export interface VoiceTarget {
  serverId: string
  serverName: string
  baseUrl: string
  channelId: string
  channelName: string
}

export interface UseVoiceChannelResult {
  // Canal da última tentativa de entrar (o status abaixo é dele), até sair.
  target: VoiceTarget | undefined
  status: VoiceChannelStatus
  error: string | undefined
  participants: VoiceParticipant[]
  micEnabled: boolean
  cameraEnabled: boolean
  cameraError: string | undefined
  // Microfone não abriu (sem dispositivo, permissão negada, em uso): a
  // pessoa continua na sala, só ouvindo, até conseguir abrir.
  micError: string | undefined
  screenSharing: boolean
  // Compartilhando tela com áudio (a pessoa marcou "Compartilhar áudio").
  screenShareAudio: boolean
  // O navegador bloqueou a reprodução do áudio da sala (autoplay, comum no
  // Chrome do Android): nada toca até a pessoa tocar em "Ativar som".
  audioPlaybackBlocked: boolean
  // A supressão reforçada foi pedida mas não pôde ser ligada (sem
  // AudioWorklet, WASM não baixou, áudio suspenso): o microfone voltou para a
  // supressão do navegador.
  noiseSuppressionError: string | undefined
  startAudio: () => void
  videoContainerRef: (node: HTMLDivElement | null) => void
  // Entra no canal; se já estiver em outro, sai dele antes.
  join: (target: VoiceTarget) => void
  leave: () => void
  toggleMic: () => void
  // Push-to-talk: abre (true) ou fecha (false) o microfone, com o bipe do
  // push-to-talk se o som estiver ligado.
  setTalking: (talking: boolean) => void
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
    // O áudio da tela é uma track separada (fonte ScreenShareAudio), que só
    // existe se a pessoa marcou "Compartilhar áudio" no seletor.
    screenShareAudio: !!p.getTrackPublication(Track.Source.ScreenShareAudio),
    speaking: p.isSpeaking,
  }
}

// Conecta a um canal de voz via LiveKit (ver docs/architecture.md, "Decisão:
// integração de voz com LiveKit"). Cada canal tem sua própria sala LiveKit
// (nome = id do canal); entrar busca um token novo em
// POST /api/channels/{id}/voice/token a cada tentativa, em vez de cachear.
// A chamada é da sessão, não da tela do canal: o hook fica no
// VoiceSessionProvider e a chamada continua enquanto a pessoa navega por
// texto, fórum, outros servidores e amigos. Ver docs/architecture.md,
// "Decisão: chamada de voz continua ao navegar".
// Constraints do áudio da tela, repassadas cruas ao getDisplayMedia pelo
// livekit-client. Sem restrictOwnAudio por enquanto: com ele (Chrome 141+),
// tela inteira com áudio do sistema no Windows publicou a track (🔊 na
// lista) mas sem som nenhum, mesmo com o som vindo de outro programa. Sem
// ele, as vozes da sala tocadas por esta página podem voltar para a sala
// nesse caso. Ver docs/architecture.md, "Decisão: áudio da tela
// compartilhada".
const SCREEN_SHARE_AUDIO_CONSTRAINTS: AudioCaptureOptions = {
  echoCancellation: false,
  noiseSuppression: false,
  autoGainControl: false,
}

// No app desktop o áudio da tela é o loopback do Windows (electron/main.ts),
// que pega também as vozes da sala tocadas por este app. Ali o
// restrictOwnAudio funciona: medido no Electron 44, zera o som da própria
// página e mantém o de outros programas. Ver docs/architecture.md, "Decisão:
// compartilhamento de tela no Electron".
const ELECTRON_SCREEN_SHARE_AUDIO_CONSTRAINTS = {
  ...SCREEN_SHARE_AUDIO_CONSTRAINTS,
  restrictOwnAudio: true,
} as AudioCaptureOptions

// Captura com a supressão reforçada (RNNoise) ligada: sem o supressor do
// navegador nem o voiceIsolation (outro supressor), para não empilhar dois;
// cancelamento de eco e ganho automático continuam. Ver docs/architecture.md,
// "Decisão: supressão de ruído no microfone".
const ENHANCED_CAPTURE_OPTIONS: AudioCaptureOptions = {
  echoCancellation: true,
  noiseSuppression: false,
  autoGainControl: true,
  voiceIsolation: false,
}

// Os padrões do livekit-client 2.22 (audioDefaults), explícitos para voltar
// a eles com restartTrack ao desligar a reforçada no meio da chamada.
const BROWSER_CAPTURE_OPTIONS: AudioCaptureOptions = {
  echoCancellation: true,
  noiseSuppression: true,
  autoGainControl: true,
  voiceIsolation: true,
}

// Onde está a track do microfone: capturada com a supressão do navegador,
// capturada sem ela e ainda sem o RNNoise (passo intermediário), ou com o
// RNNoise aplicado.
type NoiseMode = 'browser' | 'raw' | 'enhanced'

// Sem escolha nenhuma: constante do módulo para o efeito que reaplica os
// volumes não rodar a cada render.
const NO_PARTICIPANT_AUDIO: ParticipantAudioMap = {}

export function useVoiceChannel(
  accessToken: string,
  // Aviso sonoro ao mutar/desmutar e ao apertar/soltar no push-to-talk
  // (lib/voicePrefs.ts). Lido por ref para
  // mudar a preferência sem recriar os callbacks.
  micToggleSound = true,
  // Volume por pessoa e "silenciar para mim" (lib/participantAudio.ts),
  // chaveado pela identity LiveKit, que é o memberId.
  participantAudio: ParticipantAudioMap = NO_PARTICIPANT_AUDIO,
  // Modo "apertar para falar": entra com o microfone publicado mas mutado, e
  // quem abre e fecha é setTalking. Trocar o modo conectado fecha ou abre o
  // microfone. Ver docs/architecture.md, "Decisão: push-to-talk".
  pushToTalk = false,
  // "Supressão de ruído reforçada" (lib/rnnoiseProcessor.ts). Trocar
  // conectado recaptura o microfone com as novas constraints.
  enhancedNoiseSuppression = false,
  // Som de alguém entrando ou saindo da chamada (lib/presenceSound.ts), lido
  // por ref como o micToggleSound.
  presenceSound = true,
): UseVoiceChannelResult {
  const roomRef = useRef<Room | undefined>(undefined)
  // Cada join e cada saída avançam o contador: um join que volta de um await
  // depois de outro join (troca de canal) ou de sair desiste sozinho.
  const joinSeqRef = useRef(0)
  const [target, setTarget] = useState<VoiceTarget>()
  // join chamado de dentro dos handlers da sala (ser movido de sala).
  const joinRef = useRef<(target: VoiceTarget, issued?: { token: string; url: string }) => Promise<void>>(
    async () => {},
  )
  const targetRef = useRef<VoiceTarget | undefined>(undefined)
  const micToggleSoundRef = useRef(micToggleSound)
  useEffect(() => {
    micToggleSoundRef.current = micToggleSound
  }, [micToggleSound])
  const presenceSoundRef = useRef(presenceSound)
  useEffect(() => {
    presenceSoundRef.current = presenceSound
  }, [presenceSound])
  // Quem está na sala para o som de entrada e saída. `ready` só liga depois
  // do join terminar: o LiveKit dispara ParticipantConnected para quem já
  // estava na sala durante o connect, e isso não é alguém entrando. `known`
  // sobrevive ao reconectar completo, que desfaz e refaz todos os
  // participantes remotos e soaria como a sala inteira saindo e voltando.
  const presenceRef = useRef<{ room: Room; ready: boolean; known: Set<string> } | undefined>(undefined)
  const participantAudioRef = useRef(participantAudio)
  const pushToTalkRef = useRef(pushToTalk)
  // Estado do microfone pedido por último e a sala com uma troca em
  // andamento (ver setMicDesired).
  const micDesiredRef = useRef(false)
  const micBusyRoomRef = useRef<Room | undefined>(undefined)
  const enhancedNoiseRef = useRef(enhancedNoiseSuppression)
  // Modo aplicado à track do microfone da sala atual (ausente até o
  // microfone ser publicado) e a fila das trocas (ver syncNoiseSuppression).
  const noiseModeRef = useRef<{ room: Room; mode: NoiseMode } | undefined>(undefined)
  const noiseQueueRef = useRef<Promise<void>>(Promise.resolve())
  const audioElsRef = useRef<Set<HTMLMediaElement>>(new Set())
  const videoContainerElRef = useRef<HTMLDivElement | null>(null)
  const videoTilesRef = useRef<Map<string, HTMLDivElement>>(new Map())
  const [status, setStatus] = useState<VoiceChannelStatus>('idle')
  const [error, setError] = useState<DisplayError>()
  const [participants, setParticipants] = useState<VoiceParticipant[]>([])
  const [micEnabled, setMicEnabled] = useState(false)
  const [cameraError, setCameraError] = useState<DisplayError>()
  const [micError, setMicError] = useState<DisplayError>()
  const [audioPlaybackBlocked, setAudioPlaybackBlocked] = useState(false)
  const [noiseSuppressionError, setNoiseSuppressionError] = useState<DisplayError>()

  const cleanupAudioEls = useCallback(() => {
    audioElsRef.current.forEach((el) => el.remove())
    audioElsRef.current.clear()
  }, [])

  // Toca uma track de áudio remota (voz ou tela) no volume escolhido para
  // aquela pessoa. O ganho vem do GainNode do webAudioMix (ver join), por isso
  // passa de 100%. Volume 0 ou pessoa silenciada desanexa a track em vez de
  // pôr o ganho em 0: o attach() do LiveKit recria o GainNode em 100% e só
  // reaplica volumes "truthy", então com 0 a voz vazaria a cada nova
  // assinatura (reconexão, tela compartilhada de novo).
  const applyRemoteAudio = useCallback((track: RemoteAudioTrack, identity: string) => {
    const audio = participantAudioOf(participantAudioRef.current, identity)
    const volume = audio.muted ? 0 : track.source === Track.Source.ScreenShareAudio ? audio.screen : audio.voice
    if (volume === 0) {
      track.detach().forEach((el) => {
        audioElsRef.current.delete(el)
        el.remove()
      })
      return
    }
    if (track.attachedElements.length === 0) {
      const el = track.attach()
      document.body.appendChild(el)
      audioElsRef.current.add(el)
    }
    track.setVolume(volume)
  }, [])

  // Reaplica em todas as tracks já tocando quando a pessoa mexe num volume.
  useEffect(() => {
    participantAudioRef.current = participantAudio
    roomRef.current?.remoteParticipants.forEach((p) => {
      p.audioTrackPublications.forEach((publication) => {
        if (publication.audioTrack) applyRemoteAudio(publication.audioTrack as RemoteAudioTrack, p.identity)
      })
    })
  }, [participantAudio, applyRemoteAudio])

  const cleanupVideoTiles = useCallback(() => {
    videoTilesRef.current.forEach((tile) => tile.remove())
    videoTilesRef.current.clear()
  }, [])

  // Callback ref: a UI monta/desmonta a <div> de destino ao longo do ciclo de
  // vida do componente, mas as tiles de vídeo (câmera e tela) são inseridas
  // de forma imperativa (mesmo padrão já usado para os elementos de áudio),
  // então guardamos o nó atual para anexar/remover tiles depois.
  // A grade sai da tela quando a pessoa abre outro canal com a chamada em
  // andamento, e o navegador pausa um <video> tirado do documento: ao voltar,
  // as tiles são reanexadas e o vídeo precisa de play() de novo.
  const videoContainerRef = useCallback((node: HTMLDivElement | null) => {
    videoContainerElRef.current = node
    if (node) {
      videoTilesRef.current.forEach((tile) => {
        node.appendChild(tile)
        void tile.querySelector('video')?.play().catch(() => {})
      })
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
    const labelText = () => {
      if (participant.isLocal) return i18n.t('voice.selfName', { name })
      if (publication.source === Track.Source.ScreenShare) return i18n.t('voice.tile.screen', { name })
      return name
    }
    const tile = document.createElement('div')
    tile.className = participant.isLocal ? 'video-tile local' : 'video-tile'
    if (publication.isMuted) tile.classList.add('muted')
    // Foco: no máximo uma tile com a classe "focused"; o CSS amplia essa
    // tile e reduz as demais a miniaturas. Uma tile focada que some
    // (removida) ou é escondida (câmera desligada) desfaz o layout sozinha,
    // porque o seletor é :has(.video-tile.focused:not(.muted)).
    tile.tabIndex = 0
    const toggleFocus = () => {
      const focusing = !tile.classList.contains('focused')
      videoTilesRef.current.forEach((t) => t.classList.remove('focused'))
      tile.classList.toggle('focused', focusing)
    }
    tile.addEventListener('click', toggleFocus)
    tile.addEventListener('keydown', (event) => {
      if (event.target !== tile || (event.key !== 'Enter' && event.key !== ' ')) return
      event.preventDefault()
      toggleFocus()
    })
    const fullscreen = document.createElement('button')
    fullscreen.type = 'button'
    fullscreen.className = 'video-tile-fullscreen'
    const updateTexts = () => {
      label.textContent = labelText()
      tile.title = i18n.t('voice.tile.clickToEnlarge')
      fullscreen.title = i18n.t('voice.tile.fullscreen')
      fullscreen.setAttribute('aria-label', i18n.t('voice.tile.fullscreenOf', { name: label.textContent }))
    }
    updateTexts()
    tileTextUpdaters.set(tile, updateTexts)
    fullscreen.textContent = '⛶'
    fullscreen.addEventListener('click', (event) => {
      event.stopPropagation()
      if (document.fullscreenElement === tile) {
        void document.exitFullscreen()
      } else {
        void tile.requestFullscreen().catch(() => {
          // Fullscreen negado (iframe sem allowfullscreen, política do
          // navegador): o foco na grade continua disponível.
        })
      }
    })
    tile.appendChild(video)
    tile.appendChild(label)
    tile.appendChild(fullscreen)
    videoTilesRef.current.set(publication.trackSid, tile)
    videoContainerElRef.current?.appendChild(tile)
  }, [])

  // Tiles já montadas acompanham a troca de idioma.
  useEffect(() => {
    const onLanguageChanged = () => videoTilesRef.current.forEach((tile) => tileTextUpdaters.get(tile)?.())
    i18n.on('languageChanged', onLanguageChanged)
    return () => i18n.off('languageChanged', onLanguageChanged)
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
      if (room && presenceRef.current?.room === room) {
        if (presenceRef.current.ready && presenceSoundRef.current) playPresenceSound(false)
        presenceRef.current = undefined
      }
      room?.disconnect()
      // Sala de uma chamada anterior (o Disconnected dela chega depois de
      // trocar de canal): o estado já é da sala nova.
      if (room !== roomRef.current) return
      cleanupAudioEls()
      cleanupVideoTiles()
      roomRef.current = undefined
      noiseModeRef.current = undefined
      setStatus('idle')
      setParticipants([])
      setMicEnabled(false)
      setCameraError(undefined)
      setMicError(undefined)
      setAudioPlaybackBlocked(false)
      setNoiseSuppressionError(undefined)
    },
    [cleanupAudioEls, cleanupVideoTiles],
  )

  // Estar na chamada conta como atividade para o "ausente" automático
  // (hooks/useIdle.ts).
  useEffect(() => {
    if (status !== 'connected') return
    setVoiceConnected(true)
    return () => setVoiceConnected(false)
  }, [status])

  // App Android: serviço em primeiro plano com a notificação "Em chamada"
  // (lib/androidCallService.ts) do clique em entrar até sair. Começa já em
  // 'connecting', ainda com o app visível, como o Android 14 exige para o
  // microfone; isso também mantém o serviço ao ser movido de sala com o app
  // em segundo plano, quando ele não poderia ser ligado de novo. Erro,
  // desconexão e logout (o provider desmonta) levam a undefined e param o
  // serviço. No navegador e no desktop não faz nada.
  const { t } = useTranslation()
  useEffect(() => {
    const active = target && (status === 'connecting' || status === 'connected')
    syncCallNotification(
      active
        ? {
            channelName: t('voice.androidCall.channelName'),
            title: t('voice.androidCall.title', { channel: target.channelName, server: target.serverName }),
            text:
              status === 'connecting'
                ? t('voice.androidCall.connecting')
                : micEnabled
                  ? t('voice.androidCall.micOn')
                  : t('voice.androidCall.micOff'),
            toggleMicLabel:
              status === 'connected' ? (micEnabled ? t('voice.androidCall.mute') : t('voice.androidCall.unmute')) : undefined,
            leaveLabel: t('voice.androidCall.leave'),
          }
        : undefined,
    )
  }, [target, status, micEnabled, t])
  useEffect(() => () => syncCallNotification(undefined), [])

  // Sair da chamada quando a sessão acaba (logout desmonta o provider).
  useEffect(() => {
    const joinSeq = joinSeqRef
    return () => {
      joinSeq.current++
      disconnect(roomRef.current)
    }
  }, [disconnect])

  // Leva a track do microfone ao modo pedido (reforçada ou do navegador).
  // As trocas vão numa fila, porque recapturar e plugar o processador levam
  // tempo e a pessoa pode ligar e desligar antes de terminar. Se a reforçada
  // falhar, volta para a supressão do navegador e mostra o aviso, em vez de
  // deixar o microfone sem supressão nenhuma.
  const syncNoiseSuppression = useCallback((room: Room) => {
    noiseQueueRef.current = noiseQueueRef.current
      .then(async () => {
        // Cada troca pedida tenta de novo, então o aviso de uma falha
        // anterior sai.
        if (roomRef.current === room) setNoiseSuppressionError(undefined)
        let failed = false
        while (roomRef.current === room && noiseModeRef.current?.room === room) {
          const applied = noiseModeRef.current
          const want: NoiseMode = enhancedNoiseRef.current && !failed ? 'enhanced' : 'browser'
          if (applied.mode === want) return
          const track = room.localParticipant.getTrackPublication(Track.Source.Microphone)?.track as
            | LocalAudioTrack
            | undefined
          if (!track) return
          try {
            if (want === 'enhanced') {
              if (applied.mode === 'browser') {
                await track.restartTrack(ENHANCED_CAPTURE_OPTIONS)
                applied.mode = 'raw'
              }
              const { createRnnoiseProcessor } = await import('../lib/rnnoiseProcessor')
              await track.setProcessor(createRnnoiseProcessor())
              applied.mode = 'enhanced'
            } else {
              if (applied.mode === 'enhanced') {
                await track.stopProcessor()
                applied.mode = 'raw'
              }
              await track.restartTrack(BROWSER_CAPTURE_OPTIONS)
              applied.mode = 'browser'
            }
          } catch (err) {
            // Falha ao voltar para a do navegador não tem para onde cair; o
            // microfone segue como ficou.
            if (want === 'browser') return
            failed = true
            console.warn('[ffcom] supressão de ruído reforçada', err)
            if (roomRef.current === room) {
              setNoiseSuppressionError(new LocalizedError(() => i18n.t('voice.noiseSuppressionFailed')))
            }
          }
        }
      })
      .catch(() => {})
  }, [])

  // issued: token já emitido pelo servidor (ao ser movido de sala, ver
  // VOICE_MOVE_TOPIC), em vez de pedir um com POST .../voice/token.
  const join = useCallback(async (next: VoiceTarget, issued?: { token: string; url: string }) => {
    const current = targetRef.current
    if (
      roomRef.current &&
      current?.serverId === next.serverId &&
      current.channelId === next.channelId
    ) {
      return
    }
    const seq = ++joinSeqRef.current
    disconnect(roomRef.current)
    targetRef.current = next
    setTarget(next)
    // Ainda dentro do clique: destrava o AudioContext para o som de entrada,
    // que só toca depois do connect.
    if (presenceSoundRef.current) primeMicToggleSound()
    setStatus('connecting')
    setError(undefined)
    try {
      const { token, url } = issued ?? (await fetchVoiceToken(next.baseUrl, next.channelId, accessToken))
      if (seq !== joinSeqRef.current) return
      // webAudioMix: o áudio remoto toca por um AudioContext com um GainNode
      // por track em vez do volume do <audio> (limitado a 100%), para o
      // volume por pessoa ir até 200%. Ver docs/architecture.md, "Decisão:
      // volume por pessoa no canal de voz".
      // Com a reforçada ligada, o microfone já nasce capturado sem o
      // supressor do navegador, e o WASM começa a baixar enquanto conecta.
      const captureEnhanced = enhancedNoiseRef.current
      if (captureEnhanced) {
        void import('../lib/rnnoiseProcessor').then((m) => m.preloadRnnoise()).catch(() => {})
      }
      const room = new Room({
        webAudioMix: true,
        audioCaptureDefaults: captureEnhanced ? ENHANCED_CAPTURE_OPTIONS : undefined,
      })
      roomRef.current = room
      const presence = { room, ready: false, known: new Set<string>() }
      presenceRef.current = presence

      room.on(RoomEvent.ParticipantConnected, (participant) => {
        if (!presence.known.has(participant.identity)) {
          presence.known.add(participant.identity)
          if (presence.ready && presenceSoundRef.current) playPresenceSound(true)
        }
        refreshParticipants(room)
      })
      room.on(RoomEvent.ParticipantDisconnected, (participant) => {
        // No reconectar completo o LiveKit desfaz os participantes ainda com
        // a sala em Connected e só depois passa para Reconnecting, no mesmo
        // tick: conferir o estado depois separa isso de uma saída de verdade.
        queueMicrotask(() => {
          if (room.state !== ConnectionState.Connected) return
          if (room.remoteParticipants.has(participant.identity)) return
          if (!presence.known.delete(participant.identity)) return
          if (presence.ready && presenceSoundRef.current) playPresenceSound(false)
        })
        refreshParticipants(room)
      })
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
          applyRemoteAudio(track as RemoteAudioTrack, participant.identity)
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
      room.on(RoomEvent.ActiveSpeakersChanged, () => refreshParticipants(room))
      // Alguém com MoveMembers puxou você para outra sala. Só vale vindo do
      // servidor (sem participante de origem): um participante comum também
      // consegue mandar dados com qualquer tópico.
      room.on(RoomEvent.DataReceived, (payload, participant, _kind, topic) => {
        if (topic !== VOICE_MOVE_TOPIC || participant || roomRef.current !== room) return
        const move = parseVoiceMove(payload)
        const from = targetRef.current
        if (!move || !from) return
        void joinRef.current(
          { ...from, channelId: move.channelId, channelName: move.channelName },
          { token: move.token, url: move.url },
        )
      })
      room.on(RoomEvent.Disconnected, () => disconnect(room))
      // Os <audio> das vozes remotas são criados depois do clique em
      // "Entrar" (quando cada track chega), e o navegador pode bloquear o
      // play() deles: o LiveKit avisa aqui e só destrava com startAudio()
      // chamado a partir de um toque. Sem tratar isso, o celular ficava
      // mudo sem nenhum aviso (o microfone dele funcionava normalmente).
      room.on(RoomEvent.AudioPlaybackStatusChanged, () => setAudioPlaybackBlocked(!room.canPlaybackAudio))

      // Chave de depuração: com localStorage['ffcom:forceRelay'] = '1' o ICE
      // só usa candidatos TURN, para testar se o relay funciona de ponta a
      // ponta. Ver docs/architecture.md, "Decisão: TURN/TLS na 443".
      let forceRelay = false
      try {
        forceRelay = localStorage.getItem('ffcom:forceRelay') === '1'
      } catch {
        // localStorage indisponível: segue sem forçar
      }
      if (forceRelay) console.info('[ffcom] forceRelay: ICE só por TURN')
      await room.connect(url, token, forceRelay ? { rtcConfig: { iceTransportPolicy: 'relay' } } : undefined)
      if (seq !== joinSeqRef.current) return
      setMicError(undefined)
      try {
        if (pushToTalkRef.current) {
          // Pede a permissão e publica já mutado, para o primeiro aperto abrir
          // na hora (sem o seletor de permissão com a tecla apertada) e sem
          // transmitir nada ao entrar. O setMicrophoneEnabled(true) seguinte
          // só desmuta a publicação existente.
          const micTrack = await createLocalAudioTrack(room.options.audioCaptureDefaults)
          try {
            await micTrack.mute()
            await room.localParticipant.publishTrack(micTrack, { source: Track.Source.Microphone })
          } catch (err) {
            // Não publicada, a track não sai com o disconnect: sem isso o
            // microfone ficaria capturado (luz acesa) depois do erro.
            micTrack.stop()
            throw err
          }
          micDesiredRef.current = false
          setMicEnabled(false)
        } else {
          await room.localParticipant.setMicrophoneEnabled(true)
          micDesiredRef.current = true
          setMicEnabled(true)
        }
      } catch (err) {
        // Sem microfone (ou sem permissão) a pessoa ainda pode ouvir: fica
        // na sala com o microfone fechado e o aviso na tela. Outras falhas
        // (publicar a track) seguem derrubando a entrada.
        if (seq !== joinSeqRef.current) return
        if (!isMediaDeviceError(err)) throw err
        micDesiredRef.current = false
        setMicEnabled(false)
        setMicError(micErrorMessage(err))
      }
      // Com o microfone aberto, ele sai sem supressão nenhuma até o RNNoise
      // entrar (o WASM já vem baixando desde o começo do join); não espera
      // por isso para mostrar a sala.
      noiseModeRef.current = { room, mode: captureEnhanced ? 'raw' : 'browser' }
      syncNoiseSuppression(room)
      setAudioPlaybackBlocked(!room.canPlaybackAudio)
      setStatus('connected')
      refreshParticipants(room)
      if (presenceRef.current === presence) {
        presence.ready = true
        if (presenceSoundRef.current) playPresenceSound(true)
      }
    } catch (err) {
      // Trocou de canal ou saiu no meio: quem fez isso já desfez esta sala.
      if (seq !== joinSeqRef.current) return
      disconnect(roomRef.current)
      setStatus('error')
      setError(err instanceof Error ? err : new LocalizedError(() => i18n.t('voice.connectFailed')))
    }
  }, [
    accessToken,
    refreshParticipants,
    disconnect,
    addVideoTile,
    removeVideoTile,
    applyRemoteAudio,
    syncNoiseSuppression,
  ])

  useEffect(() => {
    joinRef.current = join
  }, [join])

  const leave = useCallback(() => {
    joinSeqRef.current++
    disconnect(roomRef.current)
    targetRef.current = undefined
    setTarget(undefined)
  }, [disconnect])

  // Precisa rodar dentro do handler do clique: é o toque que autoriza o
  // navegador a tocar áudio.
  const startAudio = useCallback(() => {
    const room = roomRef.current
    if (!room) return
    room
      .startAudio()
      .then(() => setAudioPlaybackBlocked(!room.canPlaybackAudio))
      .catch(() => setAudioPlaybackBlocked(true))
  }, [])

  const toggleMic = useCallback(() => {
    const room = roomRef.current
    if (!room) return
    const next = !room.localParticipant.isMicrophoneEnabled
    // Destrava o AudioContext ainda dentro do gesto; o som só toca depois
    // que a troca resolver, para não avisar uma troca que falhou.
    if (micToggleSoundRef.current) primeMicToggleSound()
    room.localParticipant
      .setMicrophoneEnabled(next)
      .then(() => {
        setMicEnabled(next)
        setMicError(undefined)
        if (micToggleSoundRef.current) playMicToggleSound(next)
        // Se o microfone não abriu ao entrar, a track nasce agora e precisa
        // da supressão escolhida.
        if (next) syncNoiseSuppression(room)
        refreshParticipants(room)
      })
      .catch((err) => {
        if (isMediaDeviceError(err)) setMicError(micErrorMessage(err))
        refreshParticipants(room)
      })
  }, [refreshParticipants, syncNoiseSuppression])

  // "Mutar"/"Desmutar" e "Sair" da notificação "Em chamada" do app Android
  // (lib/androidCallService.ts). Por ref, para assinar uma vez só: trocar o
  // listener a cada render abriria uma janela com dois e uma ação dobrada.
  const notificationActionsRef = useRef({ toggleMic, leave })
  useEffect(() => {
    notificationActionsRef.current = { toggleMic, leave }
  }, [toggleMic, leave])
  useEffect(
    () =>
      onCallNotificationAction((action) => {
        if (action === 'toggleMic') notificationActionsRef.current.toggleMic()
        else if (action === 'leave') notificationActionsRef.current.leave()
      }),
    [],
  )

  // Push-to-talk aperta e solta mais rápido do que setMicrophoneEnabled
  // resolve; chamadas sobrepostas poderiam terminar fora de ordem e deixar o
  // microfone aberto depois de soltar. Guarda só o último estado pedido e
  // aplica em série até a sala bater com ele.
  const setMicDesired = useCallback(
    (enabled: boolean) => {
      const room = roomRef.current
      if (!room) return
      micDesiredRef.current = enabled
      if (micBusyRoomRef.current === room) return
      micBusyRoomRef.current = room
      void (async () => {
        try {
          // Só conta as voltas em que o LiveKit resolveu sem mudar o estado,
          // para não ficar em laço; apertar e soltar muitas vezes seguidas
          // não esbarra no limite.
          let stuck = 0
          while (roomRef.current === room) {
            const want = micDesiredRef.current
            if (room.localParticipant.isMicrophoneEnabled === want) break
            await room.localParticipant.setMicrophoneEnabled(want)
            if (room.localParticipant.isMicrophoneEnabled !== want && ++stuck >= 3) break
          }
          if (roomRef.current === room && room.localParticipant.isMicrophoneEnabled) {
            setMicError(undefined)
            syncNoiseSuppression(room)
          }
        } catch (err) {
          // Dispositivo sumiu ou permissão revogada: o estado real aparece
          // abaixo.
          if (roomRef.current === room && isMediaDeviceError(err)) setMicError(micErrorMessage(err))
        } finally {
          if (micBusyRoomRef.current === room) micBusyRoomRef.current = undefined
          if (roomRef.current === room) {
            setMicEnabled(room.localParticipant.isMicrophoneEnabled)
            refreshParticipants(room)
          }
        }
      })()
    },
    [refreshParticipants, syncNoiseSuppression],
  )

  // O bipe toca no aperto e no soltar, sem esperar a troca resolver: no
  // push-to-talk ele é o retorno da tecla, e abrir o microfone já publicado
  // é só desmutar. Trocar de modo (efeito abaixo) chama setMicDesired direto
  // e não toca nada.
  const setTalking = useCallback(
    (talking: boolean) => {
      if (micToggleSoundRef.current) playPushToTalkSound(talking)
      setMicDesired(talking)
    },
    [setMicDesired],
  )

  // Trocar de modo conectado: "apertar para falar" começa fechado e "sempre
  // aberto" abre o microfone.
  useEffect(() => {
    pushToTalkRef.current = pushToTalk
    // Ainda entrando na sala: join lê pushToTalkRef e aplica o modo sozinho.
    if (roomRef.current?.state === ConnectionState.Connected) setMicDesired(!pushToTalk)
  }, [pushToTalk, setMicDesired])

  // Ligar ou desligar a reforçada conectado. Antes de o microfone ser
  // publicado, join aplica o modo sozinho ao terminar.
  useEffect(() => {
    enhancedNoiseRef.current = enhancedNoiseSuppression
    const room = roomRef.current
    if (room && noiseModeRef.current?.room === room) syncNoiseSuppression(room)
  }, [enhancedNoiseSuppression, syncNoiseSuppression])

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
  // erro real do canal de voz, só a desistência do usuário. No app desktop o
  // seletor já foi mostrado antes (ScreenSharePicker) e o getDisplayMedia
  // usa a fonte escolhida lá.
  //
  // Pede o áudio junto (publicado como track ScreenShareAudio, separada do
  // microfone; quem assiste já toca toda track de áudio remota). Só vem se a
  // pessoa marcar "Compartilhar áudio" no seletor: Chrome/Edge capturam o de
  // uma aba e, no Windows, o do sistema na tela inteira; Firefox e Safari
  // não capturam. Cancelamento de eco, supressão de ruído e ganho automático
  // ficam desligados porque são feitos para voz e estragam música e jogo.
  // Ver docs/architecture.md, "Decisão: áudio da tela compartilhada".
  const toggleScreenShare = useCallback(async () => {
    const room = roomRef.current
    if (!room) return
    const next = !room.localParticipant.isScreenShareEnabled
    try {
      await room.localParticipant.setScreenShareEnabled(next, {
        audio: window.ffcomElectron ? ELECTRON_SCREEN_SHARE_AUDIO_CONSTRAINTS : SCREEN_SHARE_AUDIO_CONSTRAINTS,
        systemAudio: 'include',
      })
      // Diagnóstico do áudio da tela: o que o navegador aplicou de fato
      // (dispositivo, filtros, restrictOwnAudio quando suportado). Fica no
      // console de quem compartilha, para investigar áudio mudo sem chute.
      const screenAudio = room.localParticipant.getTrackPublication(Track.Source.ScreenShareAudio)?.track
      if (next) {
        console.info('[ffcom] áudio da tela', screenAudio ? screenAudio.mediaStreamTrack.getSettings() : 'sem track de áudio')
      }
    } catch {
      return
    }
    refreshParticipants(room)
  }, [refreshParticipants])

  const screenSharing = participants.some((p) => p.isLocal && p.screenSharing)
  const screenShareAudio = participants.some((p) => p.isLocal && p.screenShareAudio)
  const cameraEnabled = participants.some((p) => p.isLocal && p.cameraEnabled)
  // Erros guardados como Error/LocalizedError e traduzidos aqui, no idioma
  // ativo (useErrorText renderiza de novo quando ele muda).
  const errorText = useErrorText(error)
  const cameraErrorText = useErrorText(cameraError)
  const micErrorText = useErrorText(micError)
  const noiseSuppressionErrorText = useErrorText(noiseSuppressionError)

  return {
    target,
    status,
    error: errorText,
    participants,
    micEnabled,
    cameraEnabled,
    cameraError: cameraErrorText,
    micError: micErrorText,
    screenSharing,
    screenShareAudio,
    audioPlaybackBlocked,
    noiseSuppressionError: noiseSuppressionErrorText,
    startAudio,
    videoContainerRef,
    join,
    leave,
    toggleMic,
    setTalking,
    toggleCamera,
    toggleScreenShare,
  }
}

function cameraErrorMessage(err: unknown): DisplayError {
  const name = err instanceof Error ? err.name : ''
  if (name === 'NotAllowedError') return new LocalizedError(() => i18n.t('voice.camera.permissionDenied'))
  if (name === 'NotFoundError' || name === 'OverconstrainedError') {
    return new LocalizedError(() => i18n.t('voice.camera.notFound'))
  }
  if (name === 'NotReadableError') return new LocalizedError(() => i18n.t('voice.camera.inUse'))
  return err instanceof Error ? err : new LocalizedError(() => i18n.t('voice.camera.failed'))
}

// Erros do getUserMedia ao abrir o microfone: o dispositivo não existe, a
// permissão foi negada ou outro programa está com ele.
const MEDIA_DEVICE_ERRORS = new Set(['NotFoundError', 'OverconstrainedError', 'NotAllowedError', 'NotReadableError'])

function isMediaDeviceError(err: unknown): boolean {
  return err instanceof Error && MEDIA_DEVICE_ERRORS.has(err.name)
}

function micErrorMessage(err: unknown): DisplayError {
  const name = err instanceof Error ? err.name : ''
  if (name === 'NotAllowedError') return new LocalizedError(() => i18n.t('voice.mic.permissionDenied'))
  if (name === 'NotReadableError') return new LocalizedError(() => i18n.t('voice.mic.inUse'))
  return new LocalizedError(() => i18n.t('voice.mic.notFound'))
}

// Tópico da mensagem do servidor que manda trocar de sala (POST
// /api/voice/move). Mesmo valor de VoiceMoveTopic em
// server-channel/internal/httpapi/voice_move.go.
const VOICE_MOVE_TOPIC = 'ffcom.voice.move'

interface VoiceMove {
  channelId: string
  channelName: string
  token: string
  url: string
}

function parseVoiceMove(payload: Uint8Array): VoiceMove | undefined {
  try {
    const move = JSON.parse(new TextDecoder().decode(payload)) as Partial<VoiceMove>
    if (typeof move.channelId !== 'string' || typeof move.token !== 'string' || typeof move.url !== 'string') {
      return undefined
    }
    return { channelId: move.channelId, channelName: move.channelName ?? '', token: move.token, url: move.url }
  } catch {
    return undefined
  }
}
