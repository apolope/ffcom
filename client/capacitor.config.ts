import type { CapacitorConfig } from '@capacitor/cli'

// App Android (Capacitor). A WebView carrega o client publicado, não os
// arquivos de dist/ embutidos no APK: a parte web se atualiza a cada deploy
// de client-v*, como o navegador e o PWA, e o APK só precisa mudar quando a
// casca nativa muda. O dist/ copiado pelo `cap sync` fica no APK só como
// fallback do Capacitor. Ver docs/android-runbook.md e docs/architecture.md,
// "Decisão: app Android com Capacitor (fase 1)".
//
// O Android mínimo (API 29) fica em android/variables.gradle, que é onde o
// Capacitor lê o minSdkVersion.
const config: CapacitorConfig = {
  appId: 'br.com.a3sitsolutions.ffcom',
  appName: 'FFCom',
  webDir: 'dist',
  server: {
    url: 'https://app.ffcom.a3sitsolutions.com.br',
    // Só o próprio domínio navega dentro da WebView; qualquer outro endereço
    // abre fora do app. O login no Authentik passa para o navegador do
    // sistema na fase 3 do runbook.
    allowNavigation: ['app.ffcom.a3sitsolutions.com.br'],
  },
}

export default config
