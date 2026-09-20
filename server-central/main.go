package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/httpapi"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

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

	router := httpapi.NewRouter(verifier, db, parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")))

	log.Printf("server-central: ouvindo em :%s (OIDC issuer: %s)", port, issuerURL)
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
