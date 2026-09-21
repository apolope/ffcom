import { defineConfig, minimal2023Preset } from '@vite-pwa/assets-generator/config'

// Gera os ícones do manifest (192/512/maskable/apple-touch-icon/favicon.ico)
// a partir do favicon.svg existente. Rodar via `npm run generate:pwa-assets`
// sempre que o favicon.svg mudar — os PNGs gerados ficam versionados em
// public/, não são gerados no build (evita depender de resvg/sharp no
// pipeline de deploy).
export default defineConfig({
  preset: minimal2023Preset,
  images: ['public/favicon.svg'],
})
