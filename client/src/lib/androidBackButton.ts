import { isAndroidApp } from './platform'

// Botão "voltar" do Android no app. Com um listener de backButton, o
// Capacitor deixa de fazer o padrão dele (voltar na WebView ou fechar a
// Activity) e quem decide é este código:
// - se a WebView tem histórico, volta nele. É o que mantém o fechar das
//   gavetas: abrir uma gaveta empilha um estado e o popstate dela fecha
//   (App.tsx, client-v0.17.4);
// - sem nada para voltar, minimiza o app em vez de fechar a Activity, que
//   derrubaria uma chamada em andamento.
// No navegador não faz nada: o voltar continua sendo o do próprio navegador.
export async function setupAndroidBackButton(): Promise<void> {
  if (!isAndroidApp()) return
  const { App } = await import('@capacitor/app')
  await App.addListener('backButton', ({ canGoBack }) => {
    if (canGoBack) window.history.back()
    else void App.minimizeApp()
  })
}
