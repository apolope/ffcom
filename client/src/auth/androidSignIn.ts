import type { PluginListenerHandle } from '@capacitor/core'
import { ANDROID_AUTH_CALLBACK_URL } from './config'
import { userManager } from './userManager'

// Ponte entre o login do app Android e o Custom Tab. A URL de autorização é
// montada na WebView (oidc-client-ts, com state e PKCE no localStorage dela,
// como no web) e aberta pelo @capacitor/browser. O Authentik devolve para
// https://app.ffcom.a3sitsolutions.com.br/auth/android, um App Link
// verificado: o Android entrega a URL ao MainActivity (singleTask, que fecha
// o Custom Tab ao voltar para frente) e o @capacitor/app avisa a página por
// appUrlOpen. Com o app fechado, a mesma URL vem de App.getLaunchUrl() na
// abertura. Ver docs/architecture.md, "Decisão: login do app Android no
// navegador do sistema (fase 3)".

export function isAndroidAuthCallbackUrl(url: string): boolean {
  try {
    const u = new URL(url)
    const expected = new URL(ANDROID_AUTH_CALLBACK_URL)
    return u.origin === expected.origin && u.pathname === expected.pathname && u.searchParams.has('state')
  } catch {
    return false
  }
}

// State de cada retorno já entregue nesta página. O getLaunchUrl devolve a
// mesma URL enquanto a Activity vive (inclusive depois de um reload pelo
// botão de atualizar), e o code é de uso único.
const handledStates = new Set<string>()

function claim(url: string): boolean {
  if (!isAndroidAuthCallbackUrl(url)) return false
  const state = new URL(url).searchParams.get('state') ?? ''
  if (handledStates.has(state)) return false
  handledStates.add(state)
  return true
}

// O Custom Tab aberto pelo login, até o @capacitor/browser avisar que fechou.
let customTabOpen = false

export async function openAndroidSignIn(url: string): Promise<void> {
  const { Browser } = await import('@capacitor/browser')
  await Browser.open({ url })
  customTabOpen = true
}

// O Custom Tab costuma fechar sozinho: o App Link traz o MainActivity
// (singleTask) para frente e o Android tira o que estava por cima dele. O
// Browser.close() fica de reserva para quando isso não acontece, e só com o
// tab ainda aberto: chamado depois que ele já fechou, o plugin abriria de
// novo a Activity transparente que controla o tab.
async function closeCustomTab() {
  await new Promise((resolve) => setTimeout(resolve, 500))
  if (!customTabOpen) return
  customTabOpen = false
  try {
    const { Browser } = await import('@capacitor/browser')
    await Browser.close()
  } catch (err) {
    console.error('ffcom: falha ao fechar o Custom Tab do login', err)
  }
}

// URL de retorno com que o app foi aberto (o login terminou com o processo
// morto), ou null. Cada state sai uma vez só, e só se ainda está guardado:
// depois de concluído, o oidc-client-ts o apaga, e uma reabertura do app pelo
// mesmo intent (recentes, depois de o Android matar o processo) não tenta de
// novo um code já usado.
export async function takeAndroidLaunchCallback(): Promise<string | null> {
  try {
    const { App } = await import('@capacitor/app')
    const launch = await App.getLaunchUrl()
    if (!launch?.url || !isAndroidAuthCallbackUrl(launch.url)) return null
    const state = new URL(launch.url).searchParams.get('state') ?? ''
    if (!(await userManager.settings.stateStore.get(state))) return null
    return claim(launch.url) ? launch.url : null
  } catch (err) {
    console.error('ffcom: falha ao ler a URL de abertura do app', err)
    return null
  }
}

// Avisa cada retorno do login que chega com o app aberto (onCallback) e o
// Custom Tab fechado sem login (onClosed). Devolve a função que remove os
// listeners.
export function listenAndroidSignIn(onCallback: (url: string) => void, onClosed: () => void): () => void {
  const handles: Promise<PluginListenerHandle>[] = []
  let removed = false
  void (async () => {
    const [{ App }, { Browser }] = await Promise.all([import('@capacitor/app'), import('@capacitor/browser')])
    if (removed) return
    handles.push(
      App.addListener('appUrlOpen', ({ url }) => {
        if (!claim(url)) return
        void closeCustomTab()
        onCallback(url)
      }),
      Browser.addListener('browserFinished', () => {
        customTabOpen = false
        onClosed()
      }),
    )
  })()
  return () => {
    removed = true
    for (const h of handles) void h.then((handle) => handle.remove())
  }
}
