import nacl from 'tweetnacl'
import { scryptAsync } from '@noble/hashes/scrypt.js'

// Backup da chave privada de E2E, cifrado com a frase de recuperação -- ver
// docs/architecture.md, "Decisão: backup da chave de E2E com frase de
// recuperação". A frase nunca sai do dispositivo: dela sai (scrypt) a chave
// simétrica que cifra a chave privada com NaCl secretbox, e só o resultado
// vai para server-central (PUT /api/me/e2e-key-backup). Quem tem o backup mas
// não a frase precisa de força bruta contra o scrypt.

// Tamanho mínimo da frase. O backup fica no servidor, então uma frase curta
// cai em força bruta offline mesmo com scrypt.
export const MIN_PASSPHRASE_LENGTH = 12

// Parâmetros recomendados pela OWASP para scrypt (N=2^17, r=8, p=1: 128 MiB,
// ~1 s por tentativa). Vão gravados no backup para poderem subir no futuro
// sem invalidar backups antigos.
const SCRYPT_PARAMS = { N: 2 ** 17, r: 8, p: 1 }

interface StoredBackupV1 {
  v: 1
  kdf: 'scrypt'
  N: number
  r: number
  p: number
  salt: string
  nonce: string
  box: string
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

// NFKC para a mesma frase com acento dar a mesma chave em qualquer teclado ou
// sistema (é -> e + acento combinante em alguns), e trim para espaço solto
// nas pontas não virar "frase incorreta".
function normalizePassphrase(passphrase: string): string {
  return passphrase.normalize('NFKC').trim()
}

function deriveKey(passphrase: string, salt: Uint8Array, params: { N: number; r: number; p: number }): Promise<Uint8Array> {
  return scryptAsync(normalizePassphrase(passphrase), salt, { ...params, dkLen: nacl.secretbox.keyLength })
}

// encryptKeyBackup cifra a chave privada com a frase e devolve o backup em
// base64 (o formato que server-central guarda como bytes opacos).
export async function encryptKeyBackup(secretKey: Uint8Array, passphrase: string): Promise<string> {
  const salt = nacl.randomBytes(16)
  const nonce = nacl.randomBytes(nacl.secretbox.nonceLength)
  const key = await deriveKey(passphrase, salt, SCRYPT_PARAMS)
  const stored: StoredBackupV1 = {
    v: 1,
    kdf: 'scrypt',
    ...SCRYPT_PARAMS,
    salt: bytesToBase64(salt),
    nonce: bytesToBase64(nonce),
    box: bytesToBase64(nacl.secretbox(secretKey, nonce, key)),
  }
  return bytesToBase64(new TextEncoder().encode(JSON.stringify(stored)))
}

// decryptKeyBackup devolve a chave privada, ou null se a frase estiver
// errada. Lança erro se o backup não estiver num formato conhecido.
export async function decryptKeyBackup(backupB64: string, passphrase: string): Promise<Uint8Array | null> {
  const stored = JSON.parse(new TextDecoder().decode(base64ToBytes(backupB64))) as StoredBackupV1
  // Teto no N para um backup adulterado não travar o dispositivo pedindo
  // gigabytes de memória.
  if (stored.v !== 1 || stored.kdf !== 'scrypt' || stored.N > 2 ** 20 || stored.r > 16 || stored.p > 4) {
    throw new Error('formato de backup de chave desconhecido; atualize o FFCom')
  }
  const key = await deriveKey(passphrase, base64ToBytes(stored.salt), { N: stored.N, r: stored.r, p: stored.p })
  return nacl.secretbox.open(base64ToBytes(stored.box), base64ToBytes(stored.nonce), key)
}
