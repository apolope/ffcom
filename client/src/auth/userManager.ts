import { UserManager, WebStorageStateStore } from 'oidc-client-ts'
import { AUTHENTIK_ISSUER, AUTHENTIK_CLIENT_ID, AUTH_CALLBACK_PATH } from './config'

// Instância única do UserManager (oidc-client-ts) para o fluxo Authorization
// Code + PKCE contra o Authentik central. Client público (sem client_secret)
// — ver docs/architecture.md, seção de autenticação.
export const userManager = new UserManager({
  authority: AUTHENTIK_ISSUER,
  client_id: AUTHENTIK_CLIENT_ID,
  redirect_uri: `${window.location.origin}${AUTH_CALLBACK_PATH}`,
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
