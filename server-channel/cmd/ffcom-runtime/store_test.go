package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestStoreInstallAndVerify(t *testing.T) {
	st := store{dir: t.TempDir()}
	bin := []byte("binário 0.7.0")
	if err := st.install("0.7.0", bytes.NewReader(bin), shaOf(bin)); err != nil {
		t.Fatal(err)
	}
	if err := st.verifyInstalled("0.7.0"); err != nil {
		t.Fatalf("instalação recém-feita não confere: %v", err)
	}
	// Binário trocado no disco é detectado.
	if err := os.WriteFile(st.binPath("0.7.0"), []byte("outro"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := st.verifyInstalled("0.7.0"); err == nil {
		t.Error("binário adulterado deveria falhar em verifyInstalled")
	}
	if err := st.verifyInstalled("0.9.9"); err == nil {
		t.Error("versão ausente deveria falhar")
	}
}

func TestStoreInstallRejects(t *testing.T) {
	st := store{dir: t.TempDir()}
	bin := []byte("binário")
	if err := st.install("0.7.1", bytes.NewReader(bin), shaOf([]byte("outro"))); err == nil || !strings.Contains(err.Error(), "sha256 divergente") {
		t.Errorf("sha divergente: err = %v", err)
	}
	if _, err := os.Stat(st.versionDir("0.7.1")); !os.IsNotExist(err) {
		t.Error("sha divergente não deveria criar o diretório da versão")
	}
	if entries, _ := os.ReadDir(st.versionsDir()); len(entries) != 0 {
		t.Errorf("sobrou lixo em versions/: %v", entries)
	}
	if err := st.install("../x", bytes.NewReader(bin), shaOf(bin)); err == nil {
		t.Error("versão fora de X.Y.Z deveria ser recusada")
	}
}

func TestStoreCurrentAndBad(t *testing.T) {
	st := store{dir: filepath.Join(t.TempDir(), "rt")}
	if v, err := st.readCurrent(); v != "" || err != nil {
		t.Errorf("current inexistente: %q, %v", v, err)
	}
	if err := st.writeCurrent("0.7.2"); err != nil {
		t.Fatal(err)
	}
	if v, err := st.readCurrent(); v != "0.7.2" || err != nil {
		t.Errorf("current: %q, %v", v, err)
	}
	for _, v := range []string{"0.7.3", "0.7.1", "0.7.3"} {
		if err := st.addBad(v); err != nil {
			t.Fatal(err)
		}
	}
	bad, err := st.readBad()
	if err != nil || len(bad) != 2 || !bad["0.7.1"] || !bad["0.7.3"] {
		t.Errorf("bad = %v, %v", bad, err)
	}
	if data, _ := os.ReadFile(st.badPath()); string(data) != "0.7.1\n0.7.3\n" {
		t.Errorf("arquivo bad = %q", data)
	}
}

func TestStorePrune(t *testing.T) {
	st := store{dir: t.TempDir()}
	for _, v := range []string{"0.6.0", "0.7.0", "0.7.1", "0.7.2"} {
		b := []byte(v)
		if err := st.install(v, bytes.NewReader(b), shaOf(b)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(st.versionsDir(), ".tmp-123"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := st.prune("0.7.2", "0.7.1", "")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(removed)
	if strings.Join(removed, ",") != "0.6.0,0.7.0" {
		t.Errorf("removidas = %v", removed)
	}
	entries, _ := os.ReadDir(st.versionsDir())
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if strings.Join(left, ",") != "0.7.1,0.7.2" {
		t.Errorf("restaram %v", left)
	}
}
