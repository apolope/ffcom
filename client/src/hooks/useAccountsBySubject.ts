import { useEffect, useMemo, useState } from 'react'
import { lookupAccounts, type AccountSummary } from '../lib/serverCentralApi'

// Resultado por subject durante a sessão: null = consultado e sem conta em
// server-central (nunca logou lá), para não perguntar de novo a cada troca
// de servidor.
const cache = new Map<string, AccountSummary | null>()

// Liga membros de server-channel às contas de server-central pelo
// oidcSubject (POST /api/accounts/lookup), para mostrar o avatar de cada
// um. Só pede os subjects que ainda não estão no cache. Ver
// docs/architecture.md, "Decisão: status de presença e avatar nas listas de
// membros".
export function useAccountsBySubject(accessToken: string, subjects: string[]): Map<string, AccountSummary> {
  const [version, setVersion] = useState(0)
  // Chave estável para o efeito não rodar a cada render com o mesmo conjunto.
  const key = useMemo(() => [...new Set(subjects)].sort().join('\n'), [subjects])

  useEffect(() => {
    const missing = key ? key.split('\n').filter((s) => !cache.has(s)) : []
    if (!accessToken || missing.length === 0) return
    let cancelled = false
    lookupAccounts(accessToken, missing)
      .then((found) => {
        missing.forEach((s) => cache.set(s, null))
        found.forEach((a) => cache.set(a.oidcSubject, a))
        if (!cancelled) setVersion((v) => v + 1)
      })
      .catch(() => {
        /* sem avatar: as listas caem para a inicial do nome */
      })
    return () => {
      cancelled = true
    }
  }, [accessToken, key])

  return useMemo(() => {
    void version
    const out = new Map<string, AccountSummary>()
    for (const s of key ? key.split('\n') : []) {
      const account = cache.get(s)
      if (account) out.set(s, account)
    }
    return out
  }, [key, version])
}
