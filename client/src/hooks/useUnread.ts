import { useMemo } from 'react'
import { unreadIds, type UnreadEntry, type UnreadKind } from '../lib/unread'

// Deriva o conjunto de ids (canais de um server-channel, ou contas de
// amigo/DM) com mensagem mais nova que o cursor local de "última leitura"
// (ver lib/unread.ts, unreadIds). activeId é excluído do resultado -- o que
// está aberto agora nunca aparece como não lido, ver docs/architecture.md,
// "Decisão: indicador de não lida".
export function useUnread(
  accountSub: string,
  kind: UnreadKind,
  entries: UnreadEntry[],
  activeId: string | undefined,
): Set<string> {
  const signature = entries.map((e) => `${e.id}:${e.lastMessageAt ?? ''}`).join('|')

  return useMemo(
    () => unreadIds(accountSub, kind, entries, activeId),
    // `entries` é recriado a cada render (categories.flatMap/friends.map em
    // App.tsx); usa `signature` como dependência estável em vez do array.
    [accountSub, kind, activeId, signature],
  )
}
