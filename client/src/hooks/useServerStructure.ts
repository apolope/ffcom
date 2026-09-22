import { useEffect, useState } from 'react'
import { fetchCategories, fetchChannels, groupIntoCategories } from '../lib/serverChannelApi'
import type { Category } from '../types'

export type ServerStructureStatus = 'loading' | 'ready' | 'error'

interface UseServerStructureResult {
  categories: Category[]
  status: ServerStructureStatus
  error: string | undefined
}

// Intervalo de repolling da estrutura (categorias/canais). server-channel não
// tem um feed de atividade cruzando canais (o realtime.Hub é particionado
// por canal, ver docs/architecture.md, "Decisão: canal de texto em
// server-channel") -- então lastMessageAt de um canal fora do selecionado só
// atualiza revalidando esta lista periodicamente. Usado pelo indicador de
// não lida (ver hooks/useUnread.ts); não é tempo real, mas é suficiente para
// uma bolinha de "tem mensagem nova" (ver docs/architecture.md, "Decisão:
// indicador de não lida").
const STRUCTURE_POLL_INTERVAL_MS = 20_000

// Carrega categorias e canais reais de um server-channel, recarregando
// sempre que o servidor selecionado muda e periodicamente enquanto o
// servidor continua selecionado. Ver TODO.md ("API REST em server-channel
// para o client listar categorias/canais reais").
export function useServerStructure(
  serverBaseUrl: string,
  accessToken: string,
): UseServerStructureResult {
  const [categories, setCategories] = useState<Category[]>([])
  const [status, setStatus] = useState<ServerStructureStatus>('loading')
  const [error, setError] = useState<string>()

  useEffect(() => {
    let cancelled = false
    setCategories([])
    setStatus('loading')
    setError(undefined)

    function load(isFirstLoad: boolean) {
      return Promise.all([fetchCategories(serverBaseUrl, accessToken), fetchChannels(serverBaseUrl, accessToken)])
        .then(([remoteCategories, remoteChannels]) => {
          if (cancelled) return
          setCategories(groupIntoCategories(remoteCategories, remoteChannels))
          setStatus('ready')
        })
        .catch((err) => {
          if (cancelled || !isFirstLoad) return
          setStatus('error')
          setError(err instanceof Error ? err.message : 'falha ao carregar categorias/canais')
        })
    }

    void load(true)
    // Repolls silenciosos (erro não derruba o estado já carregado, só a
    // primeira carga vira tela de erro).
    const interval = setInterval(() => void load(false), STRUCTURE_POLL_INTERVAL_MS)

    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [serverBaseUrl, accessToken])

  return { categories, status, error }
}
