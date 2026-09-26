import { useCallback, useEffect, useState } from 'react'
import {
  decodePresenceSocketFrame,
  fetchDirectMessages,
  sendDirectMessageFrame,
  type RemoteDirectMessage,
} from '../lib/serverCentralApi'
import { decryptDM, encryptDM, type E2EKeyPair } from '../crypto/e2e'
import type { Friend } from '../types'

export type DirectMessagesStatus = 'loading' | 'ready' | 'error'

// content === null quando a mensagem não pôde ser decifrada neste
// dispositivo (ver docs/architecture.md, "Decisão: criptografia
// ponta-a-ponta em DMs" -- ex.: mensagem cifrada para uma chave antiga, de
// antes de uma troca de dispositivo).
export interface DecryptedDirectMessage extends Omit<RemoteDirectMessage, 'ciphertext' | 'nonce'> {
  content: string | null
}

interface UseDirectMessagesResult {
  messages: DecryptedDirectMessage[]
  status: DirectMessagesStatus
  error: string | undefined
  sendMessage: (content: string) => void
}

// Conversa de DM com um amigo (peer). Não abre conexão própria: recebe o
// WebSocket de presença já mantido por useFriends e anexa um listener extra
// nele (ver docs/architecture.md, "Decisão: DMs entregues no mesmo
// WebSocket de presença", e hooks/useFriends.ts). Cifra/decifra localmente
// com a chave do amigo (peer.e2ePublicKey) e a chave privada deste
// dispositivo (myKeyPair, ver hooks/useE2EKeys.ts) -- o servidor nunca vê o
// texto puro.
export function useDirectMessages(
  accessToken: string,
  peer: Friend | undefined,
  socket: WebSocket | null,
  myKeyPair: E2EKeyPair,
): UseDirectMessagesResult {
  const [messages, setMessages] = useState<DecryptedDirectMessage[]>([])
  const [status, setStatus] = useState<DirectMessagesStatus>('loading')
  const [error, setError] = useState<string>()

  const peerId = peer?.accountId
  const peerPublicKey = peer?.e2ePublicKey

  const decrypt = useCallback(
    (m: RemoteDirectMessage): DecryptedDirectMessage => {
      const { ciphertext, nonce, ...rest } = m
      const content = peerPublicKey ? decryptDM(ciphertext, nonce, peerPublicKey, myKeyPair.secretKey) : null
      return { ...rest, content }
    },
    [peerPublicKey, myKeyPair],
  )

  useEffect(() => {
    if (!peerId) return
    let cancelled = false
    setMessages([])
    setStatus('loading')
    setError(undefined)

    fetchDirectMessages(accessToken, peerId)
      .then((history) => {
        if (cancelled) return
        setMessages(history.map(decrypt))
        setStatus('ready')
      })
      .catch((err) => {
        if (cancelled) return
        setStatus('error')
        setError(err instanceof Error ? err.message : 'falha ao carregar conversa')
      })

    return () => {
      cancelled = true
    }
  }, [accessToken, peerId, decrypt])

  useEffect(() => {
    if (!socket || !peerId) return

    function handleMessage(event: MessageEvent) {
      const frame = decodePresenceSocketFrame(String(event.data))
      if (!frame) return
      if (frame.type === 'dm.created') {
        const m = frame.message
        if (m.senderId !== peerId && m.recipientId !== peerId) return
        setMessages((prev) => [...prev, decrypt(m)])
      } else if (frame.type === 'error') {
        setError(frame.error)
      }
    }

    socket.addEventListener('message', handleMessage)
    return () => socket.removeEventListener('message', handleMessage)
  }, [socket, peerId, decrypt])

  const sendMessage = useCallback(
    (content: string) => {
      if (!socket || socket.readyState !== WebSocket.OPEN || !peerId) return
      if (!peerPublicKey) {
        setError('amigo ainda não criou a frase de recuperação da criptografia')
        return
      }
      const { ciphertext, nonce } = encryptDM(content, peerPublicKey, myKeyPair.secretKey)
      sendDirectMessageFrame(socket, peerId, ciphertext, nonce)
    },
    [socket, peerId, peerPublicKey, myKeyPair],
  )

  return { messages, status, error, sendMessage }
}
