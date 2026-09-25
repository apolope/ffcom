import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  addKnownServer,
  fetchKnownServers,
  removeKnownServer,
  reorderKnownServers,
  type RemoteKnownServer,
} from '../lib/serverCentralApi'
import { joinServer } from '../lib/serverChannelApi'
import { createRetryingSaver, type RetryingSaver } from '../lib/retryingSaver'
import type { KnownServer } from '../types'

export type KnownServersStatus = 'loading' | 'ready' | 'error'

interface UseKnownServersResult {
  servers: KnownServer[]
  status: KnownServersStatus
  error: string | undefined
  addServer: (address: string, name: string, inviteCode?: string) => Promise<void>
  removeServer: (id: string) => Promise<void>
  // Aplica na tela e grava (tentando até conseguir) a ordem nova do rail.
  saveOrder: (ids: string[]) => void
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
//
// onSaveError: primeira falha ao gravar a ordem do rail (ver saveOrder).
export function useKnownServers(accessToken: string, onSaveError?: () => void): UseKnownServersResult {
  const [loaded, setLoaded] = useState<KnownServer[]>([])
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
        setLoaded(remote.map(toKnownServer))
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

  // Ordem arrastada ainda não gravada: fica por cima da lista do servidor
  // até o job (lib/retryingSaver.ts) conseguir gravar. Servidor que não
  // estava na ordem arrastada (adicionado em outra aba) vai para o topo,
  // onde o server-central põe servidor novo.
  const [pendingOrder, setPendingOrder] = useState<string[]>()
  const onSaveErrorRef = useRef(onSaveError)
  useEffect(() => {
    onSaveErrorRef.current = onSaveError
  }, [onSaveError])
  const saverRef = useRef<RetryingSaver<string[]>>(undefined)

  // Um job por sessão; ids pendentes de uma sessão anterior não casam com
  // os da nova e são ignorados pela ordenação abaixo.
  useEffect(() => {
    if (!accessToken) return
    const saver = createRetryingSaver({
      label: 'a ordem dos servidores',
      save: async (ids: string[]) => {
        const current = (await fetchKnownServers(accessToken)).map((s) => s.id)
        const ordered = ids.filter((id) => current.includes(id))
        const added = current.filter((id) => !ordered.includes(id))
        await reorderKnownServers(accessToken, [...added, ...ordered])
        return (await fetchKnownServers(accessToken)).map(toKnownServer)
      },
      onSaved: (_ids, saved) => {
        setLoaded(saved)
        setPendingOrder(undefined)
      },
      onFirstFailure: () => onSaveErrorRef.current?.(),
    })
    saverRef.current = saver
    return () => saver.cancel()
  }, [accessToken])

  const saveOrder = useCallback((ids: string[]) => {
    setPendingOrder(ids)
    saverRef.current?.schedule(ids)
  }, [])

  const servers = useMemo(() => {
    if (!pendingOrder) return loaded
    const rank = new Map(pendingOrder.map((id, index) => [id, index]))
    return [...loaded].sort((a, b) => (rank.get(a.id) ?? -1) - (rank.get(b.id) ?? -1))
  }, [loaded, pendingOrder])

  return { servers, status, error, addServer, removeServer, saveOrder }
}
