import { useState } from 'react'
import App from './App.tsx'
import { ANDROID_AUTH_CALLBACK_PATH } from './auth/config'
import { AndroidAuthHandoff } from './components/AndroidAuthHandoff'
import { isAndroidApp } from './lib/platform'

// /auth/android é o retorno do login do app Android. No app ele nunca carrega
// na WebView (o Android entrega a URL ao app); aberto num navegador, mostra a
// explicação em vez do app.
export function Root() {
  const [androidHandoff, setAndroidHandoff] = useState(
    () => !isAndroidApp() && window.location.pathname === ANDROID_AUTH_CALLBACK_PATH,
  )
  if (androidHandoff) return <AndroidAuthHandoff onDone={() => setAndroidHandoff(false)} />
  return <App />
}
