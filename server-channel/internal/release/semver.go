// Package release implementa o índice assinado de versões do server-channel
// usado pelo container evergreen (ver docs/architecture.md, "Decisão:
// container evergreen em `server-channel`"): tipos do índice, assinatura
// ed25519, semver mínimo, resolução da versão alvo e leitura/escrita de
// chaves. Só stdlib, porque é embutido no lançador ffcom-runtime.
package release

import (
	"fmt"
	"strconv"
	"strings"
)

// Semver é uma versão MAJOR.MINOR.PATCH sem pre-release nem build metadata.
// O fluxo de release só publica versões finais (tags channel-vX.Y.Z), então
// não há necessidade de ordenar pre-releases.
type Semver struct {
	Major, Minor, Patch int
}

// ParseSemver aceita exatamente "X.Y.Z" (sem "v" na frente), com números
// decimais sem zero à esquerda (exceto o próprio "0").
func ParseSemver(s string) (Semver, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("versão %q: esperado MAJOR.MINOR.PATCH", s)
	}
	var n [3]int
	for i, p := range parts {
		v, err := parseNumber(p)
		if err != nil {
			return Semver{}, fmt.Errorf("versão %q: %w", s, err)
		}
		n[i] = v
	}
	return Semver{Major: n[0], Minor: n[1], Patch: n[2]}, nil
}

func parseNumber(p string) (int, error) {
	if p == "" {
		return 0, fmt.Errorf("componente vazio")
	}
	for _, c := range p {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("componente %q não é numérico", p)
		}
	}
	if len(p) > 1 && p[0] == '0' {
		return 0, fmt.Errorf("componente %q com zero à esquerda", p)
	}
	v, err := strconv.Atoi(p)
	if err != nil {
		return 0, fmt.Errorf("componente %q: %w", p, err)
	}
	return v, nil
}

// String devolve "X.Y.Z".
func (v Semver) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare devolve -1, 0 ou 1 conforme v é menor, igual ou maior que o.
func (v Semver) Compare(o Semver) int {
	switch {
	case v.Major != o.Major:
		return cmpInt(v.Major, o.Major)
	case v.Minor != o.Minor:
		return cmpInt(v.Minor, o.Minor)
	default:
		return cmpInt(v.Patch, o.Patch)
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
