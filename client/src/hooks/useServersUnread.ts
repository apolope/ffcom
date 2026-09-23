import { useEffect, useMemo, useState } from 'react'
import { fetchChannels } from '../lib/serverChannelApi'
import { unreadIds, type UnreadEntry } from '../lib/unread'
import type { KnownServer } from '../types'

// Intervalo do poll dos servidores que não estão abertos. Mais longo que o
// do servidor aberto (STRUCTURE_POLL_INTERVAL_MS em useServerStructure, 20s)
// porque aqui são N servidores e a bolinha no ServerRail só diz "tem coisa
// nova lá", não precisa chegar antes de a pessoa trocar de servidor.
const SERVERS_UNREAD_POLL_INTERVAL_MS = 60_000

// Ids dos servidores conhecidos, fora o aberto, com pelo menos um canal não
// lido -- indicador agregado do ServerRail (ver docs/architecture.md,
// "Decisão: indicador de não lida"). Busca só GET /api/channels de cada um
// (lastMessageAt por canal, já filtrado por ViewChannels no servidor), não a
// estrutura inteira. O servidor aberto fica de fora: App.tsx já tem o
// unreadChannelIds dele via useServerStructure, sem pedir de novo.
//
// Um servidor que falha (fora do ar, membro removido) só fica sem bolinha;
// não há estado de erro, porque o rail não tem onde mostrar.
export function useServersUnread(
  servers: KnownServer[],
  openServerId: string | undefined,
  accessToken: string,
  accountSub: string,
): Set<string> {
  const [channelsByServer, setChannelsByServer] = useState<Map<string, UnreadEntry[]>>(new Map())

  // useKnownServers só recria `servers` ao carregar ou adicionar um
  // servidor, então a identidade é estável entre renders.
  const background = useMemo(() => servers.filter((s) => s.id !== openServerId), [servers, openServerId])

  useEffect(() => {
    if (!accessToken || background.length === 0) return
    let cancelled = false

    function load() {
      void Promise.all(
        background.map((server) =>
          fetchChannels(server.baseUrl, accessToken)
            .then((channels) => [server.id, channels.map((c) => ({ id: c.id, lastMessageAt: c.lastMessageAt }))] as const)
            .catch(() => undefined),
        ),
      ).then((results) => {
        if (cancelled) return
        const next = new Map<string, UnreadEntry[]>()
        for (const result of results) {
          if (result) next.set(result[0], result[1])
        }
        setChannelsByServer(next)
      })
    }

    load()
    const interval = setInterval(load, SERVERS_UNREAD_POLL_INTERVAL_MS)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [accessToken, background])

  // Recalcula também quando `background` muda (troca de servidor): os
  // cursores em localStorage mudaram enquanto o servidor anterior estava
  // aberto (App.tsx marca como lido ao entrar e sair de cada canal). Filtrar
  // por `background` descarta o que sobrou de um servidor que agora está
  // aberto ou foi removido, até a próxima carga substituir o mapa.
  return useMemo(() => {
    const unread = new Set<string>()
    for (const server of background) {
      const entries = channelsByServer.get(server.id)
      if (entries && unreadIds(accountSub, 'channel', entries, undefined).size > 0) unread.add(server.id)
    }
    return unread
  }, [channelsByServer, background, accountSub])
}
