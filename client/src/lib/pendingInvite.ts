import { isInvitePath, parseInviteLink, type ParsedInvite } from './inviteLink'
import { isAndroidApp } from './platform'

// Convite recebido pela rota /convite (lib/inviteLink.ts), à espera de a
// página mostrar o "Adicionar servidor" já preenchido. Ver
// docs/architecture.md, "Decisão: convites pelo domínio do app (fase 4)".
//
// - Navegador, PWA: a página abre em /convite. captureInviteFromLocation(),
//   antes do primeiro render, guarda o convite e troca o endereço por "/"
//   (replaceState: não empilha nada, o popstate das gavetas não vê).
// - App Android: o App Link /convite chega por appUrlOpen (app aberto) ou
//   por App.getLaunchUrl() (app fechado). A WebView continua no endereço
//   raiz, o Capacitor não navega até a URL do intent.
// - Desktop: não registra o link; ele abre no navegador, como no web.
//
// Fica no sessionStorage até a página pegar: sem sessão, o login no web sai
// para o Authentik e volta na mesma aba, e o convite continua guardado.
// Logado, App.tsx pega na hora.

const PENDING_KEY = 'ffcom:pending-invite'
// Última URL de abertura do app Android já tratada. O getLaunchUrl devolve a
// mesma URL enquanto a Activity vive, inclusive depois de recarregar a
// página, e o diálogo não deve voltar a cada reload.
const LAUNCH_HANDLED_KEY = 'ffcom:invite-launch-handled'
const INVITE_EVENT = 'ffcom:pending-invite'

// Sem sessionStorage (bloqueado), o convite fica só na memória: some num
// login pelo web, mas ainda abre para quem já está logado.
let memoryInvite: ParsedInvite | undefined

function store(invite: ParsedInvite) {
  memoryInvite = invite
  try {
    window.sessionStorage.setItem(PENDING_KEY, JSON.stringify(invite))
  } catch {
    // Fica na memória.
  }
  window.dispatchEvent(new Event(INVITE_EVENT))
}

// URL de convite (/convite com `invite`) para guardar; false se não for.
function acceptInviteUrl(url: string): boolean {
  let pathname: string
  try {
    pathname = new URL(url).pathname
  } catch {
    return false
  }
  if (!isInvitePath(pathname)) return false
  const invite = parseInviteLink(url)
  if (!invite) return false
  store(invite)
  return true
}

export function takePendingInvite(): ParsedInvite | undefined {
  let invite = memoryInvite
  memoryInvite = undefined
  try {
    const raw = window.sessionStorage.getItem(PENDING_KEY)
    window.sessionStorage.removeItem(PENDING_KEY)
    if (raw) {
      const parsed = JSON.parse(raw) as ParsedInvite
      if (parsed?.address && parsed?.inviteCode) invite = parsed
    }
  } catch {
    // Vale o da memória.
  }
  return invite
}

// Avisa quando chega um convite com a página aberta. Devolve a função que
// remove o listener.
export function onPendingInvite(listener: () => void): () => void {
  window.addEventListener(INVITE_EVENT, listener)
  return () => window.removeEventListener(INVITE_EVENT, listener)
}

// Navegador e PWA: chamada antes do primeiro render, no main.tsx.
export function captureInviteFromLocation(): void {
  if (!isInvitePath(window.location.pathname)) return
  acceptInviteUrl(window.location.href)
  window.history.replaceState(null, '', '/')
}

function launchAlreadyHandled(url: string): boolean {
  try {
    if (window.localStorage.getItem(LAUNCH_HANDLED_KEY) === url) return true
    window.localStorage.setItem(LAUNCH_HANDLED_KEY, url)
  } catch {
    // Sem localStorage, um reload pode mostrar o convite de novo.
  }
  return false
}

// App Android: escuta o App Link /convite. O retorno do login
// (/auth/android) tem o próprio listener em auth/androidSignIn.ts; os dois
// recebem todo appUrlOpen e cada um ignora a URL que não é sua.
export async function setupAndroidInviteLinks(): Promise<void> {
  if (!isAndroidApp()) return
  const { App } = await import('@capacitor/app')
  await App.addListener('appUrlOpen', ({ url }) => {
    acceptInviteUrl(url)
  })
  try {
    const launch = await App.getLaunchUrl()
    if (launch?.url && isInviteUrl(launch.url) && !launchAlreadyHandled(launch.url)) acceptInviteUrl(launch.url)
  } catch (err) {
    console.error('ffcom: falha ao ler a URL de abertura do app', err)
  }
}

function isInviteUrl(url: string): boolean {
  try {
    return isInvitePath(new URL(url).pathname)
  } catch {
    return false
  }
}
