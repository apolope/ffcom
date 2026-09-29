// Config do login OIDC do client contra a instância central de Authentik do
// abs-3d-printer (ver docs/architecture.md, "Decisão: autenticação em
// server-central — Authentik"). client_id/issuer não são segredo (client
// público, PKCE, sem client secret) — só o access token obtido é sensível.
export const AUTHENTIK_ISSUER = 'https://authentik.abs.a3sitsolutions.com.br/application/o/ffcom/'
export const AUTHENTIK_CLIENT_ID = 'ffcom'
export const AUTH_CALLBACK_PATH = '/auth/callback'

// "Esqueci minha senha": fluxo de recuperação do FFCom na brand própria do
// Authentik (domínio auth.ffcom), o mesmo em que cai o link do e-mail de
// cadastro aprovado. Termina de volta no app. Ver docs/architecture.md,
// "Decisão: cadastro com aprovação pelo Telegram".
export const AUTH_RECOVERY_URL = 'https://auth.ffcom.a3sitsolutions.com.br/if/flow/ffcom-recovery-flow/'
