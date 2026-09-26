package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

// Layout de FFCOM_RUNTIME_DIR:
//
//	versions/<versão>/server-channel   binário instalado (0755)
//	versions/<versão>/sha256           sha256 gravado na instalação
//	current                            versão ativa (texto)
//	bad                                versões que falharam na troca, uma por linha
//
// Tudo é gravado em temporário + rename, para uma queda no meio não deixar
// arquivo truncado. Temporários órfãos (".tmp-*") são apagados na poda.
const (
	binName          = "server-channel"
	maxArtifactBytes = 256 << 20
)

type store struct{ dir string }

func (s store) versionsDir() string          { return filepath.Join(s.dir, "versions") }
func (s store) versionDir(v string) string   { return filepath.Join(s.versionsDir(), v) }
func (s store) binPath(v string) string      { return filepath.Join(s.versionDir(v), binName) }
func (s store) sumPath(v string) string      { return filepath.Join(s.versionDir(v), "sha256") }
func (s store) currentPath() string          { return filepath.Join(s.dir, "current") }
func (s store) badPath() string              { return filepath.Join(s.dir, "bad") }
func (s store) removeVersion(v string) error { return os.RemoveAll(s.versionDir(v)) }

// readCurrent devolve "" se ainda não há versão ativa.
func (s store) readCurrent() (string, error) {
	data, err := os.ReadFile(s.currentPath())
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(data))
	if _, err := release.ParseSemver(v); err != nil {
		return "", fmt.Errorf("%s: %w", s.currentPath(), err)
	}
	return v, nil
}

func (s store) writeCurrent(v string) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(s.currentPath(), []byte(v+"\n"), 0o644)
}

func (s store) readBad() (map[string]bool, error) {
	bad := map[string]bool{}
	data, err := os.ReadFile(s.badPath())
	if errors.Is(err, fs.ErrNotExist) {
		return bad, nil
	}
	if err != nil {
		return bad, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			bad[line] = true
		}
	}
	return bad, nil
}

func (s store) addBad(v string) error {
	bad, err := s.readBad()
	if err != nil {
		return err
	}
	bad[v] = true
	list := make([]string, 0, len(bad))
	for b := range bad {
		list = append(list, b)
	}
	sort.Strings(list)
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(s.badPath(), []byte(strings.Join(list, "\n")+"\n"), 0o644)
}

// verifyInstalled confere que o binário de v existe e que o sha256 dele
// bate com o gravado na instalação (pega volume corrompido ou binário
// trocado à mão).
func (s store) verifyInstalled(v string) error {
	want, err := os.ReadFile(s.sumPath(v))
	if err != nil {
		return err
	}
	got, err := sha256File(s.binPath(v))
	if err != nil {
		return err
	}
	if got != strings.TrimSpace(string(want)) {
		return fmt.Errorf("sha256 de %s não confere com o gravado na instalação", s.binPath(v))
	}
	return nil
}

// install grava r como binário da versão v, exigindo sha256 == wantSHA.
// Uma instalação anterior da mesma versão é substituída.
func (s store) install(v string, r io.Reader, wantSHA string) error {
	if _, err := release.ParseSemver(v); err != nil {
		return err // v vira nome de diretório: só X.Y.Z numérico
	}
	if err := os.MkdirAll(s.versionsDir(), 0o755); err != nil {
		return err
	}
	tmp, err := downloadVerified(s.versionsDir(), r, wantSHA)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if err := os.RemoveAll(s.versionDir(v)); err != nil {
		return err
	}
	if err := os.MkdirAll(s.versionDir(v), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.binPath(v)); err != nil {
		return err
	}
	return writeFileAtomic(s.sumPath(v), []byte(wantSHA+"\n"), 0o644)
}

// prune apaga de versions/ tudo que não está em keep, inclusive
// temporários órfãos. Devolve as versões apagadas.
func (s store) prune(keep ...string) ([]string, error) {
	entries, err := os.ReadDir(s.versionsDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	keepSet := map[string]bool{}
	for _, k := range keep {
		if k != "" {
			keepSet[k] = true
		}
	}
	var removed []string
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if keepSet[name] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.versionsDir(), name)); err != nil {
			errs = append(errs, err)
			continue
		}
		if !strings.HasPrefix(name, ".tmp-") {
			removed = append(removed, name)
		}
	}
	return removed, errors.Join(errs...)
}

// downloadVerified copia r (até maxArtifactBytes) para um temporário em dir,
// confere o sha256 e deixa o arquivo com 0755. Devolve o caminho do
// temporário; quem chama renomeia ou apaga. Em erro não sobra nada.
func downloadVerified(dir string, r io.Reader, wantSHA string) (string, error) {
	if err := release.ValidateSHA256(wantSHA); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, maxArtifactBytes+1))
	if err != nil {
		return "", err
	}
	if n > maxArtifactBytes {
		return "", fmt.Errorf("artefato maior que %d bytes", maxArtifactBytes)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return "", fmt.Errorf("sha256 divergente: esperado %s, baixado %s", wantSHA, got)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return "", err
	}
	ok = true
	return tmp.Name(), nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeFileAtomic grava num temporário ao lado e renomeia.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// lastUpdate é o conteúdo de <runtime>/last-update: a última troca feita
// pelo auto-update. Serve para o rollback de uma versão que passou no
// /healthz mas depois entra em crash loop (ver launcher.rollbackAfterCrashes).
type lastUpdate struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	At   time.Time `json:"at"`
}

func (s store) lastUpdatePath() string { return filepath.Join(s.dir, "last-update") }

// readLastUpdate devolve nil sem erro se não há registro.
func (s store) readLastUpdate() (*lastUpdate, error) {
	data, err := os.ReadFile(s.lastUpdatePath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lu lastUpdate
	if err := json.Unmarshal(data, &lu); err != nil {
		return nil, fmt.Errorf("%s: %w", s.lastUpdatePath(), err)
	}
	return &lu, nil
}

func (s store) writeLastUpdate(lu lastUpdate) error {
	data, err := json.Marshal(lu)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(s.lastUpdatePath(), append(data, '\n'), 0o644)
}

func (s store) clearLastUpdate() error {
	err := os.Remove(s.lastUpdatePath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
