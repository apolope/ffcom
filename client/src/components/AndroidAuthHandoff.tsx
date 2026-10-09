import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthProvider'
import './LoginScreen.css'

// /auth/android aberto num navegador: é o retorno do login do app Android
// (App Link), que só o app sabe concluir, porque o state e o PKCE ficaram na
// WebView dele. Cai aqui quem não tem o app, ou quando o Android não passou o
// link ao app. "Abrir no navegador" começa um login do web, com o retorno de
// sempre (/auth/callback); como a sessão do Authentik acabou de ser aberta
// neste navegador, ele costuma voltar sem pedir a senha. Ver
// docs/architecture.md, "Decisão: login do app Android no navegador do
// sistema (fase 3)".
export function AndroidAuthHandoff({ onDone }: { onDone: () => void }) {
  const { status, signIn } = useAuth()
  const { t } = useTranslation()

  function continueInBrowser() {
    // Tira o code da barra de endereço: ele não serve para este navegador.
    window.history.replaceState({}, '', '/')
    if (status !== 'signed-in') signIn()
    onDone()
  }

  return (
    <div className="login-screen">
      <div className="login-card">
        <div className="login-brand">
          <img className="login-logo" src="/favicon.svg" alt="" width="72" height="72" />
          <h1>FFCom</h1>
        </div>
        <h2 className="login-handoff-title">{t('auth.androidHandoff.title')}</h2>
        <p className="placeholder">{t('auth.androidHandoff.body')}</p>
        <button type="button" className="login-button" onClick={continueInBrowser} disabled={status === 'loading'}>
          {t('auth.androidHandoff.continue')}
        </button>
      </div>
    </div>
  )
}
