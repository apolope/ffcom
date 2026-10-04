import i18n, { type Resource, type ResourceLanguage } from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from '@locales/en.json'
import ptBR from '@locales/pt-BR.json'

// Internacionalização do client: i18next + react-i18next com os arquivos de
// locales/ na raiz embutidos no bundle (alias @locales). Ver
// docs/architecture.md, "Decisão: internacionalização".
//
// Idioma inicial: escolha salva na conta (aplicada por applyAccountLanguage
// quando o GET /api/me do server-central chega) → localStorage →
// navigator.languages (qualquer pt* vira pt-BR, o resto vira en) → pt-BR.

export const SUPPORTED_LANGUAGES = ['pt-BR', 'en'] as const
export type Language = (typeof SUPPORTED_LANGUAGES)[number]
export const DEFAULT_LANGUAGE: Language = 'pt-BR'

const STORAGE_KEY = 'ffcom.language'

export function isLanguage(value: unknown): value is Language {
  return typeof value === 'string' && (SUPPORTED_LANGUAGES as readonly string[]).includes(value)
}

function storedLanguage(): Language | undefined {
  try {
    const value = localStorage.getItem(STORAGE_KEY)
    return isLanguage(value) ? value : undefined
  } catch {
    return undefined
  }
}

function storeLanguage(lang: Language): void {
  try {
    localStorage.setItem(STORAGE_KEY, lang)
  } catch {
    // Sem localStorage (modo privado restrito): vale só nesta sessão.
  }
}

function browserLanguage(): Language | undefined {
  const preferred = typeof navigator === 'undefined' ? [] : (navigator.languages ?? [navigator.language])
  for (const tag of preferred) {
    if (!tag) continue
    return tag.toLowerCase().startsWith('pt') ? 'pt-BR' : 'en'
  }
  return undefined
}

// Plural "many": o Intl.PluralRules do português tem a forma `many`
// (1.000.000, 2.000.000...), e o i18next, sem `x_many`, não cai em `x_other`:
// mostra a própria chave. A regra é escrever `_many` em pt-BR (o
// check-locales avisa quando falta); como rede de proteção, a forma que o
// idioma usa e o arquivo não tem é preenchida aqui com o texto de `_other`.
function fillMissingPluralForms(lang: string, node: ResourceLanguage): ResourceLanguage {
  let categories: readonly string[] = []
  try {
    categories = new Intl.PluralRules(lang).resolvedOptions().pluralCategories
  } catch {
    return node
  }
  const fill = (obj: Record<string, unknown>): Record<string, unknown> => {
    const out: Record<string, unknown> = {}
    for (const [key, value] of Object.entries(obj)) {
      out[key] = value !== null && typeof value === 'object' ? fill(value as Record<string, unknown>) : value
    }
    for (const [key, value] of Object.entries(obj)) {
      if (!key.endsWith('_other') || typeof value !== 'string') continue
      const base = key.slice(0, -'_other'.length)
      for (const category of categories) {
        if (!(`${base}_${category}` in out)) out[`${base}_${category}`] = value
      }
    }
    return out
  }
  return fill(node as Record<string, unknown>) as ResourceLanguage
}

const resources: Resource = {
  'pt-BR': { translation: fillMissingPluralForms('pt-BR', ptBR) },
  en: { translation: fillMissingPluralForms('en', en) },
}

export function initialLanguage(): Language {
  return storedLanguage() ?? browserLanguage() ?? DEFAULT_LANGUAGE
}

function syncDocumentLanguage(lang: string): void {
  if (typeof document !== 'undefined') document.documentElement.lang = lang
  // App desktop: o main traduz a barra de menu e o que mais for nativo
  // (electron/i18n.ts).
  if (typeof window !== 'undefined') window.ffcomElectron?.setLanguage(lang)
}

// Chamado em main.tsx antes do render. Com os recursos embutidos e
// initAsync falso, o init termina na hora: a primeira tela já sai traduzida.
export function initI18n(): typeof i18n {
  if (i18n.isInitialized) return i18n
  i18n.on('languageChanged', syncDocumentLanguage)
  void i18n.use(initReactI18next).init({
    resources,
    lng: initialLanguage(),
    fallbackLng: DEFAULT_LANGUAGE,
    supportedLngs: [...SUPPORTED_LANGUAGES],
    // React já escapa o texto renderizado.
    interpolation: { escapeValue: false },
    initAsync: false,
    returnNull: false,
  })
  syncDocumentLanguage(i18n.language)
  return i18n
}

export function currentLanguage(): Language {
  return isLanguage(i18n.language) ? i18n.language : DEFAULT_LANGUAGE
}

// Troca feita pela pessoa (seletor nas configurações): muda a interface na
// hora e guarda no localStorage. Gravar na conta fica com quem chama
// (PATCH /api/me no server-central), que tem o token.
export function changeLanguage(lang: Language): Promise<unknown> {
  storeLanguage(lang)
  return i18n.changeLanguage(lang)
}

// Idioma salvo na conta (GET /api/me do server-central): tem precedência
// sobre o localStorage e o navegador, e passa a ser o do localStorage,
// para a próxima abertura já começar nele antes da resposta chegar.
export function applyAccountLanguage(lang: string | null | undefined): void {
  if (!isLanguage(lang)) return
  storeLanguage(lang)
  if (i18n.language !== lang) void i18n.changeLanguage(lang)
}

export default i18n
