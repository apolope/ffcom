import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthProvider'
import { AUTH_RECOVERY_URL } from '../auth/config'
import './LoginScreen.css'

// Logo do FFCom (o mesmo do favicon, já no precache do service worker, e o
// que a brand auth.ffcom do Authentik mostra), para a tela do app e a do
// Authentik parecerem o mesmo lugar.
function LoginBrand() {
  return (
    <div className="login-brand">
      <img className="login-logo" src="/favicon.svg" alt="" width="72" height="72" />
      <h1>FFCom</h1>
    </div>
  )
}

export function LoginScreen() {
  const { signIn, redirecting } = useAuth()
  const { t, i18n } = useTranslation()

  // Saindo, o botão nem aparece: clicar nele antes de o Authentik encerrar
  // a sessão entrava de novo na conta que acabou de sair.
  if (redirecting === 'signing-out') {
    return (
      <div className="login-screen">
        <div className="login-card" aria-busy="true">
          <LoginBrand />
          <p className="placeholder login-status">
            <span className="login-spinner" aria-hidden="true" />
            {t('auth.signingOut')}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="login-screen">
      <div className="login-card">
        <LoginBrand />
        {/* App desktop: o login segue no navegador do sistema e volta
            sozinho. O botão continua ativo para abrir de novo se a aba foi
            fechada. */}
        {redirecting === 'in-browser' ? (
          <p className="placeholder login-status">
            <span className="login-spinner" aria-hidden="true" />
            {t('auth.continueInBrowser')}
          </p>
        ) : (
          <p className="placeholder">{t('auth.signInPrompt')}</p>
        )}
        <button
          type="button"
          className="login-button"
          onClick={signIn}
          disabled={redirecting === 'signing-in'}
        >
          {redirecting === 'signing-in'
            ? t('auth.openingSignIn')
            : redirecting === 'in-browser'
              ? t('auth.reopenSignIn')
              : t('auth.signIn')}
        </button>
        {/* Nova aba (no Electron, o navegador do sistema): a tela de login
            fica aqui para entrar depois de definir a senha. */}
        {/* ?locale= é lido pela página do Authentik (acima do idioma do
            navegador), que então pede tudo à API nesse idioma. */}
        <a
          className="login-recovery"
          href={`${AUTH_RECOVERY_URL}?locale=${encodeURIComponent(i18n.language)}`}
          target="_blank"
          rel="noreferrer"
        >
          {t('auth.forgotPassword')}
        </a>
      </div>
    </div>
  )
}
