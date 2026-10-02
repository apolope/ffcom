import { UserManager, WebStorageStateStore } from 'oidc-client-ts'
import { AUTHENTIK_ISSUER, AUTHENTIK_CLIENT_ID, AUTH_CALLBACK_PATH, DESKTOP_AUTH_CALLBACK_URL } from './config'

// App desktop empacotado (servido de app://ffcom): o login sai para o
// navegador do sistema, onde ficam a conta e as senhas salvas, e volta por
// ffcom://auth/callback. No dev do Electron (http://localhost) e no web, o
// login continua na própria janela. Ver docs/architecture.md, "Decisão:
// login do app desktop no navegador do sistema".
export const externalSignIn = window.location.protocol === 'app:' && !!window.ffcomElectron

// Instância única do UserManager (oidc-client-ts) para o fluxo Authorization
// Code + PKCE contra o Authentik central. Client público (sem client_secret)
// — ver docs/architecture.md, seção de autenticação.
export const userManager = new UserManager({
  authority: AUTHENTIK_ISSUER,
  client_id: AUTHENTIK_CLIENT_ID,
  redirect_uri: externalSignIn ? DESKTOP_AUTH_CALLBACK_URL : `${window.location.origin}${AUTH_CALLBACK_PATH}`,
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
