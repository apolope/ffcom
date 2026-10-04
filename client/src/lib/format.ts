import i18n from '../i18n'

// Datas, horas e números no idioma ativo da interface (i18n.language), nunca
// num locale fixo. Quem chama no render de um componente deve usar
// useTranslation() (ou receber `t`) para renderizar de novo quando o idioma
// muda; o `locale` opcional serve para forçar um idioma específico.

type DateInput = Date | string | number

const cache = new Map<string, Intl.DateTimeFormat | Intl.NumberFormat>()

function activeLocale(locale?: string): string {
  return locale ?? i18n.language ?? 'pt-BR'
}

function dateFormatter(locale: string, options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = `d|${locale}|${JSON.stringify(options)}`
  let formatter = cache.get(key) as Intl.DateTimeFormat | undefined
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(locale, options)
    cache.set(key, formatter)
  }
  return formatter
}

function numberFormatter(locale: string, options: Intl.NumberFormatOptions): Intl.NumberFormat {
  const key = `n|${locale}|${JSON.stringify(options)}`
  let formatter = cache.get(key) as Intl.NumberFormat | undefined
  if (!formatter) {
    formatter = new Intl.NumberFormat(locale, options)
    cache.set(key, formatter)
  }
  return formatter
}

function toDate(value: DateInput): Date | undefined {
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? undefined : date
}

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
}

// Só a hora: "14:32" (pt-BR), "2:32 PM" (en).
export function formatTime(value: DateInput, locale?: string): string {
  const date = toDate(value)
  return date ? dateFormatter(activeLocale(locale), { timeStyle: 'short' }).format(date) : ''
}

// Só a data, curta: "03/10/2026" (pt-BR), "10/3/26" (en).
export function formatDate(value: DateInput, locale?: string): string {
  const date = toDate(value)
  return date ? dateFormatter(activeLocale(locale), { dateStyle: 'short' }).format(date) : ''
}

// Data e hora completas, para title/tooltip: "3 de out. de 2026 14:32".
export function formatDateTime(value: DateInput, locale?: string): string {
  const date = toDate(value)
  return date ? dateFormatter(activeLocale(locale), { dateStyle: 'medium', timeStyle: 'short' }).format(date) : ''
}

// Carimbo de mensagem: só a hora quando é de hoje, data curta e hora quando
// é de outro dia.
export function formatMessageTime(value: DateInput, now: Date = new Date(), locale?: string): string {
  const date = toDate(value)
  if (!date) return ''
  const options: Intl.DateTimeFormatOptions = sameDay(date, now)
    ? { timeStyle: 'short' }
    : { dateStyle: 'short', timeStyle: 'short' }
  return dateFormatter(activeLocale(locale), options).format(date)
}

// Número com separadores do idioma: 1.234 (pt-BR), 1,234 (en).
export function formatNumber(value: number, options: Intl.NumberFormatOptions = {}, locale?: string): string {
  return numberFormatter(activeLocale(locale), options).format(value)
}
