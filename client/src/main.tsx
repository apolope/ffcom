import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { AuthProvider } from './auth/AuthProvider'
import { initI18n } from './i18n'
import { installScrollbarActivity } from './lib/scrollbarActivity'

// Antes do render: a primeira tela já sai no idioma escolhido.
initI18n()
installScrollbarActivity()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthProvider>
      <App />
    </AuthProvider>
  </StrictMode>,
)
