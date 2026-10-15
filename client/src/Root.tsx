import { useState } from 'react'
import App from './App.tsx'
import { ANDROID_AUTH_CALLBACK_PATH, DESKTOP_AUTH_CALLBACK_PATH } from './auth/config'
import { AndroidAuthHandoff } from './components/AndroidAuthHandoff'
import { DesktopAuthHandoff } from './components/DesktopAuthHandoff'
import { isAndroidApp } from './lib/platform'

type Handoff = 'android' | 'desktop' | null

// /auth/android é o retorno do login do app Android. No app ele nunca carrega
// na WebView (o Android entrega a URL ao app); aberto num navegador, mostra a
// explicação em vez do app. /auth/desktop é o retorno do login do app
// desktop: sempre abre no navegador do sistema, que repassa o code ao app.
function initialHandoff(): Handoff {
  const path = window.location.pathname
  if (!isAndroidApp() && path === ANDROID_AUTH_CALLBACK_PATH) return 'android'
  if (!window.ffcomElectron && path === DESKTOP_AUTH_CALLBACK_PATH) return 'desktop'
  return null
}

export function Root() {
  const [handoff, setHandoff] = useState<Handoff>(initialHandoff)
  if (handoff === 'android') return <AndroidAuthHandoff onDone={() => setHandoff(null)} />
  if (handoff === 'desktop') return <DesktopAuthHandoff onDone={() => setHandoff(null)} />
  return <App />
}
