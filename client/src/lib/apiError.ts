import { useTranslation } from 'react-i18next'
import i18n from '../i18n'

// Erros da API HTTP e do frame `error` do WebSocket, nos dois servidores:
// {"code": "area.motivo", "message": "texto em português", "params"?: {...}}
// (docs/protocol.md, "Erros da API HTTP" e "Frame `error` do WebSocket").
// O client mostra a chave errors.<code> com os params e cai na message quando o
// código não tem tradução (servidor mais novo que o client).
export interface ApiProblem {
  code?: string
  message: string
  params?: Record<string, unknown>
}

const CODE_PATTERN = /^[a-z][a-z0-9_]*\.[a-z0-9_.]+$/

// translateApiError traduz no idioma ativo na hora da chamada. Quem guarda
// o erro em estado deve guardar o ApiProblem (ou o ApiError) e chamar isto
// no render, para o texto acompanhar uma troca de idioma.
export function translateApiError(problem: ApiProblem): string {
  const { code, params, message } = problem
  if (code && CODE_PATTERN.test(code)) {
    const key = `errors.${code}`
    if (i18n.exists(key)) {
      const text = i18n.t(key, { ...params })
      if (typeof text === 'string' && text !== key) return text
    }
  }
  return message
}

// ApiError é a resposta HTTP fora de 2xx. `message` é calculada a cada
// leitura (translateApiError), então `err.message` sai no idioma ativo em
// que for lida, inclusive em código antigo que só conhece Error.
export class ApiError extends Error {
  readonly status: number
  readonly code?: string
  readonly params?: Record<string, unknown>
  // message do servidor (português) ou, sem JSON, o texto cru da resposta
  // ou "<status> <statusText>".
  readonly serverMessage: string

  constructor(status: number, problem: ApiProblem) {
    super()
    this.name = 'ApiError'
    this.status = status
    this.code = problem.code
    this.params = problem.params
    this.serverMessage = problem.message
    Object.defineProperty(this, 'message', {
      get: () => translateApiError(this.problem),
      configurable: true,
      enumerable: false,
    })
  }

  get problem(): ApiProblem {
    return { code: this.code, message: this.serverMessage, params: this.params }
  }
}

// parseProblem lê o corpo {code, message, params?}; undefined se não for
// nesse formato.
export function parseProblem(value: unknown): ApiProblem | undefined {
  if (value === null || typeof value !== 'object') return undefined
  const { code, message, params } = value as Record<string, unknown>
  if (typeof message !== 'string' && typeof code !== 'string') return undefined
  return {
    code: typeof code === 'string' ? code : undefined,
    message: typeof message === 'string' ? message : String(code),
    params: params !== null && typeof params === 'object' ? (params as Record<string, unknown>) : undefined,
  }
}

// apiErrorFromResponse monta o ApiError de uma resposta fora de 2xx. Corpo
// que não é JSON no formato (proxy na frente, rota inexistente, servidor
// antigo) vira o texto cru; corpo vazio vira "<label>: <status> <statusText>".
export async function apiErrorFromResponse(res: Response, label?: string): Promise<ApiError> {
  const text = (await res.text().catch(() => '')).trim()
  let problem: ApiProblem | undefined
  if (text.startsWith('{')) {
    try {
      problem = parseProblem(JSON.parse(text))
    } catch {
      problem = undefined
    }
  }
  const fallback = `${label ? `${label}: ` : ''}${res.status} ${res.statusText}`.trim()
  return new ApiError(res.status, problem ?? { message: text || fallback })
}

// errorMessage é o texto para mostrar de qualquer erro: ApiError e frame
// `error` traduzidos pelo código, Error pela message, o resto pelo fallback.
export function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error) return err.message || fallback
  const problem = parseProblem(err)
  if (problem) return translateApiError(problem)
  return typeof err === 'string' && err ? err : fallback
}

// problemFromFrame lê o frame `error` do WebSocket: code/message/params, ou
// só o campo antigo `error` (servidor anterior ao code).
export function problemFromFrame(frame: { code?: string; message?: string; params?: Record<string, unknown>; error?: string }): ApiProblem {
  return {
    code: frame.code,
    message: frame.message ?? frame.error ?? '',
    params: frame.params,
  }
}

// Erro guardado em estado para mostrar depois: o ApiProblem de um frame
// `error`, o Error capturado (um ApiError traduz a cada leitura) ou um texto.
// Guardar isto, e não o texto pronto, deixa a mensagem mudar junto com o
// idioma.
export type DisplayError = ApiProblem | Error | string

// useErrorText devolve o texto do erro no idioma ativo e renderiza de novo
// quando o idioma muda (useTranslation assina o languageChanged).
export function useErrorText(error: DisplayError | undefined): string | undefined {
  useTranslation()
  return error === undefined ? undefined : errorMessage(error, '')
}

// LocalizedError é um erro do próprio client (validação, falha local) cujo
// texto sai do arquivo de idioma. Como no ApiError, `message` é calculada a
// cada leitura, então o erro guardado em estado acompanha a troca de idioma.
// Recebe uma função (ex. `() => i18n.t('e2e.phraseIncorrect')`) para a
// chave continuar literal e conferida pelo check-locales.
export class LocalizedError extends Error {
  constructor(text: () => string) {
    super()
    this.name = 'LocalizedError'
    Object.defineProperty(this, 'message', { get: text, configurable: true, enumerable: false })
  }
}
