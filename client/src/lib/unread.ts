// Cursor "última leitura" do indicador de não lida (ver
// docs/architecture.md, "Decisão: indicador de não lida"), guardado local ao
// dispositivo em localStorage -- mesmo padrão de risco/persistência já
// aceito para a sessão OIDC (auth/userManager.ts) e as chaves de E2E
// (crypto/e2e.ts): não sincroniza entre dispositivos, sobrevive a reload.
// Por conta (`sub` do OIDC), como o par de chaves de E2E: sem isso quem logava
// depois de outra pessoa no mesmo navegador herdava o "lido" dela.
const PREFIX = 'ffcom:lastRead:v2:'

export type UnreadKind = 'channel' | 'dm'

function storageKey(accountSub: string, kind: UnreadKind, id: string): string {
  return `${PREFIX}${accountSub}:${kind}:${id}`
}

export function getLastRead(accountSub: string, kind: UnreadKind, id: string): string | undefined {
  try {
    return localStorage.getItem(storageKey(accountSub, kind, id)) ?? undefined
  } catch {
    return undefined
  }
}

export function markRead(accountSub: string, kind: UnreadKind, id: string, at: string = new Date().toISOString()): void {
  if (!accountSub) return
  try {
    localStorage.setItem(storageKey(accountSub, kind, id), at)
  } catch {
    // localStorage indisponível (aba anônima, quota, etc.) -- o indicador de
    // não lida simplesmente não persiste, sem outro efeito.
  }
}
