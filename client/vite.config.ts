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
    // "app instalado" e não precisa de service worker/manifest.
    mode !== 'electron' &&
      VitePWA({
        registerType: 'autoUpdate',
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
