#!/usr/bin/env bash
# Imprime os itens da entrada de uma tag no CHANGELOG.md, para virar a
# descrição da release no GitHub (o mesmo texto que a home page mostra). Uso:
# scripts/changelog-notes.sh <tag>, no formato de scripts/check-changelog.sh
# (client-v0.19.0, channel-v0.8.1, ...). Falha se a entrada não existir.
set -euo pipefail

tag="${1:-}"
if [[ ! "$tag" =~ ^(client|central|channel|channel-image|site)-v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "Tag '$tag' fora do formato <componente>-vX.Y.Z" >&2
  exit 1
fi
comp="${BASH_REMATCH[1]}"
ver="${BASH_REMATCH[2]}"
file="$(dirname "$0")/../CHANGELOG.md"

header="^## ${comp} v${ver//./\.} · [0-9]{4}-[0-9]{2}-[0-9]{2}$"
notes=$(awk -v h="$header" '
  { sub(/\r$/, "") }
  $0 ~ h { dentro = 1; next }
  dentro && /^## / { exit }
  dentro && /^- / { print }
' "$file")
if [ -z "$notes" ]; then
  echo "Sem entrada de ${comp} v${ver} no CHANGELOG.md" >&2
  exit 1
fi

printf '%s\n\nHistórico completo: [CHANGELOG.md](https://github.com/apolope/ffcom/blob/main/CHANGELOG.md) · [ffcom.a3sitsolutions.com.br](https://ffcom.a3sitsolutions.com.br/#versoes)\n' "$notes"
