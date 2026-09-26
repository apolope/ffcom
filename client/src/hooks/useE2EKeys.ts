import { useCallback, useEffect, useState } from 'react'
import {
  adoptLegacyKeyPair,
  generateKeyPair,
  keyPairFromSecretKey,
  legacyPublicKeyBase64,
  loadKeyPair,
  publicKeyToBase64,
  storeKeyPair,
  type E2EKeyPair,
} from '../crypto/e2e'
import { decryptKeyBackup, encryptKeyBackup } from '../crypto/keyBackup'
import {
  E2EKeyBackupConflictError,
  fetchMyE2EKeyBackup,
  fetchMyProfile,
  setMyE2EKeyBackup,
  type MyProfile,
} from '../lib/serverCentralApi'

// Estado da chave de E2E desta conta neste dispositivo:
// - needs-setup: a conta ainda não tem frase de recuperação; criar uma
//   publica a chave e sobe o backup (primeiro dispositivo, ou conta vinda de
//   antes do backup).
// - needs-unlock: a conta já tem backup, mas este dispositivo não tem a chave
//   (dispositivo novo, localStorage limpo, ou a chave foi trocada em outro
//   dispositivo por "esqueci a frase").
export type E2EKeyStatus = 'loading' | 'needs-setup' | 'needs-unlock' | 'ready' | 'error'

export interface E2EKeysResult {
  status: E2EKeyStatus
  keyPair: E2EKeyPair | null
  error: string | undefined
  // Em needs-setup: a chave publicada hoje é de outro dispositivo, que tem o
  // histórico. Criar a frase aqui troca a chave e deixa esse histórico
  // ilegível; o certo é criar a frase lá.
  publishedElsewhere: boolean
  setupPassphrase: (passphrase: string) => Promise<void>
  unlock: (passphrase: string) => Promise<void>
  resetWithNewPassphrase: (passphrase: string) => Promise<void>
  retry: () => void
}

interface State {
  accountSub: string
  status: E2EKeyStatus
  keyPair: E2EKeyPair | null
  error?: string
  publishedElsewhere: boolean
}

// localKeyPair devolve o par desta conta guardado neste dispositivo. Se o
// navegador ainda tiver o par de antes da chave por conta, ele só é adotado
// quando a chave pública bate com a que a conta publicou (ver
// docs/architecture.md, "Decisão: chave de E2E e cursores de não lida por
// conta"); senão fica onde está para a conta dona dele.
function localKeyPair(accountSub: string, profile: MyProfile): E2EKeyPair | null {
  const existing = loadKeyPair(accountSub)
  if (existing) return existing
  const legacyPub = legacyPublicKeyBase64()
  if (legacyPub && profile.e2ePublicKey === legacyPub) return adoptLegacyKeyPair(accountSub)
  return null
}

// resolveState decide o estado a partir do que server-central diz da conta
// e do que este dispositivo tem guardado.
function resolveState(accountSub: string, profile: MyProfile): State {
  if (profile.hasE2EKeyBackup === undefined) {
    throw new Error('server-central desatualizado: não informa o backup da chave de E2E')
  }
  const local = localKeyPair(accountSub, profile)
  const localMatches = !!local && publicKeyToBase64(local) === profile.e2ePublicKey
  if (profile.hasE2EKeyBackup) {
    return localMatches
      ? { accountSub, status: 'ready', keyPair: local, publishedElsewhere: false }
      : { accountSub, status: 'needs-unlock', keyPair: null, publishedElsewhere: false }
  }
  return {
    accountSub,
    status: 'needs-setup',
    keyPair: null,
    publishedElsewhere: !!profile.e2ePublicKey && !localMatches,
  }
}

// Chave de E2E da conta (ver crypto/e2e.ts e crypto/keyBackup.ts, e
// docs/architecture.md, "Decisão: backup da chave de E2E com frase de
// recuperação"). Instanciado uma vez em App.tsx. Resolve de novo a cada
// renovação de token, para um dispositivo aberto perceber quando a chave da
// conta foi trocada em outro ("esqueci a frase") e parar de cifrar com a
// antiga.
export function useE2EKeys(accessToken: string, accountSub: string): E2EKeysResult {
  const [state, setState] = useState<State>()
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    if (!accessToken || !accountSub) return

    let cancelled = false
    fetchMyProfile(accessToken)
      .then((profile) => {
        if (cancelled) return
        const next = resolveState(accountSub, profile)
        // Mantém a identidade do par entre renovações de token: DMs decifram
        // de novo sempre que keyPair muda (useDirectMessages.ts).
        setState((prev) =>
          prev?.accountSub === accountSub &&
          prev.status === 'ready' &&
          next.status === 'ready' &&
          prev.keyPair &&
          next.keyPair &&
          publicKeyToBase64(prev.keyPair) === publicKeyToBase64(next.keyPair)
            ? prev
            : next,
        )
      })
      .catch((err: unknown) => {
        if (cancelled) return
        console.error('ffcom: falha ao preparar chave de E2E', err)
        setState((prev) =>
          // Falha de rede numa renovação não derruba uma chave já pronta.
          prev?.accountSub === accountSub && prev.status === 'ready'
            ? prev
            : {
                accountSub,
                status: 'error',
                keyPair: null,
                error: err instanceof Error ? err.message : 'falha ao preparar a chave de criptografia',
                publishedElsewhere: false,
              },
        )
      })

    return () => {
      cancelled = true
    }
  }, [accessToken, accountSub, attempt])

  // publishNewBackup cifra o par com a frase, grava em server-central e só
  // então guarda neste dispositivo, para nunca existir aqui uma chave que a
  // conta não publicou.
  const publishNewBackup = useCallback(
    async (keyPair: E2EKeyPair, passphrase: string, replace: boolean) => {
      const backup = await encryptKeyBackup(keyPair.secretKey, passphrase)
      await setMyE2EKeyBackup(accessToken, { publicKey: publicKeyToBase64(keyPair), backup }, replace)
      storeKeyPair(accountSub, keyPair)
      setState({ accountSub, status: 'ready', keyPair, publishedElsewhere: false })
    },
    [accessToken, accountSub],
  )

  const setupPassphrase = useCallback(
    async (passphrase: string) => {
      const profile = await fetchMyProfile(accessToken)
      const local = localKeyPair(accountSub, profile)
      // Reaproveita o par deste dispositivo quando é o publicado, para o
      // histórico de DM continuar legível; senão começa um par novo.
      const keyPair = local && publicKeyToBase64(local) === profile.e2ePublicKey ? local : generateKeyPair()
      try {
        await publishNewBackup(keyPair, passphrase, false)
      } catch (err) {
        if (err instanceof E2EKeyBackupConflictError) {
          // Outro dispositivo criou a frase enquanto esta tela estava aberta.
          setState({ accountSub, status: 'needs-unlock', keyPair: null, publishedElsewhere: false })
          throw new Error('Esta conta já tem uma frase de recuperação, criada em outro dispositivo. Digite-a para continuar.')
        }
        throw err
      }
    },
    [accessToken, accountSub, publishNewBackup],
  )

  const unlock = useCallback(
    async (passphrase: string) => {
      const { publicKey, backup } = await fetchMyE2EKeyBackup(accessToken)
      const secretKey = await decryptKeyBackup(backup, passphrase)
      if (!secretKey) throw new Error('Frase de recuperação incorreta.')
      const keyPair = keyPairFromSecretKey(secretKey)
      // A chave decifrada tem que ser a que a conta publicou; se não for, o
      // backup e a chave pública estão dessincronizados no servidor.
      if (publicKeyToBase64(keyPair) !== publicKey) {
        throw new Error('O backup não corresponde à chave publicada da conta.')
      }
      storeKeyPair(accountSub, keyPair)
      setState({ accountSub, status: 'ready', keyPair, publishedElsewhere: false })
    },
    [accessToken, accountSub],
  )

  const resetWithNewPassphrase = useCallback(
    (passphrase: string) => publishNewBackup(generateKeyPair(), passphrase, true),
    [publishNewBackup],
  )

  const retry = useCallback(() => setAttempt((n) => n + 1), [])

  const current = state?.accountSub === accountSub ? state : undefined
  return {
    status: current?.status ?? 'loading',
    keyPair: current?.keyPair ?? null,
    error: current?.error,
    publishedElsewhere: current?.publishedElsewhere ?? false,
    setupPassphrase,
    unlock,
    resetWithNewPassphrase,
    retry,
  }
}
