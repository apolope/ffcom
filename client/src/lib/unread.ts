// Cursor "última leitura" do indicador de não lida (ver
// docs/architecture.md, "Decisão: indicador de não lida"), guardado local ao
// dispositivo em localStorage -- mesmo padrão de risco/persistência já
// aceito para a sessão OIDC (auth/userManager.ts) e as chaves de E2E
// (crypto/e2e.ts): não sincroniza entre dispositivos, sobrevive a reload.
const PREFIX = 'ffcom:lastRead:'

export type UnreadKind = 'channel' | 'dm'

function storageKey(kind: UnreadKind, id: string): string {
  return `${PREFIX}${kind}:${id}`
}

export function getLastRead(kind: UnreadKind, id: string): string | undefined {
  try {
    return localStorage.getItem(storageKey(kind, id)) ?? undefined
  } catch {
    return undefined
  }
}

export function markRead(kind: UnreadKind, id: string, at: string = new Date().toISOString()): void {
  try {
    localStorage.setItem(storageKey(kind, id), at)
  } catch {
    // localStorage indisponível (aba anônima, quota, etc.) -- o indicador de
    // não lida simplesmente não persiste, sem outro efeito.
  }
}
