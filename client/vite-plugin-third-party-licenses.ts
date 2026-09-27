import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import type { Plugin } from 'vite'

// Gera /third-party-licenses.txt no build com o texto de licença de cada
// pacote de produção do package-lock.json (o que não é `dev`), que é o que
// pode acabar no bundle servido ao navegador e empacotado no Electron. Pegar
// do lockfile em vez do grafo de módulos do Rollup inclui algum pacote a mais
// (ex. dependência só de tipos), o que não faz mal, e cobre os worklets e
// workers, que são builds separados. O menu do avatar (StatusMenu) aponta
// para o arquivo. Ver docs/architecture.md, "Decisão: licença AGPL-3.0".

const LICENSE_FILE = /^(licen[cs]e|copying|notice)([.-].*)?$/i

interface LockPackage {
  version?: string
  license?: string
  dev?: boolean
}

function buildNotices(root: string): string {
  const lock = JSON.parse(readFileSync(join(root, 'package-lock.json'), 'utf8')) as {
    packages: Record<string, LockPackage>
  }
  const sections: string[] = []
  const missing: string[] = []

  for (const [key, pkg] of Object.entries(lock.packages).sort(([a], [b]) => a.localeCompare(b))) {
    if (!key || pkg.dev) continue
    const dir = join(root, key)
    // Dependência opcional de outra plataforma (ex. binário nativo) não
    // instalada aqui: não está no build.
    if (!existsSync(dir)) continue
    const name = key.slice(key.lastIndexOf('node_modules/') + 'node_modules/'.length)
    const files = readdirSync(dir)
      .filter((f) => LICENSE_FILE.test(f))
      .sort()
    const license = pkg.license ?? 'licença não declarada'
    const header = `${name}@${pkg.version ?? '?'} ${license.startsWith('(') ? license : `(${license})`}`
    if (files.length === 0) missing.push(header)
    const body = files.length
      ? files.map((f) => readFileSync(join(dir, f), 'utf8').replace(/\r\n/g, '\n').trimEnd()).join('\n\n')
      : `O pacote não traz arquivo de licença; licença declarada no package.json: ${pkg.license ?? 'nenhuma'}.\n` +
        'Texto padrão de cada licença SPDX: https://spdx.org/licenses/<identificador>.html'
    sections.push(`${'='.repeat(80)}\n${header}\n${'='.repeat(80)}\n\n${body}\n`)
  }

  if (missing.length) {
    console.warn(`[third-party-licenses] sem arquivo de licença:\n  ${missing.join('\n  ')}`)
  }
  return [
    'Licenças de terceiros incluídas no client do FFCom.',
    'O FFCom é AGPL-3.0-or-later: https://github.com/apolope/ffcom',
    '',
    ...sections,
  ].join('\n')
}

export function thirdPartyLicenses(fileName = 'third-party-licenses.txt'): Plugin {
  let root = process.cwd()
  return {
    name: 'ffcom-third-party-licenses',
    apply: 'build',
    configResolved(config) {
      root = config.root
    },
    generateBundle() {
      this.emitFile({ type: 'asset', fileName, source: buildNotices(root) })
    },
  }
}
