import { registerPlugin, type PluginListenerHandle } from '@capacitor/core'
import i18n from '../i18n'
import { isAndroidApp } from './platform'
import { ApiError } from './apiError'
import { createPushGrant, deletePushDevice, registerPushDevice } from './serverCentralApi'
import { deletePushGrant, setPushGrant } from './serverChannelApi'

// Notificações push no app Android (fase 6 do docs/android-runbook.md; ver
// docs/architecture.md, "Decisão: notificações push no app Android (fase 6,
// client)" e docs/protocol.md, "Notificações push"). O nativo
// (client/android/.../PushPlugin.java, "FfcomPush") dá a permissão, o token
// FCM e as notificações; aqui ficam o registro do aparelho no server-central,
// os grants por servidor e os textos traduzidos. Fora do app Android tudo é
// no-op, e hooks/useAndroidPush.ts nem liga.

export interface PushStatus {
  // Firebase configurado neste APK (google-services.json no build).
  available: boolean
  // Notificações do app ligadas no Android.
  enabled: boolean
  // A explicação do push já apareceu nesta instalação.
  asked: boolean
}

// Toque numa notificação.
export interface PushOpen {
  type: 'channel_message' | 'dm' | 'friend_request' | 'friend_accepted'
  serverAddress?: string
  channelId?: string
  accountId?: string
}

// O que está na tela: o nativo não notifica isso com o app na frente e tira
// a notificação dele.
export type PushActive = { serverAddress: string; channelId: string } | { dmAccountId: string } | undefined

// Contrato com o PushPlugin.java (@CapacitorPlugin(name = "FfcomPush")).
interface FfcomPushPlugin {
  getStatus(): Promise<PushStatus>
  requestPermission(): Promise<{ enabled: boolean }>
  dismissPermission(): Promise<void>
  getToken(): Promise<{ token: string }>
  deleteToken(): Promise<void>
  setTexts(texts: Record<string, string>): Promise<void>
  setActive(active: { serverAddress?: string; channelId?: string; dmAccountId?: string }): Promise<void>
  clearAll(): Promise<void>
  addListener(event: 'open', callback: (data: PushOpen) => void): Promise<PluginListenerHandle>
  addListener(event: 'token', callback: (data: { token: string }) => void): Promise<PluginListenerHandle>
}

let plugin: FfcomPushPlugin | undefined

function pushPlugin(): FfcomPushPlugin {
  plugin ??= registerPlugin<FfcomPushPlugin>('FfcomPush')
  return plugin
}

const UNAVAILABLE: PushStatus = { available: false, enabled: false, asked: true }

// APK sem o plugin (anterior à fase 6) responde UNIMPLEMENTED: push
// indisponível, como no navegador.
export async function getPushStatus(): Promise<PushStatus> {
  if (!isAndroidApp()) return UNAVAILABLE
  try {
    return await pushPlugin().getStatus()
  } catch {
    return UNAVAILABLE
  }
}

export async function requestPushPermission(): Promise<boolean> {
  try {
    return (await pushPlugin().requestPermission()).enabled
  } catch {
    return false
  }
}

export function dismissPushPermission(): void {
  void pushPlugin().dismissPermission().catch(() => {})
}

export async function getPushToken(): Promise<string> {
  return (await pushPlugin().getToken()).token
}

// Listener de um evento do plugin; devolve a função que o remove.
function listen<T>(event: 'open' | 'token', callback: (data: T) => void): () => void {
  if (!isAndroidApp()) return () => {}
  let removed = false
  let handle: PluginListenerHandle | undefined
  Promise.resolve()
    .then(() => pushPlugin().addListener(event as 'open', callback as unknown as (data: PushOpen) => void))
    .then((h) => {
      handle = h
      if (removed) void h.remove()
    })
    .catch(() => {})
  return () => {
    removed = true
    void handle?.remove()
  }
}

// Toque numa notificação, inclusive o que abriu o app a frio (o nativo
// guarda o evento até a página ouvir).
export function onPushOpen(callback: (open: PushOpen) => void): () => void {
  return listen('open', callback)
}

// O FCM trocou o token do aparelho com a página aberta.
export function onPushToken(callback: (token: string) => void): () => void {
  return listen<{ token: string }>('token', (data) => callback(data.token))
}

let lastActive = ''

export function setPushActive(active: PushActive): void {
  if (!isAndroidApp()) return
  const next = JSON.stringify(active ?? {})
  if (next === lastActive) return
  lastActive = next
  void pushPlugin()
    .setActive(active ?? {})
    .catch(() => {})
}

// Modelos dos textos no idioma ativo, com os marcadores que o nativo troca
// (PushTexts.java): o serviço do FCM roda sem a WebView e sem i18n.
export function syncPushTexts(): void {
  if (!isAndroidApp()) return
  const t = i18n.t.bind(i18n)
  const texts: Record<string, string> = {
    channelMessages: t('push.android.channelMessages'),
    channelDms: t('push.android.channelDms'),
    channelFriends: t('push.android.channelFriends'),
    channelTitle: t('push.android.channelTitle', { channel: '{channel}', server: '{server}' }),
    newMessagesOne: t('push.android.newMessagesOne'),
    newMessagesMany: t('push.android.newMessagesMany', { n: '{n}' }),
    dmTitle: t('push.android.dmTitle', { author: '{author}' }),
    friendRequest: t('push.android.friendRequest', { author: '{author}' }),
    friendAccepted: t('push.android.friendAccepted', { author: '{author}' }),
    attachment: t('push.android.attachment', { name: '{name}' }),
    someone: t('push.android.someone'),
    you: t('push.android.you'),
  }
  void pushPlugin()
    .setTexts(texts)
    .catch(() => {})
}

// Estado do push da conta neste aparelho, no localStorage da WebView: o
// token registrado no server-central e o grant entregue a cada servidor.
// O grant só é pedido de novo quando o servidor entra na lista, quando o
// token do aparelho muda ou a cada GRANT_TTL (o server-channel não tem rota
// para conferir se ainda guarda o nosso: um kick seguido de convite, por
// exemplo, apaga o grant lá sem o app saber).
interface GrantEntry {
  // Grant entregue (guardado para o DELETE com {token} no logout).
  grant?: string
  // Servidor sem a rota de push (404): tenta de novo depois de UNSUPPORTED_TTL.
  unsupported?: true
  device: string
  at: number
}

interface PushStore {
  device?: string
  grants: Record<string, GrantEntry>
}

const GRANT_TTL = 7 * 24 * 60 * 60 * 1000
const UNSUPPORTED_TTL = 24 * 60 * 60 * 1000

function storeKey(accountSub: string) {
  return `ffcom.push.${accountSub}`
}

function readStore(accountSub: string): PushStore {
  try {
    const raw = localStorage.getItem(storeKey(accountSub))
    const parsed = raw ? (JSON.parse(raw) as PushStore) : undefined
    if (parsed && typeof parsed === 'object' && parsed.grants && typeof parsed.grants === 'object') return parsed
  } catch {
    // estragado: recomeça
  }
  return { grants: {} }
}

function writeStore(accountSub: string, store: PushStore): void {
  try {
    localStorage.setItem(storeKey(accountSub), JSON.stringify(store))
  } catch {
    // sem localStorage: os grants saem de novo na próxima abertura
  }
}

function clearStore(accountSub: string): void {
  try {
    localStorage.removeItem(storeKey(accountSub))
  } catch {
    // nada a fazer
  }
}

// Registra o aparelho na conta. A cada abertura, porque o server-central
// guarda o "último uso" e tira o aparelho mais antigo acima de 20.
export async function registerDevice(accessToken: string, accountSub: string, token: string): Promise<void> {
  await registerPushDevice(accessToken, token)
  const store = readStore(accountSub)
  store.device = token
  writeStore(accountSub, store)
}

// Notificações desligadas pela pessoa: o aparelho sai da conta, para o
// texto das mensagens não passar pelo Firebase à toa. Volta sozinho quando
// ela liga de novo (hooks/useAndroidPush.ts confere ao voltar ao app).
export async function unregisterDeviceIfRegistered(accessToken: string, accountSub: string): Promise<void> {
  const store = readStore(accountSub)
  if (!store.device) return
  await deletePushDevice(accessToken, store.device)
  delete store.device
  writeStore(accountSub, store)
}

// Pede ao central e entrega a cada servidor o grant que falta. Erros só vão
// para o console: um servidor fora do ar ou antigo fica sem push, sem aviso
// na tela.
// Uma sincronização por vez: duas seguidas (token novo e servidor novo no
// mesmo instante) pediriam dois grants para o mesmo servidor. A segunda lê
// o que a primeira guardou e pula o que já está feito.
let grantQueue: Promise<void> = Promise.resolve()

export function syncGrants(
  accessToken: string,
  accountSub: string,
  device: string,
  serverAddresses: string[],
): Promise<void> {
  grantQueue = grantQueue
    .then(() => syncGrantsNow(accessToken, accountSub, device, serverAddresses))
    .catch((err) => console.warn('[ffcom] push: sincronização dos grants falhou', err))
  return grantQueue
}

async function syncGrantsNow(
  accessToken: string,
  accountSub: string,
  device: string,
  serverAddresses: string[],
): Promise<void> {
  const store = readStore(accountSub)
  const now = Date.now()
  // Servidor que saiu da lista: o central já apagou os grants dele. Se
  // voltar, pede de novo.
  for (const address of Object.keys(store.grants)) {
    if (!serverAddresses.includes(address)) delete store.grants[address]
  }
  writeStore(accountSub, store)

  for (const address of serverAddresses) {
    const entry = store.grants[address]
    if (entry?.unsupported && now - entry.at < UNSUPPORTED_TTL) continue
    if (entry?.grant && entry.device === device && now - entry.at < GRANT_TTL) continue
    try {
      const grant = await createPushGrant(accessToken, address)
      try {
        await setPushGrant(address, accessToken, grant, address)
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          store.grants[address] = { unsupported: true, device, at: Date.now() }
          writeStore(accountSub, store)
          continue
        }
        throw err
      }
      store.grants[address] = { grant, device, at: Date.now() }
      writeStore(accountSub, store)
    } catch (err) {
      // 403 (não é mais membro), servidor fora do ar, 404 do central (a
      // lista mudou no meio): tenta na próxima sincronização.
      console.warn('[ffcom] push: grant não entregue a', address, err)
    }
  }
}

function withTimeout(promise: Promise<unknown>, ms: number): Promise<unknown> {
  return Promise.race([promise.catch(() => {}), new Promise((resolve) => setTimeout(resolve, ms))])
}

// Logout no app Android: tira os grants deste aparelho dos servidores, o
// aparelho da conta, o token do FCM e as notificações da gaveta. Tudo em
// melhor esforço e com prazo, para o "Sair" não ficar preso na rede.
export async function unregisterPush(accessToken: string, accountSub: string): Promise<void> {
  if (!isAndroidApp() || !accountSub) return
  const store = readStore(accountSub)
  const work = (async () => {
    await Promise.all(
      Object.entries(store.grants).map(([address, entry]) =>
        entry.grant ? deletePushGrant(address, accessToken, entry.grant).catch(() => {}) : Promise.resolve(),
      ),
    )
    if (store.device) await deletePushDevice(accessToken, store.device).catch(() => {})
    await pushPlugin()
      .deleteToken()
      .catch(() => {})
  })()
  await withTimeout(work, 4000)
  clearStore(accountSub)
  lastActive = ''
  await withTimeout(pushPlugin().clearAll(), 1000)
}
