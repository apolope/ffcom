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

export interface UnreadEntry {
  id: string
  lastMessageAt: string | undefined
}

// Ids de `entries` com mensagem mais nova que o cursor local. activeId é
// excluído (o que está aberto nunca conta como não lido). Uma entrada sem
// cursor ainda (primeira vez que este dispositivo vê esse id -- rollout da
// feature, ou servidor/amigo recém-adicionado) é semeada como já lida em vez
// de contar como atrasada, para não acender bolinha em tudo que já existia.
// Compartilhado por hooks/useUnread.ts (servidor aberto, DMs) e
// hooks/useServersUnread.ts (demais servidores, agregado no ServerRail).
export function unreadIds(
  accountSub: string,
  kind: UnreadKind,
  entries: UnreadEntry[],
  activeId: string | undefined,
): Set<string> {
  const unread = new Set<string>()
  if (!accountSub) return unread
  for (const entry of entries) {
    if (!entry.lastMessageAt || entry.id === activeId) continue
    const lastRead = getLastRead(accountSub, kind, entry.id)
    if (!lastRead) {
      markRead(accountSub, kind, entry.id, entry.lastMessageAt)
      continue
    }
    if (new Date(entry.lastMessageAt).getTime() > new Date(lastRead).getTime()) {
      unread.add(entry.id)
    }
  }
  return unread
}
