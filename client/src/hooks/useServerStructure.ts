import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  UNCATEGORIZED_ID,
  fetchCategories,
  fetchChannels,
  groupIntoCategories,
  reorderCategories,
  reorderChannels,
  type ChannelOrderGroup,
} from '../lib/serverChannelApi'
import { createRetryingSaver, type RetryingSaver } from '../lib/retryingSaver'
import type { Category } from '../types'

export type ServerStructureStatus = 'loading' | 'ready' | 'error'

// Ordem que a pessoa arrastou e o servidor ainda não confirmou. Fica
// aplicada por cima do que vem do servidor até ser gravada, para o poll não
// desfazer o arraste na tela enquanto o job tenta salvar.
interface PendingOrder {
  categoryIds?: string[]
  channelGroups?: ChannelOrderGroup[]
}

interface UseServerStructureResult {
  categories: Category[]
  status: ServerStructureStatus
  error: string | undefined
  // Recarrega na hora, sem esperar o próximo poll -- usado depois de
  // criar/editar/apagar categoria ou canal.
  refresh: () => void
  // Aplica na tela e agenda a gravação de uma ordem nova (arrastar e
  // soltar). Ver saverRef abaixo.
  saveCategoryOrder: (categoryIds: string[]) => void
  saveChannelOrder: (groups: ChannelOrderGroup[]) => void
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
//
// onSaveError é chamado na primeira falha de uma sequência de tentativas de
// gravar a ordem (não a cada nova tentativa), para avisar a pessoa uma vez.
export function useServerStructure(
  serverBaseUrl: string,
  accessToken: string,
  onSaveError?: () => void,
): UseServerStructureResult {
  const [loaded, setLoaded] = useState<Category[]>([])
  const [status, setStatus] = useState<ServerStructureStatus>('loading')
  const [error, setError] = useState<string>()
  const [refreshToken, setRefreshToken] = useState(0)
  const refresh = useCallback(() => setRefreshToken((n) => n + 1), [])

  const [pending, setPending] = useState<PendingOrder>({})
  const pendingRef = useRef<PendingOrder>({})
  const onSaveErrorRef = useRef(onSaveError)
  useEffect(() => {
    onSaveErrorRef.current = onSaveError
  }, [onSaveError])

  useEffect(() => {
    // Antes da lista de servidores chegar, serverBaseUrl vem vazio e o fetch
    // relativo cairia no próprio host do client.
    if (!serverBaseUrl || !accessToken) return
    let cancelled = false

    function load(isFirstLoad: boolean) {
      return Promise.all([fetchCategories(serverBaseUrl, accessToken), fetchChannels(serverBaseUrl, accessToken)])
        .then(([remoteCategories, remoteChannels]) => {
          if (cancelled) return
          setLoaded(groupIntoCategories(remoteCategories, remoteChannels))
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
  }, [serverBaseUrl, accessToken, refreshToken])

  // Job que grava a ordem arrastada até conseguir (lib/retryingSaver.ts),
  // um por servidor/sessão. Cada tentativa relê a estrutura do servidor e
  // ajusta a ordem a ela (categoria/canal apagado sai, categoria nova vai
  // para o fim), senão um 409 por lista desatualizada se repetiria para
  // sempre. Depois de gravar, relê de novo antes de largar a ordem pendente:
  // largando antes, a tela piscaria de volta à ordem do último poll.
  const saverRef = useRef<RetryingSaver<PendingOrder>>(undefined)

  // Só troca de servidor (ou de sessão) limpa a lista; um refresh() mantém
  // a estrutura atual na tela até a nova chegar. Também descarta a ordem
  // pendente: o job daquele servidor morre junto.
  useEffect(() => {
    setLoaded([])
    setStatus('loading')
    setError(undefined)
    pendingRef.current = {}
    setPending({})

    const saver = createRetryingSaver({
      label: 'a ordem das categorias e canais',
      save: async (order: PendingOrder) => {
        const [remoteCategories, remoteChannels] = await Promise.all([
          fetchCategories(serverBaseUrl, accessToken),
          fetchChannels(serverBaseUrl, accessToken),
        ])
        if (order.categoryIds) {
          const existing = new Set(remoteCategories.map((c) => c.id))
          const ordered = order.categoryIds.filter((id) => existing.has(id))
          const added = remoteCategories.map((c) => c.id).filter((id) => !ordered.includes(id))
          await reorderCategories(serverBaseUrl, accessToken, [...ordered, ...added])
        }
        if (order.channelGroups) {
          const categoryIds = new Set(remoteCategories.map((c) => c.id))
          const channelIds = new Set(remoteChannels.map((c) => c.id))
          const groups = order.channelGroups
            .filter((g) => g.categoryId === null || categoryIds.has(g.categoryId))
            .map((g) => ({ ...g, ids: g.ids.filter((id) => channelIds.has(id)) }))
          if (groups.length > 0) await reorderChannels(serverBaseUrl, accessToken, groups)
        }
        const [savedCategories, savedChannels] = await Promise.all([
          fetchCategories(serverBaseUrl, accessToken),
          fetchChannels(serverBaseUrl, accessToken),
        ])
        return groupIntoCategories(savedCategories, savedChannels)
      },
      onSaved: (_order, saved) => {
        setLoaded(saved)
        pendingRef.current = {}
        setPending({})
      },
      onFirstFailure: () => onSaveErrorRef.current?.(),
    })
    saverRef.current = saver
    return () => saver.cancel()
  }, [serverBaseUrl, accessToken])

  const schedule = useCallback(
    (change: PendingOrder) => {
      const next: PendingOrder = { ...pendingRef.current }
      if (change.categoryIds) next.categoryIds = change.categoryIds
      if (change.channelGroups) {
        // Grupo novo da mesma categoria substitui o antigo; os demais ficam.
        const replaced = new Set(change.channelGroups.map((g) => g.categoryId))
        next.channelGroups = [
          ...(next.channelGroups ?? []).filter((g) => !replaced.has(g.categoryId)),
          ...change.channelGroups,
        ]
      }
      pendingRef.current = next
      setPending(next)
      saverRef.current?.schedule(next)
    },
    [],
  )

  const saveCategoryOrder = useCallback((categoryIds: string[]) => schedule({ categoryIds }), [schedule])
  const saveChannelOrder = useCallback((groups: ChannelOrderGroup[]) => schedule({ channelGroups: groups }), [schedule])

  const categories = useMemo(() => applyPendingOrder(loaded, pending), [loaded, pending])

  return { categories, status, error, refresh, saveCategoryOrder, saveChannelOrder }
}

// Aplica a ordem pendente sobre a estrutura vinda do servidor. Canal ou
// categoria que não existe mais é ignorado; o que surgiu depois do arraste
// fica depois do que foi ordenado.
function applyPendingOrder(categories: Category[], pending: PendingOrder): Category[] {
  let result = categories
  if (pending.channelGroups) {
    const channelById = new Map(result.flatMap((c) => c.channels.map((ch) => [ch.id, ch] as const)))
    const grouped = new Set(pending.channelGroups.flatMap((g) => g.ids))
    const groupByCategory = new Map(pending.channelGroups.map((g) => [g.categoryId ?? UNCATEGORIZED_ID, g]))
    result = result.map((category) => {
      const rest = category.channels.filter((ch) => !grouped.has(ch.id))
      const group = groupByCategory.get(category.id)
      if (!group) return rest.length === category.channels.length ? category : { ...category, channels: rest }
      const ordered = group.ids.map((id) => channelById.get(id)).filter((ch) => ch !== undefined)
      return { ...category, channels: [...ordered, ...rest] }
    })
    // A sintética "Canais" só existe enquanto tiver canal.
    result = result.filter((c) => c.id !== UNCATEGORIZED_ID || c.channels.length > 0)
    // Canal arrastado para "sem categoria" quando ela ainda não existia.
    const uncategorized = groupByCategory.get(UNCATEGORIZED_ID)
    if (uncategorized && !result.some((c) => c.id === UNCATEGORIZED_ID)) {
      const channels = uncategorized.ids.map((id) => channelById.get(id)).filter((ch) => ch !== undefined)
      if (channels.length > 0) result = [{ id: UNCATEGORIZED_ID, name: 'Canais', channels }, ...result]
    }
  }
  if (pending.categoryIds) {
    const rank = new Map(pending.categoryIds.map((id, index) => [id, index]))
    // Sort estável: a sintética "Canais" (-1) fica no topo e categoria que
    // não estava na ordem arrastada vai para o fim.
    result = [...result].sort(
      (a, b) =>
        (a.id === UNCATEGORIZED_ID ? -1 : (rank.get(a.id) ?? Infinity)) -
        (b.id === UNCATEGORIZED_ID ? -1 : (rank.get(b.id) ?? Infinity)),
    )
  }
  return result
}
