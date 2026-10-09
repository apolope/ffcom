package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/authentik"
	"a3sitsolutions.com/ffcom/server-central/internal/httpapi"
	"a3sitsolutions.com/ffcom/server-central/internal/push"
	"a3sitsolutions.com/ffcom/server-central/internal/relay"
	"a3sitsolutions.com/ffcom/server-central/internal/storage"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
	"a3sitsolutions.com/ffcom/server-central/internal/telegram"
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

func main() {
	showLicenses := flag.Bool("licenses", false, "imprime as licenças de terceiros e sai")
	flag.Parse()
	if *showLicenses {
		fmt.Print(thirdPartyNotices)
		return
	}

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

	verifier, err := auth.NewVerifier(ctx, auth.ParseIssuerURLs(issuerURL))
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
	signupCfg := signupConfigFromEnv()
	pushDispatcher := pushDispatcherFromEnv(db)
	// Links do avatar de quem mandou, nas notificações push (ver
	// internal/httpapi/push_avatar.go). A limpeza dos vencidos roda mesmo
	// com o push desligado, para não sobrar linha de quando ele estava
	// ligado.
	pushAvatarLinks := httpapi.NewPushAvatarLinks(db.Push, publicURLFromEnv())
	pushDispatcher.SetAuthorAvatars(pushAvatarLinks)
	go pushAvatarLinks.RunPurge(ctx, time.Hour)
	router := httpapi.NewRouter(verifier, db, avatarFiles, avatarMaxBytes, parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")), version, rateLimitRPM, rateLimitBurst, requireTLS, ideasCfg, signupCfg, pushDispatcher)

	// Listener interno, numa porta que o proxy público não encaminha: o
	// a3s-claude-relay devolve por ali o resultado da varinha e da checagem
	// das sugestões (ver docs/architecture.md, "Decisão: sugestões de
	// melhoria com varinha do Claude"), e o a3s-network-monitor repassa os
	// cliques de aprovar/reprovar cadastro (ver "Decisão: cadastro com
	// aprovação pelo Telegram").
	if ideasCfg.Relay != nil || signupCfg.Telegram != nil {
		internalPort := os.Getenv("CLAUDE_CALLBACK_PORT")
		if internalPort == "" {
			internalPort = "8090"
		}
		go func() {
			log.Printf("server-central: listener interno ouvindo em :%s", internalPort)
			if err := http.ListenAndServe(":"+internalPort, httpapi.NewInternalHandler(db, ideasCfg, signupCfg)); err != nil {
				log.Fatalf("server-central: listener interno: %v", err)
			}
		}()
	}
	if ideasCfg.Relay != nil {
		go httpapi.RunIdeasSweeper(ctx, db, ideasCfg, 30*time.Second)
	} else {
		log.Printf("server-central: CLAUDE_RELAY_URL vazio; varinha desligada e sugestões vão direto para moderação")
	}
	if signupCfg.Telegram != nil {
		go httpapi.RunSignupNotifier(ctx, db, signupCfg, time.Minute)
	} else {
		log.Printf("server-central: TELEGRAM_BOT_TOKEN vazio; cadastro pela home desligado")
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

// signupConfigFromEnv lê a configuração dos pedidos de cadastro da home.
// Sem TELEGRAM_BOT_TOKEN o cadastro fica desligado (a rota responde 503);
// com ele, o chat, o token do Authentik, o stage do e-mail de senha e o
// segredo das decisões são obrigatórios.
func signupConfigFromEnv() httpapi.SignupConfig {
	cfg := httpapi.SignupConfig{
		UsersGroup:            os.Getenv("SIGNUP_USERS_GROUP"),
		RecoveryTokenDuration: os.Getenv("SIGNUP_RECOVERY_TOKEN_DURATION"),
		MaxPerIPPerDay:        envInt("SIGNUP_MAX_PER_IP_PER_DAY", 3),
		MaxPerHour:            envInt("SIGNUP_MAX_PER_HOUR", 20),
	}
	if cfg.UsersGroup == "" {
		cfg.UsersGroup = "ffcom-users"
	}
	if cfg.RecoveryTokenDuration == "" {
		cfg.RecoveryTokenDuration = "days=3"
	}
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return cfg
	}
	threadID, err := strconv.ParseInt(os.Getenv("TELEGRAM_THREAD_ID"), 10, 64)
	if err != nil && os.Getenv("TELEGRAM_THREAD_ID") != "" {
		log.Fatalf("server-central: TELEGRAM_THREAD_ID inválido")
	}
	cfg.Telegram = telegram.New(token, requireEnv("TELEGRAM_CHAT_ID"), threadID)
	cfg.Authentik = authentik.New(requireEnv("AUTHENTIK_URL"), requireEnv("AUTHENTIK_API_TOKEN"))
	cfg.RecoveryEmailStage = requireEnv("AUTHENTIK_RECOVERY_EMAIL_STAGE")
	cfg.DecisionSecret = requireEnv("SIGNUP_DECISION_SECRET")
	return cfg
}

// pushDispatcherFromEnv liga as notificações push do app Android quando
// FCM_SERVICE_ACCOUNT_JSON tem a chave da conta de serviço do Firebase (o
// JSON numa linha ou o caminho do arquivo). Sem ela, devolve nil: as rotas
// de push continuam respondendo, mas nada é enviado. Chave inválida impede
// o boot, para o erro não passar despercebido. Ver docs/architecture.md,
// "Decisão: notificações push (fase 6)".
func pushDispatcherFromEnv(db *store.Store) *push.Dispatcher {
	raw := os.Getenv("FCM_SERVICE_ACCOUNT_JSON")
	if raw == "" {
		log.Printf("server-central: FCM_SERVICE_ACCOUNT_JSON vazio; notificações push desligadas")
		return nil
	}
	sender, err := push.NewFCMSender(raw)
	if err != nil {
		log.Fatalf("server-central: %v", err)
	}
	return push.NewDispatcher(sender, db.Push, 4, 2000)
}

// defaultPublicURL é o endereço público da instância oficial, o mesmo que
// o server-channel usa quando FFCOM_CENTRAL_URL está vazia.
const defaultPublicURL = "https://central.ffcom.a3sitsolutions.com.br"

// publicURLFromEnv lê CENTRAL_PUBLIC_URL, o endereço pelo qual o app
// alcança este server-central. Só monta os links absolutos que saem nas
// notificações push (authorAvatar); vazia, vale a instância oficial.
func publicURLFromEnv() string {
	raw := strings.TrimRight(strings.TrimSpace(os.Getenv("CENTRAL_PUBLIC_URL")), "/")
	if raw == "" {
		return defaultPublicURL
	}
	if !strings.HasPrefix(raw, "https://") {
		log.Printf("server-central: CENTRAL_PUBLIC_URL sem https (%q); o app Android não baixa avatar por http", raw)
	}
	return raw
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

// desktopAppOrigin é a origem fixa do app desktop empacotado (esquema
// app://ffcom registrado em client/electron/main.ts). Sempre liberada, somada a
// CORS_ALLOWED_ORIGINS: nenhum site consegue enviar essa origem (só o app
// instalado), a API continua exigindo o Bearer token, e assim quem autohospeda
// não precisa configurar nada para o app desktop abrir a instância. Ver
// docs/architecture.md, "Decisão: CORS em server-channel", extensão de
// 2026-09-29.
const desktopAppOrigin = "app://ffcom"

// parseAllowedOrigins lê CORS_ALLOWED_ORIGINS (lista separada por vírgula,
// ex.: "http://localhost:5173,https://app.minhacomunidade.com") e acrescenta desktopAppOrigin. Vazio
// libera só o app desktop.
func parseAllowedOrigins(raw string) []string {
	origins := []string{desktopAppOrigin}
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" && o != desktopAppOrigin {
			origins = append(origins, o)
		}
	}
	return origins
}
