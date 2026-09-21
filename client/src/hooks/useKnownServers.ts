import { useCallback, useEffect, useState } from 'react'
import {
  addKnownServer,
  fetchKnownServers,
  removeKnownServer,
  type RemoteKnownServer,
} from '../lib/serverCentralApi'
import { joinServer } from '../lib/serverChannelApi'
import type { KnownServer } from '../types'

export type KnownServersStatus = 'loading' | 'ready' | 'error'

interface UseKnownServersResult {
  servers: KnownServer[]
  status: KnownServersStatus
  error: string | undefined
  addServer: (address: string, name: string, inviteCode?: string) => Promise<void>
  removeServer: (id: string) => Promise<void>
}

function deriveInitials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 0) return '?'
  if (words.length === 1) return words[0].slice(0, 2).toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

function toKnownServer(remote: RemoteKnownServer): KnownServer {
  return {
    id: remote.id,
    name: remote.name,
    initials: deriveInitials(remote.name),
    baseUrl: remote.address,
  }
}

// Carrega e gerencia o diretório de server-channel conhecidos pela conta
// autenticada (server-central). Ver TODO.md ("API para o client listar/
// adicionar/remover servidores conhecidos").
export function useKnownServers(accessToken: string): UseKnownServersResult {
  const [servers, setServers] = useState<KnownServer[]>([])
  const [status, setStatus] = useState<KnownServersStatus>('loading')
  const [error, setError] = useState<string>()

  const load = useCallback(() => {
    // accessToken só existe depois que o login OIDC termina (ver
    // AuthProvider) — sem essa guarda, o primeiro render (ainda em
    // 'loading' no App) já dispara fetch com token vazio, gerando 401
    // visível no console antes do token real chegar.
    if (!accessToken) return Promise.resolve()
    setStatus('loading')
    setError(undefined)
    return fetchKnownServers(accessToken)
      .then((remote) => {
        setServers(remote.map(toKnownServer))
        setStatus('ready')
      })
      .catch((err) => {
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar servidores')
      })
  }, [accessToken])

  useEffect(() => {
    void load()
  }, [load])

  const addServer = useCallback(
    async (address: string, name: string, inviteCode?: string) => {
      // Entra no server-channel antes de registrar no diretório: se o
      // convite for inválido/ausente, o servidor nem chega a ser adicionado
      // (ver docs/architecture.md, "Convites obrigatórios para entrar em
      // server-channel").
      await joinServer(address, accessToken, inviteCode)
      await addKnownServer(accessToken, address, name)
      await load()
    },
    [accessToken, load],
  )

  const removeServer = useCallback(
    async (id: string) => {
      await removeKnownServer(accessToken, id)
      await load()
    },
    [accessToken, load],
  )

  return { servers, status, error, addServer, removeServer }
}
