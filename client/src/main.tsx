import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import { Root } from './Root'
import { AuthProvider } from './auth/AuthProvider'
import { initI18n } from './i18n'
import { setupAndroidBackButton } from './lib/androidBackButton'
import { captureInviteFromLocation, setupAndroidInviteLinks } from './lib/pendingInvite'
import { installScrollbarActivity } from './lib/scrollbarActivity'

// Antes do render: a primeira tela já sai no idioma escolhido.
initI18n()
installScrollbarActivity()
void setupAndroidBackButton()
// Rota /convite (web) e App Link /convite (Android), antes do login e do
// primeiro render. Ver lib/pendingInvite.ts.
captureInviteFromLocation()
void setupAndroidInviteLinks()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthProvider>
      <Root />
    </AuthProvider>
  </StrictMode>,
)
