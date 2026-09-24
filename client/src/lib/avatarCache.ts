// Cache de avatares da sessão: com avatar em cada linha de membro, a mesma
// imagem apareceria em várias listas ao mesmo tempo, e cada UserAvatar
// baixaria o arquivo de novo. Guarda uma object URL por avatarUrl, sem
// revogar enquanto a página vive (são poucas imagens pequenas).
//
// avatarUrl é fixa por conta (/api/avatars/{accountId}), então trocar o
// próprio avatar precisa invalidar a entrada (hooks/useMyProfile.ts).
// Avatares de outras pessoas trocados durante a sessão só atualizam ao
// recarregar a página.
import { fetchAvatarBlob } from './serverCentralApi'

const cache = new Map<string, Promise<string>>()
const listeners = new Set<() => void>()
let version = 0

export function loadAvatar(avatarUrl: string, accessToken: string): Promise<string> {
  let url = cache.get(avatarUrl)
  if (!url) {
    url = fetchAvatarBlob(avatarUrl, accessToken).then((blob) => URL.createObjectURL(blob))
    // Falha não fica em cache: a próxima renderização tenta de novo.
    url.catch(() => cache.delete(avatarUrl))
    cache.set(avatarUrl, url)
  }
  return url
}

export function invalidateAvatar(avatarUrl: string): void {
  const url = cache.get(avatarUrl)
  cache.delete(avatarUrl)
  url?.then((u) => URL.revokeObjectURL(u)).catch(() => {})
  version++
  listeners.forEach((listener) => listener())
}

// Para useSyncExternalStore: muda a cada invalidação, para os UserAvatar
// buscarem de novo.
export function subscribeAvatarCache(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function avatarCacheVersion(): number {
  return version
}
