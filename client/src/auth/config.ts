// Config do login OIDC do client contra a instância central de Authentik do
// abs-3d-printer (ver docs/architecture.md, "Decisão: autenticação em
// server-central — Authentik"). client_id/issuer não são segredo (client
// público, PKCE, sem client secret) — só o access token obtido é sensível.
export const AUTHENTIK_ISSUER = 'https://authentik.abs.a3sitsolutions.com.br/application/o/ffcom/'
export const AUTHENTIK_CLIENT_ID = 'ffcom'
export const AUTH_CALLBACK_PATH = '/auth/callback'
