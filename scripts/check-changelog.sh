#!/usr/bin/env bash
# Confere que a tag de release tem entrada no CHANGELOG.md, no formato que a
# home page lê ("## <componente> v<X.Y.Z> · <AAAA-MM-DD>" seguida de pelo
# menos um item "- ..."). Uso: scripts/check-changelog.sh <tag>, com a tag no
# formato dos workflows (client-v0.14.1, channel-image-v1.0.0, ...). Sem tag
# (workflow_dispatch fora de uma tag) não há versão para conferir.
set -euo pipefail

tag="${1:-}"
if [ -z "$tag" ]; then
  echo "Sem tag de release; nada a conferir no CHANGELOG."
  exit 0
fi

if [[ ! "$tag" =~ ^(client|central|channel|channel-image|site)-v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "::error::Tag '$tag' fora do formato <componente>-vX.Y.Z"
  exit 1
fi
comp="${BASH_REMATCH[1]}"
ver="${BASH_REMATCH[2]}"
file="$(dirname "$0")/../CHANGELOG.md"

header="^## ${comp} v${ver//./\.} · [0-9]{4}-[0-9]{2}-[0-9]{2}$"
if ! grep -qE "$header" "$file"; then
  echo "::error file=CHANGELOG.md::Falta a entrada '## ${comp} v${ver} · AAAA-MM-DD' no CHANGELOG.md. Adicione o que esta versão implementou antes de criar a tag."
  exit 1
fi

# A seção precisa de pelo menos um item antes da próxima seção.
items=$(awk -v h="$header" '
  $0 ~ h { dentro = 1; next }
  dentro && /^## / { exit }
  dentro && /^- / { n++ }
  END { print n + 0 }
' "$file")
if [ "$items" -eq 0 ]; then
  echo "::error file=CHANGELOG.md::A entrada de ${comp} v${ver} no CHANGELOG.md não tem nenhum item."
  exit 1
fi

echo "CHANGELOG.md tem a entrada de ${comp} v${ver} (${items} item(ns))."
