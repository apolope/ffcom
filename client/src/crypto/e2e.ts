import nacl from 'tweetnacl'

// Criptografia ponta-a-ponta de DMs (NaCl box, X25519-XSalsa20-Poly1305) --
// ver docs/architecture.md, "Decisão: criptografia ponta-a-ponta em DMs".
// Cada dispositivo (perfil de navegador / instalação do Electron) tem seu
// próprio par de chaves, gerado uma vez e guardado em localStorage (mesmo
// padrão já usado por oidc-client-ts em auth/userManager.ts) -- não há
// sincronização entre dispositivos nesta v1.

const STORAGE_KEY = 'ffcom.e2e.keypair.v1'

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

// loadOrCreateKeyPair lê o par de chaves deste dispositivo do localStorage,
// gerando um novo na primeira vez que a função roda aqui.
export function loadOrCreateKeyPair(): E2EKeyPair {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw) {
    try {
      const stored = JSON.parse(raw) as StoredKeyPair
      return {
        publicKey: base64ToBytes(stored.publicKey),
        secretKey: base64ToBytes(stored.secretKey),
      }
    } catch {
      // localStorage corrompido -- cai para gerar um par novo abaixo.
    }
  }

  const keyPair = nacl.box.keyPair()
  const stored: StoredKeyPair = {
    publicKey: bytesToBase64(keyPair.publicKey),
    secretKey: bytesToBase64(keyPair.secretKey),
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(stored))
  return keyPair
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
