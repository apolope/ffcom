import { UserManager, WebStorageStateStore } from 'oidc-client-ts'
import {
  AUTHENTIK_ISSUER,
  AUTHENTIK_CLIENT_ID,
  AUTH_CALLBACK_PATH,
  ANDROID_AUTH_CALLBACK_URL,
  DESKTOP_AUTH_CALLBACK_URL,
} from './config'
import { isAndroidApp } from '../lib/platform'

// App desktop empacotado (servido de app://ffcom): o login sai para o
// navegador do sistema, onde ficam a conta e as senhas salvas, e volta por
// ffcom://auth/callback. No dev do Electron (http://localhost) e no web, o
// login continua na própria janela. Ver docs/architecture.md, "Decisão:
// login do app desktop no navegador do sistema".
export const desktopExternalSignIn = window.location.protocol === 'app:' && !!window.ffcomElectron

// App Android: o login abre num Custom Tab e volta pelo App Link
// https://app.ffcom.a3sitsolutions.com.br/auth/android. Ver
// docs/architecture.md, "Decisão: login do app Android no navegador do
// sistema (fase 3)".
export const androidExternalSignIn = isAndroidApp()

// Login fora da página (navegador do sistema), no desktop ou no Android.
export const externalSignIn = desktopExternalSignIn || androidExternalSignIn

function redirectUri(): string {
  if (desktopExternalSignIn) return DESKTOP_AUTH_CALLBACK_URL
  if (androidExternalSignIn) return ANDROID_AUTH_CALLBACK_URL
  return `${window.location.origin}${AUTH_CALLBACK_PATH}`
}

// Instância única do UserManager (oidc-client-ts) para o fluxo Authorization
// Code + PKCE contra o Authentik central. Client público (sem client_secret)
// — ver docs/architecture.md, seção de autenticação.
export const userManager = new UserManager({
  authority: AUTHENTIK_ISSUER,
  client_id: AUTHENTIK_CLIENT_ID,
  redirect_uri: redirectUri(),
  post_logout_redirect_uri: window.location.origin,
  response_type: 'code',
  // offline_access pede um refresh_token ao Authentik (precisa do scope
  // mapping correspondente no provider, ver blueprint em abs-3d-printer) —
  // usado pela renovação silenciosa abaixo em vez do padrão de iframe
  // (prompt=none), que depende de cookie de terceiro e não funciona bem no
  // build Electron (esquema app://). Ver docs/architecture.md, "Decisão:
  // renovação silenciosa de sessão no client".
  scope: 'openid profile email offline_access',
  automaticSilentRenew: true,
  userStore: new WebStorageStateStore({ store: window.localStorage }),
})
