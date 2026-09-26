package main

import (
	"crypto/ed25519"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

const (
	defaultIndexURL   = "https://github.com/apolope/ffcom/releases/download/channel-stable"
	defaultRuntimeDir = "/data/runtime"
	defaultSeedDir    = "/opt/ffcom/seed"
	defaultInterval   = time.Hour
	minInterval       = time.Minute
)

// config é o resultado das variáveis de ambiente, já validado.
type config struct {
	RuntimeVersion string // versão do contrato (X.Y.Z) comparada com min_runtime
	Platform       string // os/arch deste processo, chave de Artifacts no índice
	Track          string // FFCOM_CHANNEL_VERSION; vazio = derivar da semente no boot
	AutoUpdate     bool
	Interval       time.Duration
	IndexURL       string // base, sem barra no fim
	PubKey         ed25519.PublicKey
	RuntimeDir     string
	SeedDir        string
	HealthURL      string
	Warnings       []string // logados pelo launcher na partida
}

func loadConfig(getenv func(string) string, buildVersion string) (config, error) {
	cfg := config{
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
		AutoUpdate: true,
		Interval:   defaultInterval,
		IndexURL:   defaultIndexURL,
		RuntimeDir: defaultRuntimeDir,
		SeedDir:    defaultSeedDir,
	}

	rt, dev, err := runtimeVersion(buildVersion, getenv)
	if err != nil {
		return cfg, err
	}
	cfg.RuntimeVersion = rt
	if dev {
		cfg.Warnings = append(cfg.Warnings, "AVISO: build dev com FFCOM_RUNTIME_DEV=1, runtime tratado como "+rt+"; não use em produção")
	}

	if t := strings.TrimSpace(getenv("FFCOM_CHANNEL_VERSION")); t != "" {
		if _, err := release.MatchesTrack(t, release.Semver{}); err != nil {
			return cfg, fmt.Errorf("FFCOM_CHANNEL_VERSION: %w", err)
		}
		cfg.Track = t
	}

	if s := strings.TrimSpace(getenv("FFCOM_AUTO_UPDATE")); s != "" {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return cfg, fmt.Errorf("FFCOM_AUTO_UPDATE=%q: esperado true ou false", s)
		}
		cfg.AutoUpdate = b
	}

	if s := strings.TrimSpace(getenv("FFCOM_UPDATE_INTERVAL")); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return cfg, fmt.Errorf("FFCOM_UPDATE_INTERVAL=%q: %w", s, err)
		}
		if d < minInterval {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("FFCOM_UPDATE_INTERVAL=%s abaixo do mínimo, usando %s", d, minInterval))
			d = minInterval
		}
		cfg.Interval = d
	}

	if s := strings.TrimSpace(getenv("FFCOM_RELEASE_INDEX_URL")); s != "" {
		cfg.IndexURL = s
	}
	cfg.IndexURL = strings.TrimRight(cfg.IndexURL, "/")

	keyData, keySource := embeddedPubKey, "embutida"
	if s := strings.TrimSpace(getenv("FFCOM_RELEASE_PUBKEY")); s != "" {
		keyData, keySource = []byte(s), "FFCOM_RELEASE_PUBKEY"
		cfg.Warnings = append(cfg.Warnings, "AVISO: chave pública do índice sobrescrita por FFCOM_RELEASE_PUBKEY; só versões assinadas por essa chave serão aceitas")
	}
	cfg.PubKey, err = release.ParsePublicKey(keyData)
	if err != nil {
		return cfg, fmt.Errorf("chave pública %s: %w", keySource, err)
	}

	if s := getenv("FFCOM_RUNTIME_DIR"); s != "" {
		cfg.RuntimeDir = s
	}
	if s := getenv("FFCOM_SEED_DIR"); s != "" {
		cfg.SeedDir = s
	}

	// Mesma variável que o server-channel lê (main.go do serviço); o
	// healthcheck é sempre local.
	port := strings.TrimSpace(getenv("SERVER_PORT"))
	if port == "" {
		port = "8080"
	}
	if n, err := strconv.Atoi(port); err != nil || n <= 0 || n > 65535 {
		return cfg, fmt.Errorf("SERVER_PORT=%q: porta inválida", port)
	}
	cfg.HealthURL = "http://127.0.0.1:" + port + "/healthz"
	return cfg, nil
}
