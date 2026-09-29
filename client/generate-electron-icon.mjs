// Gera o ícone do app desktop (build/icon.png, 1024x1024) a partir do mesmo
// favicon.svg usado pelo web/PWA. O electron-builder converte esse PNG para
// .ico (Windows) e .icns (macOS) no empacotamento. Rodar via
// `npm run generate:electron-icon` sempre que o favicon.svg mudar; o PNG fica
// versionado, pelo mesmo motivo dos ícones do PWA (ver pwa-assets.config.ts).
import sharp from 'sharp'

await sharp('public/favicon.svg', { density: 1024 * 72 / 100 })
  .resize(1024, 1024)
  .png()
  .toFile('build/icon.png')
