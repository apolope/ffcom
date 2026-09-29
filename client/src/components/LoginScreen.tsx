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

  // Saindo, o botão nem aparece: clicar nele antes de o Authentik encerrar
  // a sessão entrava de novo na conta que acabou de sair.
  if (redirecting === 'signing-out') {
    return (
      <div className="login-screen">
        <div className="login-card" aria-busy="true">
          <LoginBrand />
          <p className="placeholder login-status">
            <span className="login-spinner" aria-hidden="true" />
            Saindo…
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="login-screen">
      <div className="login-card">
        <LoginBrand />
        <p className="placeholder">Entre com sua conta para acessar seus servidores.</p>
        <button type="button" className="login-button" onClick={signIn} disabled={!!redirecting}>
          {redirecting ? 'Abrindo o login…' : 'Entrar'}
        </button>
        {/* Nova aba (no Electron, o navegador do sistema): a tela de login
            fica aqui para entrar depois de definir a senha. */}
        <a className="login-recovery" href={AUTH_RECOVERY_URL} target="_blank" rel="noreferrer">
          Esqueci minha senha
        </a>
      </div>
    </div>
  )
}
