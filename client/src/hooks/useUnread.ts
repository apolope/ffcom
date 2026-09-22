import { useMemo } from 'react'
import { getLastRead, markRead, type UnreadKind } from '../lib/unread'

interface UnreadEntry {
  id: string
  lastMessageAt: string | undefined
}

// Deriva o conjunto de ids (canais de um server-channel, ou contas de
// amigo/DM) com mensagem mais nova que o cursor local de "última leitura"
// (ver lib/unread.ts). activeId é excluído do resultado -- o que está aberto
// agora nunca aparece como não lido, ver docs/architecture.md, "Decisão:
// indicador de não lida".
//
// Uma entrada sem cursor ainda (primeira vez que este dispositivo vê esse
// id -- rollout desta feature, ou servidor/amigo recém-adicionado) é
// semeada como já lida em vez de contar como atrasada, para não acender uma
// bolinha em tudo que já existia antes desta feature existir.
export function useUnread(kind: UnreadKind, entries: UnreadEntry[], activeId: string | undefined): Set<string> {
  const signature = entries.map((e) => `${e.id}:${e.lastMessageAt ?? ''}`).join('|')

  return useMemo(() => {
    const unread = new Set<string>()
    for (const entry of entries) {
      if (!entry.lastMessageAt || entry.id === activeId) continue
      const lastRead = getLastRead(kind, entry.id)
      if (!lastRead) {
        markRead(kind, entry.id, entry.lastMessageAt)
        continue
      }
      if (new Date(entry.lastMessageAt).getTime() > new Date(lastRead).getTime()) {
        unread.add(entry.id)
      }
    }
    return unread
    // `entries` é recriado a cada render (categories.flatMap/friends.map em
    // App.tsx); usa `signature` como dependência estável em vez do array.
  }, [kind, activeId, signature])
}
