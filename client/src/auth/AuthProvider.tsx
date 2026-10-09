import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { OidcClient, type User } from 'oidc-client-ts'
import { androidExternalSignIn, desktopExternalSignIn, externalSignIn, userManager } from './userManager'
import { listenAndroidSignIn, openAndroidSignIn, takeAndroidLaunchCallback } from './androidSignIn'
import { AUTH_CALLBACK_PATH } from './config'
import { currentLanguage } from '../i18n'

type AuthStatus = 'loading' | 'signed-out' | 'signed-in'

// Idioma das telas do Authentik (login, recuperação de senha): ui_locales
// na URL de autorização, no formato do FFCom (pt-BR, en), que o Authentik
// aceita como está. Ele grava o cookie authentik_language, então o link
// "Esqueci minha senha" da tela de login segue no mesmo idioma. Ver
// docs/architecture.md, "Decisão: internacionalização".
function signinLocale() {
  return { extraQueryParams: { ui_locales: currentLanguage() } }
}

// Redirecionamento ao Authentik em andamento. O oidc-client-ts busca o
// discovery antes de navegar, e no logout remove o usuário antes disso: sem
// este estado a tela de login aparece com o botão ativo por um instante, e
// clicar nele ali abria um login novo no meio da saída. Nos apps desktop e
// Android o login segue no navegador do sistema ('in-browser') até o retorno
// chegar.
type AuthRedirect = 'signing-in' | 'in-browser' | 'signing-out'

// App desktop: "Sair" só esquece a sessão do app, a do Authentik no navegador
// continua. Sem esta marca, o "Entrar" seguinte voltaria direto para a mesma
// conta, sem chance de trocar; com ela, o próximo login pede a senha de novo
// (prompt=login). O primeiro login aproveita a sessão do navegador. O nome da
// chave fica o do desktop para não perder a marca de quem já saiu.
const DESKTOP_SIGNED_OUT_KEY = 'ffcom:desktop-signed-out'

// No app Android todo login pede a senha (prompt=login). Com a sessão do
// Authentik viva no Chrome, o Authentik devolveria o code por redirect sem
// nenhum toque da pessoa, e o Chrome não costuma passar a um App Link uma
// navegação sem gesto: o retorno ficaria preso no Custom Tab. O toque em
// "Entrar" no Authentik (com a senha salva no Chrome, um toque só) garante o
// gesto. Ver docs/architecture.md, "Decisão: login do app Android no
// navegador do sistema (fase 3)".
function externalSigninPrompt(): string | undefined {
  if (androidExternalSignIn || readDesktopSignedOut()) return 'login'
  return undefined
}

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

    // Retorno do login do app Android (App Link /auth/android). Uma falha
    // (state vencido, code recusado) só registra: a sessão que houver
    // continua valendo.
    async function completeAndroidSignIn(url: string) {
      try {
        const u = await userManager.signinRedirectCallback(url)
        writeDesktopSignedOut(false)
        applyUser(u)
        return true
      } catch (err) {
        console.error('ffcom: falha ao concluir login OIDC', err)
        return false
      } finally {
        setRedirecting(undefined)
      }
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
      if (desktopExternalSignIn && (await completeExternalSignIn())) return
      if (androidExternalSignIn) {
        // App aberto pelo App Link do login, com o processo morto enquanto o
        // Custom Tab estava na frente.
        const url = await takeAndroidLaunchCallback()
        if (url && (await completeAndroidSignIn(url))) return
      }

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
    const removeAuthCallbackListener = desktopExternalSignIn
      ? window.ffcomElectron?.onAuthCallback(() => void completeExternalSignIn())
      : androidExternalSignIn
        ? listenAndroidSignIn(
            (url) => void completeAndroidSignIn(url),
            // Custom Tab fechado sem terminar o login: volta o botão "Entrar".
            () => setRedirecting((r) => (r === 'in-browser' ? undefined : r)),
          )
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
              prompt: externalSigninPrompt(),
              ...signinLocale(),
            })
            .then(async (request) => {
              if (androidExternalSignIn) await openAndroidSignIn(request.url)
              else await window.ffcomElectron?.openExternalSignIn(request.url)
            })
            .then(() => setRedirecting('in-browser'))
            .catch((err) => {
              console.error('ffcom: falha ao abrir o login', err)
              setRedirecting(undefined)
            })
          return
        }
        if (redirecting) return
        setRedirecting('signing-in')
        userManager.signinRedirect(signinLocale()).catch((err) => {
          console.error('ffcom: falha ao abrir o login', err)
          setRedirecting(undefined)
        })
      },
      signOut: () => {
        if (redirecting) return
        setRedirecting('signing-out')
        if (externalSignIn) {
          // A sessão do Authentik está no navegador, não nesta janela: o
          // encerramento por redirect aqui não a alcançaria (e no Android o
          // destino do logout não é um App Link, então ele terminaria no
          // Custom Tab). Revoga o refresh token e esquece o usuário; o
          // próximo login pede a senha.
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
