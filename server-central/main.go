package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/httpapi"
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

	rateLimitRPM := envInt("RATE_LIMIT_RPM", 120)
	rateLimitBurst := envInt("RATE_LIMIT_BURST", 20)
	router := httpapi.NewRouter(verifier, db, parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")), version, rateLimitRPM, rateLimitBurst)

	log.Printf("server-central: versão %s, ouvindo em :%s (OIDC issuer: %s)", version, port, issuerURL)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server-central: %v", err)
	}
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
