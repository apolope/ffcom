// "Supressão de ruído reforçada": RNNoise (Xiph) em WASM num AudioWorklet,
// via @sapphi-red/web-noise-suppressor, plugado na track do microfone como
// TrackProcessor do LiveKit. Ver docs/architecture.md, "Decisão: supressão de
// ruído no microfone".
import type { AudioProcessorOptions, Track, TrackProcessor } from 'livekit-client'
import { RnnoiseWorkletNode, loadRnnoise } from '@sapphi-red/web-noise-suppressor'
// ?url: o Vite copia os arquivos para o build com hash. O hook importa este
// módulo de forma dinâmica, então nada disso é baixado sem a opção ligada.
import rnnoiseWorkletUrl from '@sapphi-red/web-noise-suppressor/rnnoiseWorklet.js?url'
import rnnoiseWasmUrl from '@sapphi-red/web-noise-suppressor/rnnoise.wasm?url'
import rnnoiseSimdWasmUrl from '@sapphi-red/web-noise-suppressor/rnnoise_simd.wasm?url'

// O WASM (~150 KB) é baixado uma vez por página; o SIMD entra onde o
// navegador suporta. Uma falha não fica em cache, para a próxima tentativa
// baixar de novo.
let wasmPromise: Promise<ArrayBuffer> | undefined
function loadWasm(): Promise<ArrayBuffer> {
  wasmPromise ??= loadRnnoise({ url: rnnoiseWasmUrl, simdUrl: rnnoiseSimdWasmUrl }).catch((err: unknown) => {
    wasmPromise = undefined
    throw err
  })
  return wasmPromise
}

// Começa o download antes de precisar (ao entrar no canal com a opção
// ligada), para o microfone não sair sem filtro por mais tempo que o
// necessário. Erro aqui é ignorado: o init tenta de novo e reporta.
export function preloadRnnoise(): void {
  loadWasm().catch(() => {})
}

// Um processador por track. Usa um AudioContext próprio a 48 kHz em vez do
// da sala (que o LiveKit cria na taxa do dispositivo, às vezes 44,1 kHz),
// porque o RNNoise assume 48 kHz. O processedTrack é a saída de um
// MediaStreamDestination e continua o mesmo quando o LiveKit reinicia a
// captura (restart), só a fonte é trocada.
export function createRnnoiseProcessor(): TrackProcessor<Track.Kind.Audio, AudioProcessorOptions> {
  let ctx: AudioContext | undefined
  let source: MediaStreamAudioSourceNode | undefined
  let node: RnnoiseWorkletNode | undefined

  const connectSource = (track: MediaStreamTrack) => {
    source?.disconnect()
    source = ctx!.createMediaStreamSource(new MediaStream([track]))
    source.connect(node!)
  }

  const processor: TrackProcessor<Track.Kind.Audio, AudioProcessorOptions> = {
    name: 'ffcom-rnnoise',
    async init(opts) {
      const wasmBinary = await loadWasm()
      ctx = new AudioContext({ sampleRate: 48000, latencyHint: 'interactive' })
      try {
        await ctx.audioWorklet.addModule(rnnoiseWorkletUrl)
        node = new RnnoiseWorkletNode(ctx, { maxChannels: 1, wasmBinary })
        // Microfone estéreo vira mono antes do RNNoise.
        node.channelCount = 1
        node.channelCountMode = 'explicit'
        const destination = ctx.createMediaStreamDestination()
        node.connect(destination)
        connectSource(opts.track)
        processor.processedTrack = destination.stream.getAudioTracks()[0]
        // Entrar no canal é um clique, então o contexto normalmente já nasce
        // rodando. Se o navegador o deixar suspenso (o resume fica pendente
        // até um toque), a saída seria silêncio: melhor falhar e o hook
        // voltar para a supressão do navegador do que mandar voz nenhuma.
        if (ctx.state === 'suspended') {
          await Promise.race([ctx.resume(), new Promise((resolve) => setTimeout(resolve, 500))])
        }
        if (ctx.state !== 'running') throw new Error('o navegador não liberou o áudio da supressão reforçada')
      } catch (err) {
        await processor.destroy()
        throw err
      }
    },
    // O LiveKit chama ao recapturar o microfone (troca de dispositivo, track
    // encerrada, restartTrack), sem passar o audioContext.
    async restart(opts) {
      if (!ctx || !node) return
      connectSource(opts.track)
    },
    async destroy() {
      source?.disconnect()
      node?.disconnect()
      node?.destroy()
      processor.processedTrack?.stop()
      processor.processedTrack = undefined
      source = undefined
      node = undefined
      const closing = ctx
      ctx = undefined
      if (closing && closing.state !== 'closed') await closing.close()
    },
  }
  return processor
}
