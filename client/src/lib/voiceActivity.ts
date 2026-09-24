// Se esta página está conectada a um canal de voz agora. Estar numa chamada
// conta como atividade para o "ausente" automático (hooks/useIdle.ts), porque
// dá para passar muito tempo conversando sem tocar no teclado nem no mouse.
// Módulo em vez de contexto: quem escreve é o hook de voz e quem lê é um
// timer, nenhum dos dois precisa renderizar com a mudança.
let connections = 0

export function setVoiceConnected(connected: boolean): void {
  connections = Math.max(0, connections + (connected ? 1 : -1))
}

export function isVoiceConnected(): boolean {
  return connections > 0
}
