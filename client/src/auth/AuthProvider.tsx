import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import type { User } from 'oidc-client-ts'
import { userManager } from './userManager'
import { AUTH_CALLBACK_PATH } from './config'

type AuthStatus = 'loading' | 'signed-out' | 'signed-in'

// Redirecionamento ao Authentik em andamento. O oidc-client-ts busca o
// discovery antes de navegar, e no logout remove o usuário antes disso: sem
// este estado a tela de login aparece com o botão ativo por um instante, e
// clicar nele ali abria um login novo no meio da saída.
type AuthRedirect = 'signing-in' | 'signing-out'

interface AuthContextValue {
  status: AuthStatus
  user: User | null
  accessToken: string | undefined
  redirecting: AuthRedirect | undefined
  signIn: () => void
  signOut: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('loading')
  const [user, setUser] = useState<User | null>(null)
  const [redirecting, setRedirecting] = useState<AuthRedirect>()
  const initStarted = useRef(false)

  useEffect(() => {
    function applyUser(u: User | null) {
      setUser(u && !u.expired ? u : null)
      setStatus(u && !u.expired ? 'signed-in' : 'signed-out')
    }

    async function init() {
      if (window.location.pathname === AUTH_CALLBACK_PATH) {
        try {
          const u = await userManager.signinRedirectCallback()
          window.history.replaceState({}, '', '/')
          applyUser(u)
        } catch (err) {
          console.error('ffcom: falha ao concluir login OIDC', err)
          window.history.replaceState({}, '', '/')
          applyUser(null)
        }
        return
      }

      const u = await userManager.getUser()
      applyUser(u)
    }

    // Guarda contra o double-invoke de efeitos do StrictMode em dev: o
    // callback do Authentik consome um "code" de uso único, uma segunda
    // chamada a signinRedirectCallback() para o mesmo code sempre falha.
    if (!initStarted.current) {
      initStarted.current = true
      init()
    }

    function onSilentRenewError(err: Error) {
      // Refresh token expirado/revogado: sem isso o estado ficaria
      // "signed-in" com um access token morto até a próxima chamada de API
      // falhar. Força novo login em vez de deixar a UI presa.
      console.error('ffcom: falha ao renovar sessão silenciosamente', err)
      applyUser(null)
    }

    // Voltar do Authentik pelo histórico pode restaurar a página do cache
    // (bfcache) com o redirecionamento ainda marcado como em andamento.
    function onPageShow(e: PageTransitionEvent) {
      if (e.persisted) setRedirecting(undefined)
    }
    window.addEventListener('pageshow', onPageShow)

    userManager.events.addUserLoaded(applyUser)
    userManager.events.addUserUnloaded(() => applyUser(null))
    userManager.events.addSilentRenewError(onSilentRenewError)
    return () => {
      window.removeEventListener('pageshow', onPageShow)
      userManager.events.removeUserLoaded(applyUser)
      userManager.events.removeUserUnloaded(() => applyUser(null))
      userManager.events.removeSilentRenewError(onSilentRenewError)
    }
  }, [])

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      accessToken: user?.access_token,
      redirecting,
      signIn: () => {
        if (redirecting) return
        setRedirecting('signing-in')
        userManager.signinRedirect().catch((err) => {
          console.error('ffcom: falha ao abrir o login', err)
          setRedirecting(undefined)
        })
      },
      signOut: () => {
        if (redirecting) return
        setRedirecting('signing-out')
        userManager.signoutRedirect().catch((err) => {
          // O usuário local já foi removido: fica na tela de login.
          console.error('ffcom: falha ao encerrar a sessão no Authentik', err)
          setRedirecting(undefined)
        })
      },
    }),
    [status, user, redirecting],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth precisa ser usado dentro de <AuthProvider>')
  }
  return ctx
}
