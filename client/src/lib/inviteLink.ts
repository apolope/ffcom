import { APP_WEB_ORIGIN } from '../auth/config'

// Links de convite para um server-channel. Ver docs/architecture.md,
// "Decisão: convite auto-contido" e "Decisão: convites pelo domínio do app
// (fase 4)".
//
// Formato atual, gerado por InviteServerDialog:
//   https://app.ffcom.a3sitsolutions.com.br/convite?server=<endereço>&invite=<código>&name=<nome>
// Pelo domínio do client, e não pelo do server-channel (que pode ser qualquer
// um), o link abre o app Android pelo App Link e, sem o app, o client web.
//
// Formato antigo, ainda aceito ao colar: <endereço>?invite=<código>&name=<nome>.

export const INVITE_PATH = '/convite'

export interface ParsedInvite {
  address: string
  inviteCode: string
  name?: string
}

// Origem dos links gerados: a da página no navegador (o próprio client
// publicado, ou localhost em desenvolvimento) e a oficial no desktop
// (app://ffcom) e no app Android, onde a página não tem um endereço que
// alguém de fora consiga abrir.
export function inviteLinkOrigin(): string {
  if (typeof window === 'undefined') return APP_WEB_ORIGIN
  const { protocol, origin } = window.location
  if (window.ffcomElectron || (protocol !== 'https:' && protocol !== 'http:')) return APP_WEB_ORIGIN
  return origin
}

export function buildInviteLink(serverBaseUrl: string, code: string, serverName: string, origin = inviteLinkOrigin()): string {
  const params = new URLSearchParams({ server: serverBaseUrl.replace(/\/+$/, ''), invite: code })
  if (serverName.trim()) params.set('name', serverName.trim())
  return `${origin.replace(/\/+$/, '')}${INVITE_PATH}?${params.toString()}`
}

// Separa um link de convite (qualquer um dos dois formatos) em endereço,
// código e nome. Devolve undefined se o valor não for uma URL com `invite`.
// Usado pelo AddServerDialog, pelo NotMemberPanel e pela rota /convite
// (lib/pendingInvite.ts).
export function parseInviteLink(value: string): ParsedInvite | undefined {
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return undefined
  }
  const code = url.searchParams.get('invite')?.trim()
  if (!code) return undefined
  // Links anteriores ao `&name=` não trazem o nome; a pessoa digita.
  const name = url.searchParams.get('name')?.trim() || undefined
  if (isInvitePath(url.pathname)) {
    const server = url.searchParams.get('server')?.trim()
    if (!server) return undefined
    return { address: server.replace(/\/+$/, ''), inviteCode: code, name }
  }
  url.search = ''
  url.hash = ''
  return { address: url.toString().replace(/\/+$/, ''), inviteCode: code, name }
}

export function isInvitePath(pathname: string): boolean {
  return pathname.replace(/\/+$/, '') === INVITE_PATH
}
