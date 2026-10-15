import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DESKTOP_APP_CALLBACK_URL } from '../auth/config'
import './LoginScreen.css'

// /auth/desktop: retorno do login do app desktop no navegador do sistema.
// Repassa code e state (ou o erro do Authentik) ao app por
// ffcom://auth/callback, que o Windows entrega ao app instalado, e fica
// dizendo que a aba pode ser fechada. Com o ffcom:// direto no redirect_uri,
// o Authentik respondia o authorize com um 302 para o esquema, o navegador
// passava a URL ao app sem trocar de página, e a aba ficava no "carregando"
// do flow para sempre. O code não serve a este navegador (o PKCE ficou no
// app), então sai da barra de endereço. Ver docs/architecture.md, "Decisão:
// página de passagem no login do desktop".
function takeAppCallbackUrl(): string | null {
  const params = new URLSearchParams(window.location.search)
  if (!params.has('code') && !params.has('error')) return null
  window.history.replaceState({}, '', window.location.pathname)
  return `${DESKTOP_APP_CALLBACK_URL}?${params.toString()}`
}

export function DesktopAuthHandoff({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation()
  const [appUrl] = useState(takeAppCallbackUrl)
  const opened = useRef(false)

  useEffect(() => {
    // Uma vez só (StrictMode roda o efeito duas vezes em dev): cada abertura
    // do esquema mostra de novo o "Abrir FFCom?" do navegador.
    if (!appUrl || opened.current) return
    opened.current = true
    window.location.href = appUrl
  }, [appUrl])

  function continueInBrowser() {
    window.history.replaceState({}, '', '/')
    onDone()
  }

  return (
    <div className="login-screen">
      <div className="login-card">
        <div className="login-brand">
          <img className="login-logo" src="/favicon.svg" alt="" width="72" height="72" />
          <h1>FFCom</h1>
        </div>
        {appUrl ? (
          <>
            <h2 className="login-handoff-title">{t('auth.desktopHandoff.title')}</h2>
            <p className="placeholder">{t('auth.desktopHandoff.body')}</p>
            <a className="login-button" href={appUrl}>
              {t('auth.desktopHandoff.openApp')}
            </a>
          </>
        ) : (
          <>
            <h2 className="login-handoff-title">{t('auth.desktopHandoff.strayTitle')}</h2>
            <p className="placeholder">{t('auth.desktopHandoff.strayBody')}</p>
            <button type="button" className="login-button" onClick={continueInBrowser}>
              {t('auth.desktopHandoff.continue')}
            </button>
          </>
        )}
      </div>
    </div>
  )
}
