import { useEffect, useState } from 'react'
import { loadOrCreateKeyPair, publicKeyToBase64, type E2EKeyPair } from '../crypto/e2e'
import { setMyE2EPublicKey } from '../lib/serverCentralApi'

// Flag em localStorage para não republicar a mesma chave a cada carregamento
// do app -- só falha silenciosamente se a publicação anterior não tiver ido
// pra frente (ex.: primeira vez offline); tenta de novo no próximo load.
const PUBLISHED_FLAG_KEY = 'ffcom.e2e.publishedKey.v1' // gitleaks:allow (nome de chave de localStorage, não segredo)

// Garante que este dispositivo tenha um par de chaves de E2E (ver
// crypto/e2e.ts) e que a chave pública esteja publicada em server-central
// assim que houver accessToken -- ver docs/architecture.md, "Decisão:
// criptografia ponta-a-ponta em DMs". Instanciado uma vez em App.tsx, não
// adiado até abrir uma DM, para que a chave já esteja disponível quando um
// amigo for conversar.
export function useE2EKeys(accessToken: string): { keyPair: E2EKeyPair } {
  const [keyPair] = useState<E2EKeyPair>(() => loadOrCreateKeyPair())

  useEffect(() => {
    if (!accessToken) return
    const publicKeyB64 = publicKeyToBase64(keyPair)
    if (localStorage.getItem(PUBLISHED_FLAG_KEY) === publicKeyB64) return

    let cancelled = false
    setMyE2EPublicKey(accessToken, publicKeyB64)
      .then(() => {
        if (!cancelled) localStorage.setItem(PUBLISHED_FLAG_KEY, publicKeyB64)
      })
      .catch((err) => {
        console.error('ffcom: falha ao publicar chave pública de E2E', err)
      })

    return () => {
      cancelled = true
    }
  }, [accessToken, keyPair])

  return { keyPair }
}
