package release

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Component é o único valor aceito em Index.Component.
const Component = "server-channel"

// Index é o conteúdo de index.json no release fixo channel-stable. Formato
// descrito em docs/architecture.md, "Decisão: container evergreen em
// `server-channel`", item 3.
type Index struct {
	Component   string    `json:"component"`
	GeneratedAt time.Time `json:"generated_at"`
	Versions    []Version `json:"versions"`
}

// Version é uma versão publicada do serviço.
type Version struct {
	Version     string              `json:"version"`
	MinRuntime  string              `json:"min_runtime"`
	PublishedAt time.Time           `json:"published_at"`
	Artifacts   map[string]Artifact `json:"artifacts"` // chave: plataforma, ex. "linux/amd64"
}

// Artifact é um binário para uma plataforma. SHA256 em hex minúsculo.
type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// ErrNoVersion indica que nenhuma versão do índice atende à trilha e à
// plataforma pedidas (ou todas exigem um runtime mais novo).
var ErrNoVersion = errors.New("release: nenhuma versão compatível no índice")

// ParseIndex decodifica e valida index.json.
//
// Só deve ser chamado DEPOIS de Verify ter aceitado a assinatura sobre os
// mesmos bytes: a validação aqui é de formato, não de origem, e um índice
// não verificado pode apontar para qualquer binário.
//
// Valida: component == "server-channel"; version e min_runtime em semver
// X.Y.Z; versões sem duplicata; cada artefato com url não vazia e sha256 hex
// de 64 caracteres. Campos desconhecidos são ignorados, para que índices
// futuros com campos a mais continuem legíveis por lançadores antigos.
func ParseIndex(data []byte) (*Index, error) {
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("release: índice: %w", err)
	}
	if err := idx.Validate(); err != nil {
		return nil, err
	}
	return &idx, nil
}

// Validate aplica as mesmas regras de ParseIndex a um índice em memória.
func (idx *Index) Validate() error {
	if idx.Component != Component {
		return fmt.Errorf("release: índice de %q, esperado %q", idx.Component, Component)
	}
	seen := make(map[Semver]bool, len(idx.Versions))
	for i, v := range idx.Versions {
		sv, err := ParseSemver(v.Version)
		if err != nil {
			return fmt.Errorf("release: versions[%d]: %w", i, err)
		}
		if seen[sv] {
			return fmt.Errorf("release: versão %s duplicada no índice", v.Version)
		}
		seen[sv] = true
		if _, err := ParseSemver(v.MinRuntime); err != nil {
			return fmt.Errorf("release: versão %s: min_runtime: %w", v.Version, err)
		}
		for platform, a := range v.Artifacts {
			if err := validatePlatform(platform); err != nil {
				return fmt.Errorf("release: versão %s: %w", v.Version, err)
			}
			if a.URL == "" {
				return fmt.Errorf("release: versão %s, %s: url vazia", v.Version, platform)
			}
			if err := ValidateSHA256(a.SHA256); err != nil {
				return fmt.Errorf("release: versão %s, %s: %w", v.Version, platform, err)
			}
		}
	}
	return nil
}

func validatePlatform(p string) error {
	os, arch, ok := strings.Cut(p, "/")
	if !ok || os == "" || arch == "" || strings.Contains(arch, "/") {
		return fmt.Errorf("plataforma %q: esperado os/arch", p)
	}
	return nil
}

// ValidateSHA256 exige 64 caracteres hex minúsculos.
func ValidateSHA256(s string) error {
	if len(s) != 64 {
		return fmt.Errorf("sha256 com %d caracteres, esperado 64", len(s))
	}
	if strings.ToLower(s) != s {
		return fmt.Errorf("sha256 %q deve estar em minúsculas", s)
	}
	if _, err := hex.DecodeString(s); err != nil {
		return fmt.Errorf("sha256 %q não é hex", s)
	}
	return nil
}

// Marshal serializa o índice no formato publicado (JSON indentado com dois
// espaços e "\n" no fim). A assinatura é feita sobre esses bytes depois de
// gravados, nunca sobre uma re-serialização.
func (idx *Index) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(idx); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Upsert insere v no índice, substituindo uma entrada de mesma versão, e
// mantém Versions em ordem decrescente de semver. Não mexe em GeneratedAt.
func (idx *Index) Upsert(v Version) error {
	sv, err := ParseSemver(v.Version)
	if err != nil {
		return err
	}
	out := idx.Versions[:0:0]
	for _, old := range idx.Versions {
		if o, err := ParseSemver(old.Version); err == nil && o == sv {
			continue
		}
		out = append(out, old)
	}
	out = append(out, v)
	sortDesc(out)
	idx.Versions = out
	return nil
}

func sortDesc(vs []Version) {
	sort.SliceStable(vs, func(i, j int) bool {
		a, errA := ParseSemver(vs[i].Version)
		b, errB := ParseSemver(vs[j].Version)
		if errA != nil || errB != nil {
			return errB != nil && errA == nil
		}
		return a.Compare(b) > 0
	})
}

// Resolve escolhe a versão alvo do índice.
//
// track: "latest" (maior versão), "X.Y" (maior patch desse minor) ou "X.Y.Z"
// (exatamente essa). Versões sem artefato para platform são ignoradas.
// Versões com min_runtime acima de runtimeVersion também são ignoradas, e a
// mais nova entre elas (que atenda à trilha e à plataforma e seja mais nova
// que a escolhida) volta em skippedForRuntime, para o lançador logar que
// precisa de imagem nova. Se nada atender, devolve ErrNoVersion (com
// skippedForRuntime preenchido quando o motivo foi o runtime).
func Resolve(idx *Index, track, runtimeVersion, platform string) (chosen Version, skippedForRuntime *Version, err error) {
	rt, err := ParseSemver(runtimeVersion)
	if err != nil {
		return Version{}, nil, fmt.Errorf("release: versão do runtime: %w", err)
	}
	match, err := trackMatcher(track)
	if err != nil {
		return Version{}, nil, err
	}

	var (
		best, bestSkipped     *Version
		bestSV, bestSkippedSV Semver
	)
	for i := range idx.Versions {
		v := &idx.Versions[i]
		sv, err := ParseSemver(v.Version)
		if err != nil || !match(sv) {
			continue
		}
		if _, ok := v.Artifacts[platform]; !ok {
			continue
		}
		minRT, err := ParseSemver(v.MinRuntime)
		if err != nil {
			continue
		}
		if minRT.Compare(rt) > 0 {
			if bestSkipped == nil || sv.Compare(bestSkippedSV) > 0 {
				bestSkipped, bestSkippedSV = v, sv
			}
			continue
		}
		if best == nil || sv.Compare(bestSV) > 0 {
			best, bestSV = v, sv
		}
	}

	if bestSkipped != nil && (best == nil || bestSkippedSV.Compare(bestSV) > 0) {
		s := *bestSkipped
		skippedForRuntime = &s
	}
	if best == nil {
		return Version{}, skippedForRuntime, ErrNoVersion
	}
	return *best, skippedForRuntime, nil
}

func trackMatcher(track string) (func(Semver) bool, error) {
	if track == "latest" {
		return func(Semver) bool { return true }, nil
	}
	switch strings.Count(track, ".") {
	case 1:
		sv, err := ParseSemver(track + ".0")
		if err != nil {
			return nil, fmt.Errorf("release: trilha %q: esperado latest, X.Y ou X.Y.Z", track)
		}
		return func(v Semver) bool { return v.Major == sv.Major && v.Minor == sv.Minor }, nil
	case 2:
		sv, err := ParseSemver(track)
		if err != nil {
			return nil, fmt.Errorf("release: trilha %q: esperado latest, X.Y ou X.Y.Z", track)
		}
		return func(v Semver) bool { return v == sv }, nil
	default:
		return nil, fmt.Errorf("release: trilha %q: esperado latest, X.Y ou X.Y.Z", track)
	}
}
