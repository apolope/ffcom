#!/usr/bin/env node
// Confere os arquivos de idioma em locales/ (ver docs/architecture.md,
// "Decisão: internacionalização"). Node puro, sem dependências; roda no CI
// (ci.yml e deploys do client e do site) e localmente:
//
//   node scripts/check-locales.mjs
//
// Falha (saída 1) se:
//   1. algum locales/*.json não for JSON válido ou tiver valor que não é texto;
//   2. os idiomas tiverem conjuntos de chaves diferentes (plurais contam pelo
//      nome base: `x_one`/`x_other` em um idioma e `x_one`/`x_many`/`x_other`
//      em outro é a mesma chave `x`);
//   3. algum valor for vazio;
//   4. alguma chave usada no client (`t('...')`, `t("...")`, `i18nKey="..."`
//      em client/src e client/electron) faltar em pt-BR.json. Chave dinâmica
//      (template string) não é conferida. `x` existe se existir `x` ou o
//      plural `x_one`/`x_other`;
//   5. algum código de erro usado no Go (server-central, server-channel, fora
//      dos _test.go) faltar em `errors.*` de pt-BR.json, ou alguma chamada
//      do helper não tiver o código como string literal;
//   6. alguma chave usada no site faltar em pt-BR.json: `data-i18n="..."`,
//      `data-i18n-html="..."` e `data-i18n-attr="atributo:chave;..."` nos
//      site/*.html, e nos site/*.js (fora de assets/) `t('...')`, toda string
//      literal que é uma chave `site.*`/`errors.*` inteira (mapas de chaves,
//      `dataset.i18n = '...'`) e `code: '...'` (código de erro, conferido em
//      `errors.*`). Chave montada em template string não é conferida.
//
// Avisa, sem falhar, quando um plural não tem todas as formas que o idioma
// usa segundo Intl.PluralRules (ex. `_many` em pt-BR, usada em 1.000.000).
//
// Os códigos de erro do Go saem do helper `internal/apierr` de cada servidor
// (docs/protocol.md, "Erros da API HTTP"), sempre como string literal:
//   apierr.Write(w, http.StatusConflict, "friends.already_friends", "vocês já são amigos")
//   apierr.WriteParams(w, http.StatusBadRequest, "ideas.text_length", "...", apierr.Params{...})
//   apierr.New("signup.email_invalid", "e-mail inválido")
//   apierr.NewParams("signup.reason_length", "...", apierr.Params{...})
// ERROR_CALL_PATTERN acha cada chamada que leva código; ERROR_CODE_PATTERN,
// aplicado no mesmo ponto (flag `y`), pula os argumentos simples (`w`, o
// status) e tem no grupo 1 o código (`area.motivo`). Chamada que o segundo
// não casa (código em variável, argumento com parênteses antes do código) é
// erro, porque o código dela não seria conferido. Os testes Go
// internal/apierr/apierr_test.go do server-central e do server-channel usam
// os mesmos padrões; mudando um, mude os outros.
const ERROR_CALL_PATTERN = /\bapierr\.(?:Write|WriteParams|New|NewParams)\(/g
const ERROR_CODE_PATTERN =
  /apierr\.(?:Write|WriteParams|New|NewParams)\((?:\s*[\w.]+\s*,)*\s*"([a-z][a-z0-9_]*\.[a-z0-9_.]+)"/y

import { readFileSync, readdirSync, existsSync, statSync } from 'node:fs'
import { join, relative, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const localesDir = join(root, 'locales')
const referenceLang = 'pt-BR'
const pluralSuffix = /_(zero|one|two|few|many|other)$/

const errors = []
const warnings = []
const rel = (p) => relative(root, p).replaceAll('\\', '/')

// --- 1. leitura -------------------------------------------------------------

const langs = {}
for (const file of readdirSync(localesDir).filter((f) => f.endsWith('.json')).sort()) {
  const lang = file.slice(0, -'.json'.length)
  let data
  try {
    data = JSON.parse(readFileSync(join(localesDir, file), 'utf8'))
  } catch (e) {
    errors.push(`locales/${file}: JSON inválido (${e.message})`)
    continue
  }
  const flat = new Map()
  flatten(data, '', flat, file)
  langs[lang] = flat
}

function flatten(node, prefix, out, file) {
  for (const [k, v] of Object.entries(node)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
      flatten(v, key, out, file)
    } else if (typeof v !== 'string') {
      errors.push(`locales/${file}: "${key}" não é texto`)
    } else {
      out.set(key, v)
    }
  }
}

if (!langs[referenceLang]) {
  errors.push(`locales/${referenceLang}.json não existe (é o idioma de referência)`)
}

// --- 2 e 3. mesmas chaves, nenhum valor vazio --------------------------------

// Chave lógica: o nome base para plurais, a própria chave para o resto.
function logicalKeys(flat) {
  const keys = new Set()
  const plurals = new Map() // base -> Set(formas)
  for (const key of flat.keys()) {
    const m = key.match(pluralSuffix)
    if (m) {
      const base = key.slice(0, -m[0].length)
      keys.add(`${base} (plural)`)
      if (!plurals.has(base)) plurals.set(base, new Set())
      plurals.get(base).add(m[1])
    } else {
      keys.add(key)
    }
  }
  return { keys, plurals }
}

const logical = {}
for (const [lang, flat] of Object.entries(langs)) {
  logical[lang] = logicalKeys(flat)
  for (const [key, value] of flat) {
    if (value.trim() === '') errors.push(`locales/${lang}.json: "${key}" está vazio`)
  }
  let categories = []
  try {
    categories = new Intl.PluralRules(lang).resolvedOptions().pluralCategories
  } catch {
    warnings.push(`locales/${lang}.json: Intl.PluralRules não conhece "${lang}"`)
  }
  for (const [base, forms] of logical[lang].plurals) {
    for (const required of ['one', 'other']) {
      if (!forms.has(required)) errors.push(`locales/${lang}.json: plural "${base}" sem "${base}_${required}"`)
    }
    for (const c of categories) {
      if (c !== 'one' && c !== 'other' && !forms.has(c)) {
        warnings.push(`locales/${lang}.json: plural "${base}" sem "${base}_${c}" (forma "${c}" do ${lang})`)
      }
    }
  }
}

const ref = logical[referenceLang]
if (ref) {
  for (const [lang, { keys }] of Object.entries(logical)) {
    if (lang === referenceLang) continue
    for (const k of ref.keys) if (!keys.has(k)) errors.push(`locales/${lang}.json: falta "${k}" (existe em ${referenceLang})`)
    for (const k of keys) if (!ref.keys.has(k)) errors.push(`locales/${lang}.json: sobra "${k}" (não existe em ${referenceLang})`)
  }
}

const refFlat = langs[referenceLang] ?? new Map()
function hasKey(key) {
  return refFlat.has(key) || refFlat.has(`${key}_one`) || refFlat.has(`${key}_other`)
}

// --- 4. chaves usadas no client ---------------------------------------------

const clientPatterns = [
  /(?<![\w$])t\(\s*(['"])([^'"\n]+)\1/g,
  /\bi18nKey=\{?\s*(['"])([^'"\n]+)\1/g,
]
let clientKeys = 0
for (const dir of ['client/src', 'client/electron']) {
  for (const file of walk(join(root, dir), /\.(tsx?|jsx?|mjs)$/)) {
    const text = readFileSync(file, 'utf8')
    for (const re of clientPatterns) {
      for (const m of text.matchAll(re)) {
        clientKeys++
        const key = m[2]
        if (!hasKey(key)) errors.push(`${rel(file)}:${lineOf(text, m.index)}: chave "${key}" não existe em ${referenceLang}.json`)
      }
    }
  }
}

// --- 5. códigos de erro do Go -----------------------------------------------

let goCodes = 0
for (const dir of ['server-central', 'server-channel']) {
  for (const file of walk(join(root, dir), /\.go$/)) {
    if (file.endsWith('_test.go')) continue
    const text = readFileSync(file, 'utf8')
    for (const call of text.matchAll(ERROR_CALL_PATTERN)) {
      const at = call.index + call[0].indexOf('apierr')
      ERROR_CODE_PATTERN.lastIndex = at
      const m = ERROR_CODE_PATTERN.exec(text)
      if (!m) {
        errors.push(`${rel(file)}:${lineOf(text, at)}: chamada de apierr sem código literal no formato area.motivo`)
        continue
      }
      goCodes++
      const code = m[1]
      if (!refFlat.has(`errors.${code}`)) {
        errors.push(`${rel(file)}:${lineOf(text, at)}: código de erro "${code}" sem "errors.${code}" em ${referenceLang}.json`)
      }
    }
  }
}

// --- 6. chaves usadas no site -------------------------------------------------

const siteHtmlPatterns = [/\sdata-i18n(?:-html)?="([^"]+)"/g]
const siteJsPatterns = [
  { re: /(?<![\w$])t\(\s*(['"])([^'"\n]+)\1/g, key: (m) => m[2] },
  { re: /(['"])((?:site|errors)\.[A-Za-z0-9_.-]*[A-Za-z0-9_-])\1/g, key: (m) => m[2] },
  { re: /\bcode:\s*(['"])([a-z][a-z0-9_]*\.[a-z0-9_.]+)\1/g, key: (m) => `errors.${m[2]}` },
]
let siteKeys = 0
const siteDir = join(root, 'site')
const checkSiteKey = (file, text, index, key) => {
  // `t('errors.' + code)`: prefixo de chave dinâmica, não conferida.
  if (key.endsWith('.')) return
  siteKeys++
  if (!hasKey(key)) errors.push(`${rel(file)}:${lineOf(text, index)}: chave "${key}" não existe em ${referenceLang}.json`)
}
for (const file of walk(siteDir, /\.html$/)) {
  const text = readFileSync(file, 'utf8')
  for (const re of siteHtmlPatterns) {
    for (const m of text.matchAll(re)) checkSiteKey(file, text, m.index, m[1])
  }
  for (const m of text.matchAll(/\sdata-i18n-attr="([^"]+)"/g)) {
    for (const par of m[1].split(';')) {
      const [attr, key] = par.split(':').map((x) => x?.trim())
      if (!attr || !key) {
        errors.push(`${rel(file)}:${lineOf(text, m.index)}: data-i18n-attr "${par}" fora do formato atributo:chave`)
        continue
      }
      checkSiteKey(file, text, m.index, key)
    }
  }
}
for (const file of walk(siteDir, /\.js$/)) {
  if (rel(file).startsWith('site/assets/') || rel(file).startsWith('site/locales/')) continue
  const text = readFileSync(file, 'utf8')
  for (const { re, key } of siteJsPatterns) {
    for (const m of text.matchAll(re)) checkSiteKey(file, text, m.index, key(m))
  }
}

// --- resultado --------------------------------------------------------------

function* walk(dir, pattern) {
  if (!existsSync(dir)) return
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name.startsWith('.')) continue
    const p = join(dir, name)
    if (statSync(p).isDirectory()) yield* walk(p, pattern)
    else if (pattern.test(name)) yield p
  }
}

function lineOf(text, index) {
  return text.slice(0, index).split('\n').length
}

for (const w of warnings) console.log(`aviso: ${w}`)
for (const e of errors) console.log(`erro: ${e}`)
const summary = `${Object.keys(langs).length} idiomas (${Object.keys(langs).join(', ')}), ${refFlat.size} chaves em ${referenceLang}, ${clientKeys} usos de chave no client, ${siteKeys} no site, ${goCodes} códigos de erro no Go`
if (errors.length) {
  console.log(`\ncheck-locales: ${errors.length} erro(s). ${summary}`)
  process.exit(1)
}
console.log(`check-locales: ok. ${summary}`)
