import { useEffect, useState } from 'react'
import { fetchCategories, fetchChannels, groupIntoCategories } from '../lib/serverChannelApi'
import type { Category } from '../types'

export type ServerStructureStatus = 'loading' | 'ready' | 'error'

interface UseServerStructureResult {
  categories: Category[]
  status: ServerStructureStatus
  error: string | undefined
}

// Carrega categorias e canais reais de um server-channel, recarregando
// sempre que o servidor selecionado muda. Ver TODO.md ("API REST em
// server-channel para o client listar categorias/canais reais").
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

    Promise.all([fetchCategories(serverBaseUrl, accessToken), fetchChannels(serverBaseUrl, accessToken)])
      .then(([remoteCategories, remoteChannels]) => {
        if (cancelled) return
        setCategories(groupIntoCategories(remoteCategories, remoteChannels))
        setStatus('ready')
      })
      .catch((err) => {
        if (cancelled) return
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar categorias/canais')
      })

    return () => {
      cancelled = true
    }
  }, [serverBaseUrl, accessToken])

  return { categories, status, error }
}
