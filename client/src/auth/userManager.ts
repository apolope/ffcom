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
  scope: 'openid profile email',
  // Sem renovação silenciosa automática por enquanto: exigiria um segundo
  // redirect_uri (iframe de silent renew) cadastrado no blueprint do
  // Authentik, que hoje só tem o callback principal (ver TODO.md). Quando o
  // access token expirar, o usuário simplesmente loga de novo.
  automaticSilentRenew: false,
  userStore: new WebStorageStateStore({ store: window.localStorage }),
})
