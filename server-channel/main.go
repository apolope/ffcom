package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/httpapi"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

func main() {
	databaseURL := requireEnv("DATABASE_URL")
	issuerURL := requireEnv("OIDC_ISSUER_URL")
	// A porta interna do container é sempre 8080 (ver Dockerfile e
	// docker-compose.yml: SERVER_CHANNEL_PORT só controla o mapeamento de
	// porta do host, não é repassada ao container). SERVER_PORT permite
	// sobrescrever para quem rodar `go run .` direto, fora do Docker.
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	ctx := context.Background()

	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		log.Fatalf("server-channel: %v", err)
	}
	defer db.Close()

	verifier, err := auth.NewVerifier(ctx, issuerURL)
	if err != nil {
		log.Fatalf("server-channel: %v", err)
	}

	router := httpapi.NewRouter(verifier, db)

	log.Printf("server-channel: ouvindo em :%s (OIDC issuer: %s)", port, issuerURL)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server-channel: %v", err)
	}
}

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("server-channel: variável de ambiente %s não definida", name)
	}
	return value
}
