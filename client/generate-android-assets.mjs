// Gera o ícone e a splash do app Android (client/android/app/src/main/res)
// a partir do mesmo favicon.svg usado pelo web/PWA e pelo desktop, com o
// @capacitor/assets no modo "logo": ícone adaptativo com o logo sobre fundo
// branco e splash com o logo no centro (fundo claro e escuro, as cores de
// --bg do index.css). O SVG vira um PNG de 1024 px antes, porque o
// @capacitor/assets rasteriza SVG no tamanho do viewBox (100 px) e o
// resultado sairia borrado. Rodar via `npm run generate:android-assets`
// sempre que o favicon.svg mudar; os PNGs ficam versionados, pelo mesmo
// motivo dos ícones do PWA (ver pwa-assets.config.ts).
import { execFileSync } from 'node:child_process'
import { mkdirSync, rmSync } from 'node:fs'
import sharp from 'sharp'

// Caminho relativo à raiz do client: o @capacitor/assets junta o --assetPath
// à raiz do projeto e não aceita caminho absoluto.
const assetPath = 'node_modules/.tmp/android-assets'
rmSync(assetPath, { recursive: true, force: true })
mkdirSync(assetPath, { recursive: true })

// O logo ocupa 60% do quadro, com margem transparente: o @capacitor/assets
// estica o logo até a borda da camada da frente do ícone adaptativo, e a
// máscara do launcher (círculo, gota) só mostra os 66% do centro dela.
const logo = await sharp('public/favicon.svg', { density: 614 * 72 / 100 })
  .resize(614, 614)
  .png()
  .toBuffer()
await sharp({ create: { width: 1024, height: 1024, channels: 4, background: { r: 0, g: 0, b: 0, alpha: 0 } } })
  .composite([{ input: logo, gravity: 'center' }])
  .png()
  .toFile(`${assetPath}/logo.png`)

execFileSync(
  'npx',
  [
    'capacitor-assets',
    'generate',
    '--android',
    '--assetPath', assetPath,
    '--iconBackgroundColor', '#ffffff',
    // Compensa a margem do logo.png: o logo fica com ~18% da largura da tela.
    '--logoSplashScale', '0.3',
    '--splashBackgroundColor', '#ffffff',
    '--splashBackgroundColorDark', '#16171d',
  ],
  { stdio: 'inherit', shell: process.platform === 'win32' },
)
