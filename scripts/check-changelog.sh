#!/usr/bin/env bash
# Confere que a tag de release tem entrada no CHANGELOG.md (português, a
# fonte) e no CHANGELOG.en.md (a tradução), no formato que a home page lê
# ("## <componente> v<X.Y.Z> · <AAAA-MM-DD>" seguida de pelo menos um item
# "- ..."), e que os dois arquivos têm as mesmas seções, na mesma ordem e com
# as mesmas datas. Uso: scripts/check-changelog.sh <tag>, com a tag no
# formato dos workflows (client-v0.14.1, channel-image-v1.0.0, ...). Sem tag
# (workflow_dispatch fora de uma tag) só confere que os dois arquivos batem.
set -euo pipefail

root="$(dirname "$0")/.."
files=(CHANGELOG.md CHANGELOG.en.md)
falhou=0

for f in "${files[@]}"; do
  if [ ! -f "$root/$f" ]; then
    echo "::error file=$f::$f não existe."
    exit 1
  fi
done

# Lista dos títulos de seção de versão, na ordem do arquivo (sem o \r de um
# checkout com CRLF).
secoes() {
  awk '{ sub(/\r$/, "") } /^## [a-z-]+ v[0-9]+\.[0-9]+\.[0-9]+ · [0-9]{4}-[0-9]{2}-[0-9]{2}$/ { print }' "$root/$1"
}

pt=$(secoes CHANGELOG.md)
en=$(secoes CHANGELOG.en.md)
if [ "$pt" != "$en" ]; then
  echo "::error file=CHANGELOG.en.md::CHANGELOG.md e CHANGELOG.en.md não têm as mesmas seções na mesma ordem. Diferenças (< português, > inglês):"
  diff <(printf '%s\n' "$pt") <(printf '%s\n' "$en") || true
  falhou=1
fi

tag="${1:-}"
if [ -z "$tag" ]; then
  if [ "$falhou" -ne 0 ]; then exit 1; fi
  echo "Sem tag de release; CHANGELOG.md e CHANGELOG.en.md têm as mesmas $(printf '%s\n' "$pt" | grep -c .) seções."
  exit 0
fi

if [[ ! "$tag" =~ ^(client|central|channel|channel-image|site)-v([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  echo "::error::Tag '$tag' fora do formato <componente>-vX.Y.Z"
  exit 1
fi
comp="${BASH_REMATCH[1]}"
ver="${BASH_REMATCH[2]}"
header="^## ${comp} v${ver//./\.} · [0-9]{4}-[0-9]{2}-[0-9]{2}$"

declare -A itens
for f in "${files[@]}"; do
  # A seção precisa existir e ter pelo menos um item antes da próxima seção.
  n=$(awk -v h="$header" '
    { sub(/\r$/, "") }
    $0 ~ h { achou = 1; dentro = 1; next }
    dentro && /^## / { exit }
    dentro && /^- / { n++ }
    END { print (achou ? n + 0 : -1) }
  ' "$root/$f")
  if [ "$n" -lt 0 ]; then
    echo "::error file=$f::Falta a entrada '## ${comp} v${ver} · AAAA-MM-DD' no $f. Adicione o que esta versão implementou nos dois arquivos (português e inglês) antes de criar a tag."
    falhou=1
  elif [ "$n" -eq 0 ]; then
    echo "::error file=$f::A entrada de ${comp} v${ver} no $f não tem nenhum item."
    falhou=1
  fi
  itens[$f]=$n
done

if [ "$falhou" -ne 0 ]; then exit 1; fi

# Tradução fiel tem um item para cada item; diferença só avisa, porque pode
# ser escolha de quem traduziu.
if [ "${itens[CHANGELOG.md]}" != "${itens[CHANGELOG.en.md]}" ]; then
  echo "::warning file=CHANGELOG.en.md::A entrada de ${comp} v${ver} tem ${itens[CHANGELOG.md]} item(ns) no CHANGELOG.md e ${itens[CHANGELOG.en.md]} no CHANGELOG.en.md."
fi

echo "CHANGELOG.md e CHANGELOG.en.md têm a entrada de ${comp} v${ver} (${itens[CHANGELOG.md]} e ${itens[CHANGELOG.en.md]} item(ns))."
