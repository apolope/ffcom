// Textos do processo main (barra de menu e o que mais for nativo) no idioma
// escolhido no renderer. Ver docs/architecture.md, "Decisão:
// internacionalização". Os arquivos são os mesmos locales/*.json do client,
// embutidos no main.js pelo alias @locales (repetido na config do main em
// vite.config.ts, porque o vite-plugin-electron não herda a config raiz).
//
// O idioma vem do renderer (setLanguage no preload), que o resolve pela conta,
// pelo localStorage e pelo navegador. Antes disso (no boot, ou com o app
// aberto só para receber o retorno do login) vale o último recebido, gravado
// em userData/language.json, e, sem ele, o idioma do sistema (pt* vira pt-BR,
// o resto vira en), a mesma regra do renderer.
import { app } from 'electron'
import { readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import en from '@locales/en.json' with { type: 'json' }
import ptBR from '@locales/pt-BR.json' with { type: 'json' }

export const SUPPORTED_LANGUAGES = ['pt-BR', 'en'] as const
export type Language = (typeof SUPPORTED_LANGUAGES)[number]
const DEFAULT_LANGUAGE: Language = 'pt-BR'

type Messages = { [key: string]: string | Messages }
const resources: Record<Language, Messages> = { 'pt-BR': ptBR, en }

export function isLanguage(value: unknown): value is Language {
  return typeof value === 'string' && (SUPPORTED_LANGUAGES as readonly string[]).includes(value)
}

function languageFile(): string {
  return path.join(app.getPath('userData'), 'language.json')
}

function storedLanguage(): Language | undefined {
  try {
    const value: unknown = JSON.parse(readFileSync(languageFile(), 'utf8'))?.language
    return isLanguage(value) ? value : undefined
  } catch {
    return undefined
  }
}

// Só vale depois do app ready (app.getLocale).
function systemLanguage(): Language {
  const tag = app.getPreferredSystemLanguages()[0] || app.getLocale()
  if (!tag) return DEFAULT_LANGUAGE
  return tag.toLowerCase().startsWith('pt') ? 'pt-BR' : 'en'
}

let language: Language | undefined

export function currentLanguage(): Language {
  language ??= storedLanguage() ?? systemLanguage()
  return language
}

// Idioma recebido do renderer. Devolve true se mudou (para quem chama
// reconstruir o que é nativo).
// Grava sempre que difere do arquivo, mesmo quando coincide com o idioma do
// sistema, para a escolha valer se o sistema mudar depois.
export function setLanguage(lang: Language): boolean {
  const changed = lang !== currentLanguage()
  language = lang
  if (storedLanguage() !== lang) {
    try {
      writeFileSync(languageFile(), JSON.stringify({ language: lang }))
    } catch (err) {
      console.warn('[ffcom] não foi possível guardar o idioma', err)
    }
  }
  return changed
}

function lookup(messages: Messages, key: string): string | undefined {
  let node: string | Messages | undefined = messages
  for (const part of key.split('.')) {
    if (node === undefined || typeof node === 'string') return undefined
    node = node[part]
  }
  return typeof node === 'string' ? node : undefined
}

// Chave do arquivo de idioma, com interpolação {{nome}} do i18next. Sem a
// chave no idioma ativo, cai no pt-BR e, por fim, na própria chave.
export function t(key: string, params: Record<string, string | number> = {}): string {
  const text = lookup(resources[currentLanguage()], key) ?? lookup(resources[DEFAULT_LANGUAGE], key) ?? key
  return text.replace(/\{\{\s*(\w+)\s*\}\}/g, (match, name: string) =>
    name in params ? String(params[name]) : match,
  )
}
