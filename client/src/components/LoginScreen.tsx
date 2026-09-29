import { useAuth } from '../auth/AuthProvider'
import './LoginScreen.css'

export function LoginScreen() {
  const { signIn, redirecting } = useAuth()

  // Saindo, o botão nem aparece: clicar nele antes de o Authentik encerrar
  // a sessão entrava de novo na conta que acabou de sair.
  if (redirecting === 'signing-out') {
    return (
      <div className="login-screen">
        <div className="login-card">
          <h1>FFCom</h1>
          <p className="placeholder">Saindo…</p>
        </div>
      </div>
    )
  }

  return (
    <div className="login-screen">
      <div className="login-card">
        <h1>FFCom</h1>
        <p className="placeholder">Entre com sua conta para acessar seus servidores.</p>
        <button type="button" className="login-button" onClick={signIn} disabled={!!redirecting}>
          {redirecting ? 'Abrindo o Authentik…' : 'Entrar com Authentik'}
        </button>
      </div>
    </div>
  )
}
