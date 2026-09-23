import { useEffect, useState } from 'react'
import {
  adoptLegacyKeyPair,
  createKeyPair,
  legacyPublicKeyBase64,
  loadKeyPair,
  publicKeyToBase64,
  type E2EKeyPair,
} from '../crypto/e2e'
import { fetchMyProfile, setMyE2EPublicKey } from '../lib/serverCentralApi'

// Flag em localStorage para não republicar a mesma chave a cada carregamento
// do app -- só falha silenciosamente se a publicação anterior não tiver ido
// pra frente (ex.: primeira vez offline); tenta de novo no próximo load. Por
// conta, como o par de chaves (ver crypto/e2e.ts): uma flag compartilhada
// fazia a segunda conta do navegador nunca publicar a própria chave.
function publishedFlagKey(accountSub: string): string {
  return `ffcom.e2e.publishedKey.v2:${accountSub}` // gitleaks:allow (nome de chave de localStorage, não segredo)
}

const LEGACY_PUBLISHED_FLAG_KEY = 'ffcom.e2e.publishedKey.v1' // gitleaks:allow (nome de chave de localStorage, não segredo)

// resolveKeyPair devolve o par desta conta neste dispositivo, criando um se
// preciso. Se o navegador ainda tiver o par de antes da chave por conta, ele
// só é adotado quando a chave pública bate com a que a conta já publicou em
// server-central; se não bater, fica onde está para a conta dona dele e esta
// conta ganha um par novo.
async function resolveKeyPair(accessToken: string, accountSub: string, isCancelled: () => boolean): Promise<E2EKeyPair | null> {
  const existing = loadKeyPair(accountSub)
  if (existing) return existing

  const legacyPub = legacyPublicKeyBase64()
  if (legacyPub) {
    const profile = await fetchMyProfile(accessToken)
    // undefined (não null) = server-central anterior ao campo: sem como
    // conferir de quem é o par antigo, então não decide nada agora e tenta de
    // novo na próxima renovação de token / carregamento.
    if (profile.e2ePublicKey === undefined) {
      throw new Error('server-central não devolve e2ePublicKey em GET /api/me; migração da chave antiga adiada')
    }
    if (isCancelled()) return null
    if (profile.e2ePublicKey === legacyPub) {
      const adopted = adoptLegacyKeyPair(accountSub)
      if (adopted) {
        localStorage.setItem(publishedFlagKey(accountSub), legacyPub)
        localStorage.removeItem(LEGACY_PUBLISHED_FLAG_KEY)
        return adopted
      }
    }
  }

  return loadKeyPair(accountSub) ?? createKeyPair(accountSub)
}

// Garante que esta conta tenha um par de chaves de E2E neste dispositivo
// (ver crypto/e2e.ts) e que a chave pública esteja publicada em
// server-central assim que houver accessToken -- ver docs/architecture.md,
// "Decisão: criptografia ponta-a-ponta em DMs". Instanciado uma vez em
// App.tsx, não adiado até abrir uma DM, para que a chave já esteja disponível
// quando um amigo for conversar. keyPair fica null até resolver.
export function useE2EKeys(accessToken: string, accountSub: string): { keyPair: E2EKeyPair | null } {
  const [state, setState] = useState<{ accountSub: string; keyPair: E2EKeyPair }>()

  useEffect(() => {
    if (!accessToken || !accountSub) return

    let cancelled = false
    async function run() {
      const keyPair = await resolveKeyPair(accessToken, accountSub, () => cancelled)
      if (cancelled || !keyPair) return
      // Mantém a identidade do par entre renovações de token: DMs decifram
      // de novo sempre que keyPair muda (useDirectMessages.ts).
      setState((prev) => (prev?.accountSub === accountSub ? prev : { accountSub, keyPair }))

      const publicKeyB64 = publicKeyToBase64(keyPair)
      if (localStorage.getItem(publishedFlagKey(accountSub)) === publicKeyB64) return
      await setMyE2EPublicKey(accessToken, publicKeyB64)
      if (!cancelled) localStorage.setItem(publishedFlagKey(accountSub), publicKeyB64)
    }

    run().catch((err) => {
      console.error('ffcom: falha ao preparar/publicar chave de E2E', err)
    })

    return () => {
      cancelled = true
    }
  }, [accessToken, accountSub])

  return { keyPair: state?.accountSub === accountSub ? state.keyPair : null }
}
