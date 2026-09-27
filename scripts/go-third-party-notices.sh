#!/usr/bin/env bash
# Gera o THIRD_PARTY_NOTICES.txt de um binário Go do FFCom: o texto de
# licença de cada módulo de terceiro que entra no binário (go list -deps do
# pacote main, para linux, sem cgo, como os builds de release) mais o da
# própria stdlib do Go. O arquivo é versionado e embutido no binário
# (go:embed), que o imprime com --licenses; o CI refaz e recusa diferença.
# Ver docs/architecture.md, "Decisão: licença AGPL-3.0".
#
# Uso (a partir da raiz do repo):
#   scripts/go-third-party-notices.sh <diretório do módulo> <pacote main>
# Exemplos:
#   scripts/go-third-party-notices.sh server-central .
#   scripts/go-third-party-notices.sh server-channel .
#   scripts/go-third-party-notices.sh server-channel ./cmd/ffcom-runtime
set -euo pipefail

module_dir=${1:?diretório do módulo}
pkg=${2:?pacote main}

cd "$module_dir"
out="$pkg/THIRD_PARTY_NOTICES.txt"

license_files() {
  find "$1" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'LICENCE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \) | LC_ALL=C sort
}

emit() { # nome, diretório
  local files
  files=$(license_files "$2")
  if [ -z "$files" ]; then
    echo "sem arquivo de licença em $1 ($2)" >&2
    exit 1
  fi
  printf '================================================================================\n%s\n================================================================================\n\n' "$1"
  while IFS= read -r f; do
    tr -d '\r' < "$f"
    printf '\n'
  done <<< "$files"
}

{
  printf 'Licenças de terceiros incluídas neste binário do FFCom.\n'
  printf 'O FFCom é AGPL-3.0-or-later: https://github.com/apolope/ffcom\n'
  printf 'Arquivo gerado por scripts/go-third-party-notices.sh; não edite à mão.\n\n'
  emit "Go (biblioteca padrão)" "$(go env GOROOT)"
  # Com replace, o código que entra é o do módulo substituto.
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go list -deps \
    -f '{{with .Module}}{{if not .Main}}{{with .Replace}}{{.Path}} {{.Version}} {{.Dir}}{{else}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}{{end}}' \
    "$pkg" | LC_ALL=C sort -u |
    while read -r path ver dir; do
      emit "$path $ver" "$dir"
    done
} > "$out"

echo "gerado: $module_dir/$out" >&2
