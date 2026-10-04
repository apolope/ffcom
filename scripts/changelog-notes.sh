#!/usr/bin/env bash
# Imprime os itens da entrada de uma tag no CHANGELOG.md e no CHANGELOG.en.md,
# para virar a descrição da release no GitHub (o mesmo texto que a home page
# mostra): português primeiro, depois um divisor e a versão em inglês, para
# quem chega pelo GitHub sem ler português. Uso: scripts/changelog-notes.sh
# <tag>, no formato de scripts/check-changelog.sh (client-v0.19.0,
# channel-v0.8.1, ...). Falha se a entrada não existir em algum dos dois.
set -euo pipefail

tag="${1:-}"
if [[ ! "$tag" =~ ^(client|central|channel|channel-image|site)-v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "Tag '$tag' fora do formato <componente>-vX.Y.Z" >&2
  exit 1
fi
comp="${BASH_REMATCH[1]}"
ver="${BASH_REMATCH[2]}"
root="$(dirname "$0")/.."
header="^## ${comp} v${ver//./\.} · [0-9]{4}-[0-9]{2}-[0-9]{2}$"

itens() {
  local notes
  notes=$(awk -v h="$header" '
    { sub(/\r$/, "") }
    $0 ~ h { dentro = 1; next }
    dentro && /^## / { exit }
    dentro && /^- / { print }
  ' "$root/$1")
  if [ -z "$notes" ]; then
    echo "Sem entrada de ${comp} v${ver} no $1" >&2
    return 1
  fi
  printf '%s\n' "$notes"
}

pt=$(itens CHANGELOG.md)
en=$(itens CHANGELOG.en.md)
repo=https://github.com/apolope/ffcom/blob/main
site=https://ffcom.a3sitsolutions.com.br/#versoes

printf '%s\n\nHistórico completo: [CHANGELOG.md](%s/CHANGELOG.md) · [ffcom.a3sitsolutions.com.br](%s)\n\n---\n\n### English\n\n%s\n\nFull history: [CHANGELOG.en.md](%s/CHANGELOG.en.md) · [ffcom.a3sitsolutions.com.br](%s)\n' \
  "$pt" "$repo" "$site" "$en" "$repo" "$site"
