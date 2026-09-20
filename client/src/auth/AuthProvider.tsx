import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import type { User } from 'oidc-client-ts'
import { userManager } from './userManager'
import { AUTH_CALLBACK_PATH } from './config'

type AuthStatus = 'loading' | 'signed-out' | 'signed-in'

interface AuthContextValue {
  status: AuthStatus
  user: User | null
  accessToken: string | undefined
  signIn: () => void
  signOut: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('loading')
  const [user, setUser] = useState<User | null>(null)
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

    userManager.events.addUserLoaded(applyUser)
    userManager.events.addUserUnloaded(() => applyUser(null))
    return () => {
      userManager.events.removeUserLoaded(applyUser)
      userManager.events.removeUserUnloaded(() => applyUser(null))
    }
  }, [])

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      accessToken: user?.access_token,
      signIn: () => {
        userManager.signinRedirect()
      },
      signOut: () => {
        userManager.signoutRedirect()
      },
    }),
    [status, user],
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
