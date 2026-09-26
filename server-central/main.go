package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/httpapi"
	"a3sitsolutions.com/ffcom/server-central/internal/relay"
	"a3sitsolutions.com/ffcom/server-central/internal/storage"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// version é sobrescrito em tempo de build via ldflags (-X main.version=...,
// ver Dockerfile e docs/architecture.md, "Decisão: versionamento e release
// dos binários"). "dev" fora de um build versionado (ex. go run local).
var version = "dev"

func main() {
	databaseURL := requireEnv("DATABASE_URL")
	issuerURL := requireEnv("OIDC_ISSUER_URL")
	// A porta interna do container é sempre 8080 (ver Dockerfile e
	// docker-compose.yml: SERVER_CENTRAL_PORT só controla o mapeamento de
	// porta do host, não é repassada ao container). SERVER_PORT permite
	// sobrescrever para quem rodar `go run .` direto, fora do Docker.
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	// Diretório onde os bytes de avatar de conta são persistidos (ver
	// internal/storage.AvatarStore e docs/architecture.md, "Decisão: upload
	// de avatar de conta") -- precisa ser um volume Docker para sobreviver a
	// recriação do container, mesmo padrão já usado pelo Postgres.
	avatarsDir := os.Getenv("AVATARS_DIR")
	if avatarsDir == "" {
		avatarsDir = "/data/avatars"
	}
	avatarMaxBytes := int64(envInt("AVATAR_MAX_MB", 2)) << 20

	ctx := context.Background()

	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		log.Fatalf("server-central: %v", err)
	}
	defer db.Close()

	verifier, err := auth.NewVerifier(ctx, issuerURL)
	if err != nil {
		log.Fatalf("server-central: %v", err)
	}

	avatarFiles, err := storage.NewAvatarStore(avatarsDir)
	if err != nil {
		log.Fatalf("server-central: %v", err)
	}

	rateLimitRPM := envInt("RATE_LIMIT_RPM", 120)
	rateLimitBurst := envInt("RATE_LIMIT_BURST", 60)
	requireTLS := envBool("REQUIRE_TLS", false)
	ideasCfg := ideasConfigFromEnv()
	router := httpapi.NewRouter(verifier, db, avatarFiles, avatarMaxBytes, parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")), version, rateLimitRPM, rateLimitBurst, requireTLS, ideasCfg)

	// Varinha e checagem das sugestões da home via a3s-claude-relay: o relay
	// devolve o resultado por webhook num listener interno próprio, numa
	// porta que o proxy público não encaminha (ver docs/architecture.md,
	// "Decisão: sugestões de melhoria com varinha do Claude").
	if ideasCfg.Relay != nil {
		callbackPort := os.Getenv("CLAUDE_CALLBACK_PORT")
		if callbackPort == "" {
			callbackPort = "8090"
		}
		go func() {
			log.Printf("server-central: callback do relay ouvindo em :%s", callbackPort)
			if err := http.ListenAndServe(":"+callbackPort, httpapi.NewCallbackHandler(db, ideasCfg)); err != nil {
				log.Fatalf("server-central: callback do relay: %v", err)
			}
		}()
		go httpapi.RunIdeasSweeper(ctx, db, ideasCfg, 30*time.Second)
	} else {
		log.Printf("server-central: CLAUDE_RELAY_URL vazio; varinha desligada e sugestões vão direto para moderação")
	}

	log.Printf("server-central: versão %s, ouvindo em :%s (OIDC issuer: %s)", version, port, issuerURL)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server-central: %v", err)
	}
}

// ideasConfigFromEnv lê a configuração das sugestões da home. Sem
// CLAUDE_RELAY_URL a varinha fica desligada; com ela, a chave, o segredo do
// callback e a URL pela qual o relay alcança este container são
// obrigatórios.
func ideasConfigFromEnv() httpapi.IdeasConfig {
	cfg := httpapi.IdeasConfig{
		WandPerDay:       envInt("IDEAS_WAND_PER_DAY", 3),
		OffensivePenalty: envInt("IDEAS_OFFENSIVE_PENALTY", 2),
		AdminGroup:       os.Getenv("IDEAS_ADMIN_GROUP"),
		ImproveTimeout:   time.Duration(envInt("IDEAS_ASSIST_TIMEOUT_SECONDS", 360)) * time.Second,
		CheckTimeout:     time.Duration(envInt("IDEAS_CHECK_TIMEOUT_MINUTES", 30)) * time.Minute,
	}
	if cfg.AdminGroup == "" {
		cfg.AdminGroup = "ffcom-admins"
	}
	if relayURL := os.Getenv("CLAUDE_RELAY_URL"); relayURL != "" {
		cfg.Relay = relay.New(relayURL, requireEnv("CLAUDE_RELAY_API_KEY"))
		cfg.CallbackSecret = requireEnv("CLAUDE_CALLBACK_SECRET")
		cfg.CallbackBaseURL = requireEnv("CLAUDE_CALLBACK_BASE_URL")
	}
	return cfg
}

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("server-central: variável de ambiente %s não definida", name)
	}
	return value
}

// envInt lê uma variável de ambiente inteira opcional, com fallback se
// ausente ou inválida (RATE_LIMIT_RPM / RATE_LIMIT_BURST — ver
// internal/httpapi/ratelimit.go).
func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("server-central: %s inválido (%q), usando padrão %d", name, raw, fallback)
		return fallback
	}
	return value
}

// envBool lê uma variável de ambiente booleana opcional ("true"/"1" ligam),
// com fallback se ausente (REQUIRE_TLS — ver
// internal/httpapi/requiretls.go).
func envBool(name string, fallback bool) bool {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	return raw == "true" || raw == "1"
}

// parseAllowedOrigins lê CORS_ALLOWED_ORIGINS (lista separada por vírgula,
// ex.: "http://localhost:5173,https://app.minhacomunidade.com"). Vazio
// significa nenhuma origem cruzada liberada — mesmo padrão de
// server-channel (ver docs/architecture.md, "Decisão: CORS em
// server-channel").
func parseAllowedOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}
