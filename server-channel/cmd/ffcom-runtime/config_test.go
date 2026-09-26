package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(envFunc(nil), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RuntimeVersion != "1.0.0" || cfg.Track != "" || !cfg.AutoUpdate || cfg.Interval != time.Hour ||
		cfg.IndexURL != defaultIndexURL || cfg.RuntimeDir != "/data/runtime" || cfg.SeedDir != "/opt/ffcom/seed" ||
		cfg.HealthURL != "http://127.0.0.1:8080/healthz" || len(cfg.Warnings) != 0 {
		t.Errorf("padrões inesperados: %+v", cfg)
	}
	emb, err := release.ParsePublicKey(embeddedPubKey)
	if err != nil {
		t.Fatalf("release.pub embutida inválida: %v", err)
	}
	if !cfg.PubKey.Equal(emb) {
		t.Error("chave pública deveria ser a embutida")
	}
}

func TestLoadConfigEnv(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	cfg, err := loadConfig(envFunc(map[string]string{
		"FFCOM_CHANNEL_VERSION":   "0.7",
		"FFCOM_AUTO_UPDATE":       "false",
		"FFCOM_UPDATE_INTERVAL":   "10s",
		"FFCOM_RELEASE_INDEX_URL": "https://example.invalid/base/",
		"FFCOM_RELEASE_PUBKEY":    string(release.EncodeKey(pub)),
		"FFCOM_RUNTIME_DIR":       "/r",
		"FFCOM_SEED_DIR":          "/s",
		"SERVER_PORT":             "9090",
	}), "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Track != "0.7" || cfg.AutoUpdate || cfg.Interval != minInterval || cfg.IndexURL != "https://example.invalid/base" ||
		!cfg.PubKey.Equal(pub) || cfg.RuntimeDir != "/r" || cfg.SeedDir != "/s" || cfg.HealthURL != "http://127.0.0.1:9090/healthz" {
		t.Errorf("config inesperada: %+v", cfg)
	}
	w := strings.Join(cfg.Warnings, "\n")
	if !strings.Contains(w, "FFCOM_RELEASE_PUBKEY") || !strings.Contains(w, "abaixo do mínimo") {
		t.Errorf("avisos esperados ausentes: %q", w)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"trilha":    {"FFCOM_CHANNEL_VERSION": "v0.7"},
		"auto":      {"FFCOM_AUTO_UPDATE": "talvez"},
		"intervalo": {"FFCOM_UPDATE_INTERVAL": "1 hora"},
		"chave":     {"FFCOM_RELEASE_PUBKEY": "AAAA"},
		"porta":     {"SERVER_PORT": "abc"},
	} {
		if _, err := loadConfig(envFunc(env), "1.0.0"); err == nil {
			t.Errorf("%s: loadConfig deveria falhar", name)
		}
	}
}

func TestRuntimeVersionDev(t *testing.T) {
	if _, err := loadConfig(envFunc(nil), "dev"); err == nil {
		t.Error("build dev sem FFCOM_RUNTIME_DEV deveria falhar")
	}
	cfg, err := loadConfig(envFunc(map[string]string{"FFCOM_RUNTIME_DEV": "1"}), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RuntimeVersion != devRuntimeVersion || len(cfg.Warnings) == 0 {
		t.Errorf("dev: runtime %q, avisos %v", cfg.RuntimeVersion, cfg.Warnings)
	}
	if _, err := loadConfig(envFunc(nil), "v1.0.0"); err == nil {
		t.Error("versão com v deveria falhar")
	}
}

func TestRealMainVersion(t *testing.T) {
	var out, errOut strings.Builder
	if code := realMain([]string{"--version"}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != version {
		t.Errorf("--version: código %d, saída %q", code, out.String())
	}
	if code := realMain([]string{"desconhecido"}, &out, &errOut); code != 2 {
		t.Errorf("subcomando desconhecido: código %d", code)
	}
}
