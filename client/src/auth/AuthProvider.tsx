import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { OidcClient, type User } from 'oidc-client-ts'
import { externalSignIn, userManager } from './userManager'
import { AUTH_CALLBACK_PATH } from './config'

type AuthStatus = 'loading' | 'signed-out' | 'signed-in'

// Redirecionamento ao Authentik em andamento. O oidc-client-ts busca o
// discovery antes de navegar, e no logout remove o usuário antes disso: sem
// este estado a tela de login aparece com o botão ativo por um instante, e
// clicar nele ali abria um login novo no meio da saída. No app desktop o
// login segue no navegador do sistema ('in-browser') até o retorno chegar.
type AuthRedirect = 'signing-in' | 'in-browser' | 'signing-out'

// App desktop: "Sair" só esquece a sessão do app, a do Authentik no navegador
// continua. Sem esta marca, o "Entrar" seguinte voltaria direto para a mesma
// conta, sem chance de trocar; com ela, o próximo login pede a senha de novo
// (prompt=login). O primeiro login aproveita a sessão do navegador.
const DESKTOP_SIGNED_OUT_KEY = 'ffcom:desktop-signed-out'

function readDesktopSignedOut(): boolean {
  try {
    return window.localStorage.getItem(DESKTOP_SIGNED_OUT_KEY) === '1'
  } catch {
    return false
  }
}

function writeDesktopSignedOut(signedOut: boolean) {
  try {
    if (signedOut) window.localStorage.setItem(DESKTOP_SIGNED_OUT_KEY, '1')
    else window.localStorage.removeItem(DESKTOP_SIGNED_OUT_KEY)
  } catch {
    // Sem localStorage o login só não força a senha depois de sair.
  }
}

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

    // Retorno do login do app desktop (ffcom://auth/callback), entregue pelo
    // main. O "code" é de uso único: takeAuthCallback devolve cada URL uma
    // vez só, então o aviso e a busca do init não concluem o mesmo login duas
    // vezes.
    async function completeExternalSignIn() {
      const url = await window.ffcomElectron?.takeAuthCallback()
      if (!url) return false
      try {
        const u = await userManager.signinRedirectCallback(url)
        writeDesktopSignedOut(false)
        applyUser(u)
      } catch (err) {
        console.error('ffcom: falha ao concluir login OIDC', err)
      }
      setRedirecting(undefined)
      return true
    }

    async function init() {
      if (externalSignIn && (await completeExternalSignIn())) return

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
    const removeAuthCallbackListener = externalSignIn
      ? window.ffcomElectron?.onAuthCallback(() => void completeExternalSignIn())
      : undefined

    userManager.events.addUserLoaded(applyUser)
    userManager.events.addUserUnloaded(() => applyUser(null))
    userManager.events.addSilentRenewError(onSilentRenewError)
    return () => {
      window.removeEventListener('pageshow', onPageShow)
      removeAuthCallbackListener?.()
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
        if (externalSignIn) {
          // Com o login já aberto no navegador, clicar de novo abre outro
          // (a aba pode ter sido fechada); o state antigo vence sozinho.
          if (redirecting === 'signing-in' || redirecting === 'signing-out') return
          setRedirecting('signing-in')
          new OidcClient(userManager.settings)
            .createSigninRequest({
              request_type: 'si:r',
              prompt: readDesktopSignedOut() ? 'login' : undefined,
            })
            .then((request) => window.ffcomElectron?.openExternalSignIn(request.url))
            .then(() => setRedirecting('in-browser'))
            .catch((err) => {
              console.error('ffcom: falha ao abrir o login', err)
              setRedirecting(undefined)
            })
          return
        }
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
        if (externalSignIn) {
          // A sessão do Authentik está no navegador, não nesta janela: o
          // encerramento por redirect aqui não a alcançaria. Revoga o refresh
          // token e esquece o usuário; o próximo login pede a senha.
          writeDesktopSignedOut(true)
          userManager
            .revokeTokens(['refresh_token'])
            .catch((err) => console.error('ffcom: falha ao revogar a sessão no Authentik', err))
            .then(() => userManager.removeUser())
            .finally(() => setRedirecting(undefined))
          return
        }
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
