import { Capacitor } from '@capacitor/core'

// Onde o client está rodando. A mesma SPA serve o navegador (e o PWA), o app
// desktop (Electron) e o app Android (Capacitor carregando o client
// publicado; ver docs/architecture.md, "Decisão: app Android com Capacitor
// (fase 1)").

// App desktop: o preload do Electron expõe window.ffcomElectron (ver
// electron.d.ts), e o próprio teste `window.ffcomElectron` é a detecção.

// App Android: o Capacitor injeta a ponte nativa na página carregada pela
// WebView do app. No navegador, inclusive o Chrome do Android, é false.
export function isAndroidApp(): boolean {
  return Capacitor.isNativePlatform() && Capacitor.getPlatform() === 'android'
}
