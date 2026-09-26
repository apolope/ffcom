package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

// readSeed lê a versão do binário semente da imagem (<dir>/VERSION, gravado
// por install-seed). Devolve "" sem erro se a imagem não traz semente.
func readSeed(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if _, err := release.ParseSemver(v); err != nil {
		return "", fmt.Errorf("%s/VERSION: %w", dir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, binName)); err != nil {
		return "", err
	}
	return v, nil
}

// defaultTrack é a trilha usada sem FFCOM_CHANNEL_VERSION: o minor (X.Y) da
// semente, ou da versão ativa se a imagem não traz semente; sem nenhuma das
// duas, latest.
func defaultTrack(seed, current string) string {
	for _, v := range []string{seed, current} {
		if sv, err := release.ParseSemver(v); err == nil {
			return fmt.Sprintf("%d.%d", sv.Major, sv.Minor)
		}
	}
	return "latest"
}

// chooseBoot decide qual versão subir na partida.
//
// Com current íntegra (binário presente e sha256 conferindo), fica nela, a
// menos que a imagem traga uma semente mais nova que current, dentro da
// trilha e fora de bad (caso típico: o operador atualizou a imagem de
// propósito). Sem current íntegra, usa a semente. Sem as duas, devolve ""
// e o launcher baixa pelo índice.
func chooseBoot(current string, currentOK bool, seed, track string, bad map[string]bool) (v string, fromSeed bool) {
	if !currentOK {
		if seed != "" {
			return seed, true
		}
		return "", false
	}
	if seed == "" || seed == current || bad[seed] {
		return current, false
	}
	sv, err1 := release.ParseSemver(seed)
	cv, err2 := release.ParseSemver(current)
	if err1 != nil || err2 != nil || sv.Compare(cv) <= 0 {
		return current, false
	}
	if ok, err := release.MatchesTrack(track, sv); err != nil || !ok {
		return current, false
	}
	return seed, true
}

// shouldSwitch diz se a versão escolhida no índice deve substituir a atual:
// só para frente, exceto com trilha fixa X.Y.Z, em que o operador pediu
// aquela versão exata e o downgrade é permitido.
func shouldSwitch(current, chosen, track string) bool {
	if current == "" {
		return true
	}
	if chosen == current {
		return false
	}
	cv, err1 := release.ParseSemver(current)
	nv, err2 := release.ParseSemver(chosen)
	if err1 != nil || err2 != nil {
		return false
	}
	if strings.Count(track, ".") == 2 {
		return true
	}
	return nv.Compare(cv) > 0
}
