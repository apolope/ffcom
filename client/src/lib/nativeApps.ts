import { isAndroidApp } from './platform'

// Apps nativos do FFCom que o client oferece para instalar quando aberto no
// navegador (ou no PWA instalado), pelo botão do ServerRail
// (components/InstallAppButton.tsx). Ver docs/architecture.md, "Decisão:
// botão de instalar o app nativo pelo navegador".
//
// Para oferecer um sistema novo (Linux, macOS, iOS), basta uma entrada em
// NATIVE_APPS com o link de download estável (release fixa no GitHub, como
// android-stable e desktop-stable) e as chaves de texto em locales/.
// Sistema sem entrada não mostra botão nem item de menu.

export type NativePlatform = 'android' | 'windows' | 'macos' | 'linux' | 'ios'

export interface NativeApp {
  platform: NativePlatform
  // Nome do sistema como aparece no botão ("Instalar o app para Android").
  // Nome próprio, igual nos dois idiomas.
  label: string
  // Link estável do instalador mais recente.
  url: string
  // Como o clique segue:
  // - 'confirm': explica antes de baixar (o APK pede liberações no Android);
  // - 'direct': baixa na hora e mostra a nota enquanto o download corre.
  flow: 'confirm' | 'direct'
  // Título e explicação do diálogo (chaves em locales/).
  titleKey: string
  noteKey: string
  // Pacote do app no Android, para esconder o botão quando ele já está
  // instalado (navigator.getInstalledRelatedApps, só no Chrome do Android).
  // Precisa casar com related_applications em vite.config.ts e com o
  // asset_statements do AndroidManifest.xml.
  relatedAppId?: string
}

export const ANDROID_PACKAGE_ID = 'br.com.a3sitsolutions.ffcom'

export const NATIVE_APPS: Partial<Record<NativePlatform, NativeApp>> = {
  android: {
    platform: 'android',
    label: 'Android',
    url: 'https://github.com/apolope/ffcom/releases/download/android-stable/FFCom.apk',
    flow: 'confirm',
    titleKey: 'installApp.android.title',
    noteKey: 'installApp.android.note',
    relatedAppId: ANDROID_PACKAGE_ID,
  },
  windows: {
    platform: 'windows',
    label: 'Windows',
    // Só x64 (electron-builder sem arch, ver package.json): o Windows em
    // ARM roda pela emulação de x64.
    url: 'https://github.com/apolope/ffcom/releases/download/desktop-stable/FFCom-Setup.exe',
    flow: 'direct',
    titleKey: 'installApp.windows.title',
    noteKey: 'installApp.windows.note',
  },
}

interface UserAgentDataLike {
  platform?: string
}

// Sistema de quem visita: navigator.userAgentData.platform (Chromium) quando
// existe, senão o user agent. iPadOS se apresenta como Mac; o toque separa os
// dois. Chrome OS e o que não for reconhecido ficam sem sistema.
export function detectPlatform(): NativePlatform | undefined {
  if (typeof navigator === 'undefined') return undefined
  const uaData = (navigator as Navigator & { userAgentData?: UserAgentDataLike }).userAgentData
  const hint = uaData?.platform?.toLowerCase()
  if (hint) {
    if (hint === 'android') return 'android'
    if (hint === 'windows') return 'windows'
    if (hint === 'macos') return 'macos'
    if (hint === 'ios') return 'ios'
    if (hint === 'linux') return 'linux'
    if (hint !== 'unknown' && hint !== '') return undefined
  }
  const ua = navigator.userAgent
  if (/Android/i.test(ua)) return 'android'
  if (/iPhone|iPad|iPod/i.test(ua)) return 'ios'
  if (/Windows/i.test(ua)) return 'windows'
  if (/Macintosh|Mac OS X/i.test(ua)) return navigator.maxTouchPoints > 1 ? 'ios' : 'macos'
  if (/CrOS/i.test(ua)) return undefined
  if (/Linux/i.test(ua)) return 'linux'
  return undefined
}

// Dentro de um app nativo o botão nunca aparece: o desktop (Electron) e o
// app Android (Capacitor) já são o app. O PWA instalado continua recebendo a
// oferta, porque não tem push, chamada em segundo plano nem autoatualização,
// e disputa os App Links com o APK.
export function isInsideNativeApp(): boolean {
  return !!window.ffcomElectron || isAndroidApp()
}

// O app a oferecer para quem visita agora, ou undefined.
export function nativeAppForVisitor(): NativeApp | undefined {
  if (isInsideNativeApp()) return undefined
  const platform = detectPlatform()
  return platform ? NATIVE_APPS[platform] : undefined
}

interface RelatedApp {
  platform: string
  id?: string
}

// Se o app já está instalado no aparelho. Só o Chrome do Android responde
// (getInstalledRelatedApps, Chrome 80+), e só para um APK que declara
// asset_statements apontando para o site; vale também para APK instalado fora
// da Play Store. Qualquer outro caso (API ausente, erro) responde false.
export async function isNativeAppInstalled(app: NativeApp): Promise<boolean> {
  if (!app.relatedAppId) return false
  const getInstalled = (navigator as Navigator & { getInstalledRelatedApps?: () => Promise<RelatedApp[]> })
    .getInstalledRelatedApps
  if (typeof getInstalled !== 'function') return false
  try {
    const apps = await getInstalled.call(navigator)
    return apps.some((related) => related.id === app.relatedAppId)
  } catch {
    return false
  }
}

// Inicia o download com um link de verdade (o GitHub responde como anexo, e
// a página não sai do lugar).
export function startNativeAppDownload(app: NativeApp): void {
  const link = document.createElement('a')
  link.href = app.url
  link.rel = 'noopener'
  document.body.appendChild(link)
  link.click()
  link.remove()
}
