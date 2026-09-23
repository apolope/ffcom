import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import electron from 'vite-plugin-electron/simple'
import { VitePWA } from 'vite-plugin-pwa'

// https://vite.dev/config/
export default defineConfig(({ mode }) => ({
  plugins: [
    react(),
    mode === 'electron' &&
      electron({
        main: {
          entry: 'electron/main.ts',
        },
        preload: {
          input: 'electron/preload.ts',
        },
      }),
    // Só faz sentido no build web/PWA — o shell Electron já é o próprio
    // "app instalado" e não precisa de service worker/manifest. No Electron
    // o plugin fica carregado mas com `disable`, para o import de
    // `virtual:pwa-register/react` (components/UpdateButton.tsx) resolver
    // para um módulo vazio em vez de quebrar o build.
    VitePWA({
        disable: mode === 'electron',
        // 'prompt': a versão nova baixa e espera o clique no botão de
        // atualizar do ServerRail, em vez de trocar sozinha (recarregar
        // derruba uma chamada de voz). Ver docs/architecture.md, "Decisão:
        // botão de atualizar o client".
        registerType: 'prompt',
        // O registro é feito por useRegisterSW em UpdateButton.tsx.
        injectRegister: false,
        includeAssets: ['favicon.svg', 'favicon.ico', 'apple-touch-icon-180x180.png'],
        manifest: {
          name: 'FFCom',
          short_name: 'FFCom',
          description: 'Alternativa self-hosted ao Discord: servidores, categorias e canais de texto/voz/forum.',
          theme_color: '#aa3bff',
          background_color: '#ffffff',
          display: 'standalone',
          start_url: '/',
          scope: '/',
          icons: [
            { src: 'pwa-64x64.png', sizes: '64x64', type: 'image/png' },
            { src: 'pwa-192x192.png', sizes: '192x192', type: 'image/png' },
            { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png' },
            {
              src: 'maskable-icon-512x512.png',
              sizes: '512x512',
              type: 'image/png',
              purpose: 'maskable',
            },
          ],
        },
        workbox: {
          // /api (REST) e /auth/callback (retorno do OIDC) nunca podem
          // servir HTML do cache: precisam sempre bater no backend/estado
          // vivo. O WebSocket (chat, presença, voz) não passa pelo fetch
          // handler do service worker, então nem precisa de exclusão.
          navigateFallbackDenylist: [/^\/api\//, /^\/auth\//],
        },
    }),
  ],
}))
