import { execFileSync } from 'node:child_process'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import electron from 'vite-plugin-electron/simple'
import { VitePWA } from 'vite-plugin-pwa'
import { thirdPartyLicenses } from './vite-plugin-third-party-licenses.ts'

// Versão mostrada no menu do avatar. O deploy web passa a tag por
// VITE_APP_VERSION (Dockerfile, sem .git no contexto); o build local, como o
// do app desktop (`npm run package:win`), lê do git: "v0.17.5", ou
// "v0.17.5-1-gb07ec2d" fora de uma tag. Sem os dois, fica vazia e o menu
// esconde a linha.
function appVersion(): string {
  if (process.env.VITE_APP_VERSION) return process.env.VITE_APP_VERSION
  try {
    return execFileSync('git', ['describe', '--tags', '--match', 'client-v*'], { stdio: ['ignore', 'pipe', 'ignore'] })
      .toString()
      .trim()
      .replace(/^client-/, '')
  } catch {
    return ''
  }
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => ({
  // Explícito porque o vite-plugin-electron troca a base padrão por './'
  // quando ela não é definida. Com './', o retorno do login no Electron
  // (app://ffcom/auth/callback) buscava os assets em /auth/assets/ e recebia
  // o index.html do fallback no lugar do JS. Ver docs/architecture.md,
  // "Decisão: callback OIDC no Electron empacotado".
  base: '/',
  define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(appVersion()),
  },
  server: {
    watch: {
      // Saída do electron-builder: no Windows, o watcher do dev server
      // segura a pasta e o `npm run package:win` falha com EPERM ao renomear
      // release/win-unpacked.tmp.
      ignored: ['**/release/**'],
    },
  },
  plugins: [
    react(),
    thirdPartyLicenses(),
    mode === 'electron' &&
      electron({
        main: {
          entry: 'electron/main.ts',
          // Fora do main.js, carregados de node_modules em runtime: o
          // uiohook-napi por ser nativo (.node), o electron-updater por ler
          // arquivos do próprio pacote.
          vite: { build: { rolldownOptions: { external: ['uiohook-napi', 'electron-updater'] } } },
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
          // O aviso de licenças de terceiros (vite-plugin-third-party-licenses.ts)
          // abre numa aba nova, que é uma navegação: sem a exclusão, o
          // service worker responderia com a SPA em vez do arquivo.
          navigateFallbackDenylist: [/^\/api\//, /^\/auth\//, /^\/third-party-licenses\.txt$/],
        },
    }),
  ],
}))
