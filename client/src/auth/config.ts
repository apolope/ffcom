// Config do login OIDC do client contra a instância central de Authentik do
// abs-3d-printer (ver docs/architecture.md, "Decisão: autenticação em
// server-central — Authentik"). client_id/issuer não são segredo (client
// público, PKCE, sem client secret) — só o access token obtido é sensível.
// Pelo domínio auth.ffcom (brand do FFCom na mesma instância), para a tela
// de login do Authentik levar o logo do FFCom. O "iss" dos tokens passa a
// ser esse domínio: server-central e server-channel precisam dele no
// OIDC_ISSUER_URL antes de o client com esta config entrar no ar.
export const AUTHENTIK_ISSUER = 'https://auth.ffcom.a3sitsolutions.com.br/application/o/ffcom/'
export const AUTHENTIK_CLIENT_ID = 'ffcom'
export const AUTH_CALLBACK_PATH = '/auth/callback'

// "Esqueci minha senha": fluxo de recuperação do FFCom na brand própria do
// Authentik (domínio auth.ffcom), o mesmo em que cai o link do e-mail de
// cadastro aprovado. Termina de volta no app. Ver docs/architecture.md,
// "Decisão: cadastro com aprovação pelo Telegram".
export const AUTH_RECOVERY_URL = 'https://auth.ffcom.a3sitsolutions.com.br/if/flow/ffcom-recovery-flow/'
