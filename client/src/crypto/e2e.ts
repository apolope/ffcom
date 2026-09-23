import nacl from 'tweetnacl'

// Criptografia ponta-a-ponta de DMs (NaCl box, X25519-XSalsa20-Poly1305) --
// ver docs/architecture.md, "Decisão: criptografia ponta-a-ponta em DMs".
// Cada dispositivo (perfil de navegador / instalação do Electron) tem seu
// próprio par de chaves por conta, gerado uma vez e guardado em localStorage
// (mesmo padrão já usado por oidc-client-ts em auth/userManager.ts) -- não há
// sincronização entre dispositivos nesta v1.
//
// A chave de localStorage leva o `sub` do OIDC: duas contas que logam no
// mesmo navegador têm pares separados, e sair não apaga nada (o histórico de
// DM da conta continua legível quando ela voltar). Ver docs/architecture.md,
// "Decisão: chave de E2E e cursores de não lida por conta".

// Formato de antes da chave por conta: um par só para o navegador inteiro.
// Só é lido por adoptLegacyKeyPair, para migrar para a conta dona dele.
const LEGACY_STORAGE_KEY = 'ffcom.e2e.keypair.v1'

function storageKey(accountSub: string): string {
  return `ffcom.e2e.keypair.v2:${accountSub}`
}

export interface E2EKeyPair {
  publicKey: Uint8Array
  secretKey: Uint8Array
}

interface StoredKeyPair {
  publicKey: string
  secretKey: string
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function base64ToBytes(b64: string): Uint8Array {
  const binary = atob(b64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

function parseStoredKeyPair(raw: string | null): E2EKeyPair | null {
  if (!raw) return null
  try {
    const stored = JSON.parse(raw) as StoredKeyPair
    return {
      publicKey: base64ToBytes(stored.publicKey),
      secretKey: base64ToBytes(stored.secretKey),
    }
  } catch {
    // localStorage corrompido -- tratado como "sem par".
    return null
  }
}

function storeKeyPair(accountSub: string, keyPair: E2EKeyPair): void {
  const stored: StoredKeyPair = {
    publicKey: bytesToBase64(keyPair.publicKey),
    secretKey: bytesToBase64(keyPair.secretKey),
  }
  localStorage.setItem(storageKey(accountSub), JSON.stringify(stored))
}

// loadKeyPair lê o par de chaves desta conta neste dispositivo, ou null se
// ela ainda não tiver um aqui.
export function loadKeyPair(accountSub: string): E2EKeyPair | null {
  return parseStoredKeyPair(localStorage.getItem(storageKey(accountSub)))
}

export function createKeyPair(accountSub: string): E2EKeyPair {
  const keyPair = nacl.box.keyPair()
  storeKeyPair(accountSub, keyPair)
  return keyPair
}

// legacyPublicKeyBase64 devolve a chave pública do par antigo (sem conta),
// se este navegador ainda tiver um.
export function legacyPublicKeyBase64(): string | null {
  const legacy = parseStoredKeyPair(localStorage.getItem(LEGACY_STORAGE_KEY))
  return legacy ? bytesToBase64(legacy.publicKey) : null
}

// adoptLegacyKeyPair move o par antigo para esta conta. Quem chama tem que
// ter conferido antes que a chave pública dele é a que a conta publicou em
// server-central (hooks/useE2EKeys.ts) -- senão uma conta herdaria a chave
// privada de outra pessoa que usou o mesmo navegador.
export function adoptLegacyKeyPair(accountSub: string): E2EKeyPair | null {
  const legacy = parseStoredKeyPair(localStorage.getItem(LEGACY_STORAGE_KEY))
  if (!legacy) return null
  storeKeyPair(accountSub, legacy)
  localStorage.removeItem(LEGACY_STORAGE_KEY)
  return legacy
}

export function publicKeyToBase64(keyPair: E2EKeyPair): string {
  return bytesToBase64(keyPair.publicKey)
}

export interface EncryptedDM {
  ciphertext: string
  nonce: string
}

// encryptDM cifra plaintext para peerPublicKeyB64, assinado implicitamente
// pela chave privada local (autenticação do NaCl box). Nonce novo e
// aleatório a cada mensagem.
export function encryptDM(plaintext: string, peerPublicKeyB64: string, mySecretKey: Uint8Array): EncryptedDM {
  const nonce = nacl.randomBytes(nacl.box.nonceLength)
  const message = new TextEncoder().encode(plaintext)
  const box = nacl.box(message, nonce, base64ToBytes(peerPublicKeyB64), mySecretKey)
  return { ciphertext: bytesToBase64(box), nonce: bytesToBase64(nonce) }
}

// decryptDM decifra usando a chave pública ATUAL do outro lado da conversa
// (não uma chave "pinada" por mensagem -- ver limitação de multi-dispositivo
// em docs/architecture.md). Devolve null se não for possível decifrar (ex.:
// mensagem cifrada para uma chave antiga, de antes de uma rotação).
export function decryptDM(ciphertextB64: string, nonceB64: string, peerPublicKeyB64: string, mySecretKey: Uint8Array): string | null {
  try {
    const opened = nacl.box.open(
      base64ToBytes(ciphertextB64),
      base64ToBytes(nonceB64),
      base64ToBytes(peerPublicKeyB64),
      mySecretKey,
    )
    if (!opened) return null
    return new TextDecoder().decode(opened)
  } catch {
    return null
  }
}
