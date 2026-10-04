import { ApiError, LocalizedError } from './apiError'

// Mantém o WebSocket de um canal (texto ou forum) aberto enquanto o job
// existir, reconectando quando ele cai — ex. close 1001 na troca de versão
// de server-channel (ver docs/architecture.md, "Decisão: container evergreen
// em `server-channel`", item 7). Espera 1s, 2s, 4s… até 30s entre as
// tentativas, com jitter, sem limite de tentativas; a espera volta ao
// início quando uma conexão abre e sincroniza. Usado por
// hooks/useChannelChat.ts e hooks/useForumChannel.ts.
//
// - connect() abre um WebSocket novo. É chamado a cada tentativa, então
//   deve usar o token de acesso mais recente (pode ter sido renovado).
// - sync() recarrega pela REST o que pode ter se perdido enquanto a conexão
//   estava fora. Roda depois de cada abertura (inclusive a primeira), com o
//   socket já entregando frames, para não sobrar janela entre o snapshot e
//   o tempo real; quem chama mescla por id.
// - Se a conexão cai sem nunca ter aberto, sync() também serve de sonda: o
//   navegador não expõe o status HTTP de um upgrade recusado (só um close
//   1006), mas a REST do mesmo canal responde o mesmo 401/403/404. Nesses
//   casos retry não resolve: o job para e reporta 'error'.
// - cancel() fecha o socket e mata o job sem reconectar (unmount, troca de
//   canal).
export type ChannelConnectionStatus = 'loading' | 'open' | 'reconnecting' | 'error'

export interface ReconnectingSocket {
  // O socket atual, só se estiver aberto (para enviar frames).
  current: () => WebSocket | null
  cancel: () => void
}

const RECONNECT_BASE_MS = 1_000
const RECONNECT_MAX_MS = 30_000

// Status HTTP que não mudam tentando de novo: canal que não é de texto/forum
// (400), token recusado (401), sem ViewChannels ou fora do servidor (403),
// canal apagado (404).
const PERMANENT_HTTP_STATUS = new Set([400, 401, 403, 404])

// Espera antes da tentativa `failures` (1, 2, …): metade fixa e metade
// aleatória do teto 1s·2^(n-1), limitado a 30s, para os clients não
// voltarem todos no mesmo instante depois de uma troca de versão.
export function reconnectDelay(failures: number, random: () => number = Math.random): number {
  const ceiling = Math.min(RECONNECT_BASE_MS * 2 ** (failures - 1), RECONNECT_MAX_MS)
  return ceiling / 2 + (random() * ceiling) / 2
}

export function isPermanentConnectionError(err: unknown): boolean {
  return err instanceof ApiError && PERMANENT_HTTP_STATUS.has(err.status)
}

export function createReconnectingSocket(options: {
  // Só para os logs de console.
  label: string
  // Texto da falha definitiva sem um Error por trás, traduzido a cada
  // leitura (LocalizedError) para acompanhar a troca de idioma.
  failureMessage: () => string
  connect: () => WebSocket
  sync: () => Promise<void>
  onMessage: (data: string) => void
  onStatus: (status: ChannelConnectionStatus, error?: Error) => void
}): ReconnectingSocket {
  let socket: WebSocket | null = null
  let done = false
  let failures = 0
  let timer: ReturnType<typeof setTimeout> | undefined

  function stop(err: unknown) {
    done = true
    socket?.close()
    socket = null
    options.onStatus('error', err instanceof Error ? err : new LocalizedError(options.failureMessage))
  }

  function scheduleRetry() {
    if (done) return
    failures += 1
    const delay = reconnectDelay(failures)
    console.warn(
      `ffcom: conexão de ${options.label} fora, nova tentativa em ${Math.round(delay)}ms (tentativa ${failures})`,
    )
    options.onStatus('reconnecting')
    timer = setTimeout(attempt, delay)
  }

  function attempt() {
    timer = undefined
    if (done) return
    const ws = options.connect()
    socket = ws
    let opened = false

    ws.onmessage = (event) => {
      if (!done && socket === ws) options.onMessage(String(event.data))
    }
    ws.onopen = () => {
      if (done || socket !== ws) return
      opened = true
      options.sync().then(
        () => {
          if (done || socket !== ws || ws.readyState !== WebSocket.OPEN) return
          failures = 0
          options.onStatus('open')
        },
        (err) => {
          if (done || socket !== ws) return
          if (isPermanentConnectionError(err)) {
            stop(err)
            return
          }
          console.warn(`ffcom: falha ao recarregar ${options.label} depois de conectar`, err)
          // Derruba o socket para o onclose cair no retry: sem o snapshot
          // não dá para garantir que nada se perdeu.
          ws.close()
        },
      )
    }
    // onclose sempre vem depois de onerror; a decisão fica toda lá.
    ws.onerror = () => {}
    ws.onclose = () => {
      if (done || socket !== ws) return
      socket = null
      if (opened) {
        scheduleRetry()
        return
      }
      options.sync().then(scheduleRetry, (err) => {
        if (done) return
        if (isPermanentConnectionError(err)) stop(err)
        else scheduleRetry()
      })
    }
  }

  attempt()

  return {
    current() {
      return socket && socket.readyState === WebSocket.OPEN ? socket : null
    },
    cancel() {
      done = true
      clearTimeout(timer)
      socket?.close()
      socket = null
    },
  }
}
