package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

// Fluxo completo do workflow: keygen, index add (criando e estendendo),
// sign pela variável de ambiente e verify.
func TestFluxoCompleto(t *testing.T) {
	dir := t.TempDir()
	p := func(n string) string { return filepath.Join(dir, n) }
	must := func(args ...string) {
		t.Helper()
		if err := run(args, io.Discard, io.Discard); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}

	must("keygen", "-pub", p("release.pub"), "-priv", p("release.key"))
	if err := run([]string{"keygen", "-pub", p("x.pub"), "-priv", p("release.key")}, io.Discard, io.Discard); err == nil {
		t.Fatal("keygen deveria recusar sobrescrever a privada")
	}

	os.WriteFile(p("amd64"), []byte("binário amd64"), 0o644)
	os.WriteFile(p("arm64"), []byte("binário arm64"), 0o644)
	must("index", "add", "-index", p("index.json"), "-version", "0.7.0", "-min-runtime", "1.0.0",
		"-artifact", "linux/amd64=https://example.invalid/a?x=1="+p("amd64"),
		"-artifact", "linux/arm64=https://example.invalid/b="+p("arm64"))
	must("index", "add", "-index", p("index.json"), "-version", "0.8.0", "-min-runtime", "1.0.0",
		"-artifact", "linux/amd64=https://example.invalid/c="+p("amd64"))

	key, _ := os.ReadFile(p("release.key"))
	t.Setenv("TEST_SIGNING_KEY", strings.TrimSpace(string(key)))
	must("sign", "-index", p("index.json"), "-key-env", "TEST_SIGNING_KEY", "-out", p("index.json.sig"))
	must("verify", "-index", p("index.json"), "-sig", p("index.json.sig"), "-pub", p("release.pub"))

	data, _ := os.ReadFile(p("index.json"))
	idx, err := release.ParseIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Versions) != 2 || idx.Versions[0].Version != "0.8.0" {
		t.Fatalf("índice inesperado: %+v", idx.Versions)
	}
	if got := idx.Versions[1].Artifacts["linux/amd64"].URL; got != "https://example.invalid/a?x=1" {
		t.Errorf("url com '=' cortada: %q", got)
	}

	// Adulterar o índice invalida a verificação.
	os.WriteFile(p("index.json"), append(data, ' '), 0o644)
	if err := run([]string{"verify", "-index", p("index.json"), "-sig", p("index.json.sig"), "-pub", p("release.pub")}, io.Discard, io.Discard); err == nil {
		t.Error("verify deveria falhar com índice adulterado")
	}

	t.Setenv("TEST_SIGNING_KEY", "")
	if err := run([]string{"sign", "-index", p("index.json"), "-key-env", "TEST_SIGNING_KEY"}, io.Discard, io.Discard); err == nil {
		t.Error("sign deveria falhar sem a variável de ambiente")
	}
}
