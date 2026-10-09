// Política da reconexão automática da chamada (hooks/useVoiceChannel.ts). Uma
// queda curta de rede não pode tirar a pessoa da sala: o livekit-client já
// tenta retomar sozinho (Reconnecting), e quando ele desiste (Disconnected
// sem ter sido pedido) o hook entra de novo no mesmo canal, com espera
// crescente, por até REJOIN_WINDOW_MS. Fica aqui só a parte pura, sem
// React nem LiveKit, para testar com `node --test` (voiceReconnect.test.ts).
// Ver docs/architecture.md, "Decisão: chamada em segundo plano no Android
// (fase 5)".

// Espera antes de cada nova tentativa de entrar; depois da última, repete a
// última.
export const REJOIN_DELAYS_MS = [1_000, 2_000, 5_000, 10_000]

// Quanto tempo, desde a queda, continua tentando antes de desistir e mostrar
// o erro com "Tentar de novo".
export const REJOIN_WINDOW_MS = 120_000

// Sala parada em Reconnecting por mais que isso: desiste da retomada do
// livekit-client e entra de novo do zero. O servidor já tira quem ficou ~20 s
// sem conexão, e a retomada do livekit-client, sem sucesso, leva uns 45 s
// para desistir sozinha.
export const RECONNECTING_WATCHDOG_MS = 30_000

// DisconnectReason do protocolo do LiveKit (livekit_models.proto). Números de
// fio do protobuf, que não mudam; repetidos aqui para este módulo não depender
// do livekit-client. Só estes motivos são falha de conexão; os outros são
// alguém (a própria pessoa, outra aba com a mesma conta, um moderador, o fim
// da sala) encerrando a participação, e entrar de novo brigaria com isso.
const REJOIN_REASONS: ReadonlySet<number> = new Set([
  0, // UNKNOWN_REASON
  3, // SERVER_SHUTDOWN (LiveKit reiniciando)
  6, // STATE_MISMATCH
  7, // JOIN_FAILURE
  8, // MIGRATION
  9, // SIGNAL_CLOSE
  14, // CONNECTION_TIMEOUT
  15, // MEDIA_FAILURE
])
// Fora da lista, entre outros: 1 CLIENT_INITIATED (sair, trocar de canal,
// ser movido), 2 DUPLICATE_IDENTITY (a mesma conta entrou em outro
// aparelho), 4 PARTICIPANT_REMOVED (expulso), 5 ROOM_DELETED, 10 ROOM_CLOSED.

// Se um Disconnected que o próprio app não pediu deve virar reconexão.
// undefined é o livekit-client desistindo de reconectar ("giving up"), que é
// exatamente o caso de uma queda de rede longa.
export function shouldRejoin(reason: number | undefined): boolean {
  return reason === undefined || REJOIN_REASONS.has(reason)
}

// Espera antes da tentativa `attempt` (0 é a primeira), ou undefined se já
// passou da janela desde a queda (`elapsedMs`) e é hora de desistir.
export function rejoinDelay(attempt: number, elapsedMs: number): number | undefined {
  if (elapsedMs >= REJOIN_WINDOW_MS) return undefined
  const delay = REJOIN_DELAYS_MS[Math.min(attempt, REJOIN_DELAYS_MS.length - 1)]
  return Math.min(delay, REJOIN_WINDOW_MS - elapsedMs)
}

// Erro ao pedir o token de novo que não adianta repetir: sem permissão de
// entrar no canal (403) ou canal apagado (404). Falta de rede, 5xx e 401
// (token de acesso sendo renovado) continuam tentando.
export function isFinalRejoinError(status: number | undefined): boolean {
  return status === 403 || status === 404
}
