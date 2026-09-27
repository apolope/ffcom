package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/httpapi"
	"a3sitsolutions.com/ffcom/server-channel/internal/livekit"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/storage"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// version é sobrescrito em tempo de build via ldflags (-X main.version=...,
// ver Dockerfile e docs/architecture.md, "Decisão: versionamento e release
// dos binários"). "dev" fora de um build versionado (ex. go run local).
var version = "dev"

// thirdPartyNotices são as licenças dos módulos de terceiros que entram
// neste binário, impressas por --licenses. Gerado por
// scripts/go-third-party-notices.sh; o CI recusa o arquivo desatualizado.
//
//go:embed THIRD_PARTY_NOTICES.txt
var thirdPartyNotices string

// shutdownTimeout é quanto o graceful shutdown espera as requisições REST
// em andamento terminarem. Fica abaixo dos 30s que o lançador ffcom-runtime
// espera depois do SIGTERM antes de matar o processo (ver
// docs/architecture.md, "Decisão: container evergreen em server-channel").
const shutdownTimeout = 25 * time.Second

func main() {
	// --version imprime só a versão e sai, sem exigir nenhuma variável de
	// ambiente: o lançador usa para conferir o binário baixado antes de
	// trocar de versão.
	showVersion := flag.Bool("version", false, "imprime a versão e sai")
	showLicenses := flag.Bool("licenses", false, "imprime as licenças de terceiros e sai")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *showLicenses {
		fmt.Print(thirdPartyNotices)
		return
	}

	databaseURL := requireEnv("DATABASE_URL")
	issuerURL := requireEnv("OIDC_ISSUER_URL")
	liveKitAPIKey := requireEnv("LIVEKIT_API_KEY")
	liveKitAPISecret := requireEnv("LIVEKIT_API_SECRET")
	liveKitPublicURL := requireEnv("LIVEKIT_PUBLIC_URL")
	// Endereço da API do LiveKit visto de dentro (ex. http://livekit:7880),
	// usado para listar quem está em cada sala de voz. Opcional: sem ele,
	// deriva de LIVEKIT_PUBLIC_URL e dá a volta pelo proxy reverso.
	liveKitAPIURL := os.Getenv("LIVEKIT_API_URL")
	if liveKitAPIURL == "" {
		liveKitAPIURL = livekit.APIURLFromPublicURL(liveKitPublicURL)
	}
	// A porta interna do container é sempre 8080 (ver Dockerfile e
	// docker-compose.yml: SERVER_CHANNEL_PORT só controla o mapeamento de
	// porta do host, não é repassada ao container). SERVER_PORT permite
	// sobrescrever para quem rodar `go run .` direto, fora do Docker.
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	// Diretório onde os bytes de anexo de mensagem são persistidos (ver
	// internal/storage.FileStore e docs/architecture.md, "Decisão: upload de
	// anexo em mensagem") -- precisa ser um volume Docker para sobreviver a
	// recriação do container, mesmo padrão já usado pelo Postgres.
	attachmentsDir := os.Getenv("ATTACHMENTS_DIR")
	if attachmentsDir == "" {
		attachmentsDir = "/data/attachments"
	}
	attachmentMaxBytes := int64(envInt("ATTACHMENT_MAX_MB", 8)) << 20

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

	attachmentFiles, err := storage.NewFileStore(attachmentsDir)
	if err != nil {
		log.Fatalf("server-channel: %v", err)
	}

	hub := realtime.NewHub()
	router := httpapi.NewRouter(
		hub,
		verifier,
		db,
		attachmentFiles,
		attachmentMaxBytes,
		parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")),
		liveKitAPIKey,
		liveKitAPISecret,
		liveKitPublicURL,
		liveKitAPIURL,
		version,
		envBool("REQUIRE_TLS", false),
		envInt("RATE_LIMIT_RPM", 120),
		envInt("RATE_LIMIT_BURST", 60),
		envInt("RATE_LIMIT_WS_RPM", 60),
		envInt("RATE_LIMIT_WS_BURST", 10),
	)

	srv := &http.Server{Addr: ":" + port, Handler: router}
	// Shutdown não acompanha conexões sequestradas pelo upgrade do
	// WebSocket; hub.Close manda close frame para cada uma.
	srv.RegisterOnShutdown(hub.Close)

	// Graceful shutdown em SIGTERM (lançador trocando de versão, docker
	// stop) e SIGINT (Ctrl+C no go run local).
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("server-channel: versão %s, ouvindo em :%s (OIDC issuer: %s)", version, port, issuerURL)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		log.Fatalf("server-channel: %v", err)
	case <-sigCtx.Done():
	}
	stop()

	log.Printf("server-channel: sinal recebido, encerrando (até %s)", shutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server-channel: shutdown incompleto: %v", err)
	}
	// db.Close (defer acima) fecha o pool do pgx depois do return.
	log.Printf("server-channel: encerrado")
}

func requireEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("server-channel: variável de ambiente %s não definida", name)
	}
	return value
}

// envInt lê uma variável de ambiente inteira opcional, com fallback se
// ausente ou inválida (RATE_LIMIT_RPM/RATE_LIMIT_BURST e
// RATE_LIMIT_WS_RPM/RATE_LIMIT_WS_BURST — ver
// internal/httpapi/ratelimit.go).
func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("server-channel: %s inválido (%q), usando padrão %d", name, raw, fallback)
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
// ex.: "http://localhost:5173,https://chat.minhacomunidade.com"). Vazio
// significa nenhuma origem cruzada liberada — mantém o comportamento restrito
// anterior por padrão (ver docs/architecture.md, "Decisão: CORS em
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
