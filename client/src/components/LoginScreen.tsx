import { useAuth } from '../auth/AuthProvider'
import './LoginScreen.css'

export function LoginScreen() {
  const { signIn } = useAuth()

  return (
    <div className="login-screen">
      <div className="login-card">
        <h1>FFCom</h1>
        <p className="placeholder">Entre com sua conta para acessar seus servidores.</p>
        <button type="button" className="login-button" onClick={signIn}>
          Entrar com Authentik
        </button>
      </div>
    </div>
  )
}
