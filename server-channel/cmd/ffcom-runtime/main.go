// ffcom-runtime é o lançador do container evergreen de server-channel (ver
// docs/architecture.md, "Decisão: container evergreen em `server-channel`").
// Roda como PID 1 do container: escolhe a versão do serviço, baixa e
// verifica o binário pelo índice assinado do release channel-stable,
// supervisiona o processo filho server-channel e troca de versão com
// rollback.
//
// Subcomandos:
//
//	ffcom-runtime [run]
//	ffcom-runtime install-seed -version X.Y.Z [-platform linux/amd64] [-dest /opt/ffcom/seed]
//	ffcom-runtime --version
//
// A versão deste binário (-ldflags "-X main.version=1.0.0") é a do contrato
// do container (runtime), não a do serviço: é ela que se compara com o
// min_runtime de cada versão do índice.
//
// Build sem ldflags ("dev"): recusado por padrão, porque uma versão de
// runtime inventada faria o lançador aceitar binários que exigem um contrato
// que ele talvez não cumpra. Só com FFCOM_RUNTIME_DEV=1 o build "dev" roda,
// tratado como runtime 999.0.0 (aceita qualquer min_runtime) e com aviso no
// log. Serve para testar o lançador localmente, nunca para imagem publicada.
//
// Variáveis de ambiente (todas opcionais; todas são repassadas ao filho):
//
//	FFCOM_CHANNEL_VERSION    latest | X.Y | X.Y.Z (padrão: X.Y da semente)
//	FFCOM_AUTO_UPDATE        true (padrão) | false (só loga versão nova)
//	FFCOM_UPDATE_INTERVAL    duração Go, padrão 1h, mínimo 1m, +até 25% de jitter
//	FFCOM_RELEASE_INDEX_URL  base de index.json/index.json.sig (padrão: release channel-stable)
//	FFCOM_RELEASE_PUBKEY     chave pública ed25519 em base64 no lugar da embutida (forks)
//	FFCOM_RUNTIME_DIR        padrão /data/runtime
//	FFCOM_SEED_DIR           padrão /opt/ffcom/seed
//	SERVER_PORT              porta do server-channel (padrão 8080), para o /healthz
//	FFCOM_RUNTIME_DEV        1 = aceita build "dev" como runtime 999.0.0
//
// Sinais: SIGTERM/SIGINT repassam SIGTERM ao filho, esperam até 30s e saem
// com 0; SIGHUP força a checagem de atualização na hora.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

// version é a versão do contrato do container, via ldflags.
var version = "dev"

// embeddedPubKey é a chave pública que verifica index.json.sig. Gerada com
// `release-tool keygen`; a privada fica só no secret
// CHANNEL_RELEASE_SIGNING_KEY do environment channel-release.
//
//go:embed release.pub
var embeddedPubKey []byte

// devRuntimeVersion é a versão de runtime assumida pelo build "dev" com
// FFCOM_RUNTIME_DEV=1.
const devRuntimeVersion = "999.0.0"

const usage = `uso:
  ffcom-runtime [run]
  ffcom-runtime install-seed -version X.Y.Z [-platform os/arch] [-dest /opt/ffcom/seed]
  ffcom-runtime --version
`

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("ffcom-runtime: ")
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

func realMain(args []string, stdout, stderr io.Writer) int {
	cmd := "run"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "--version", "-version":
		fmt.Fprintln(stdout, version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "run":
		if len(args) > 0 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		cfg, err := loadConfig(os.Getenv, version)
		if err != nil {
			log.Print(err)
			return 1
		}
		return newLauncher(cfg, log.Default()).run(context.Background())
	case "install-seed":
		if err := cmdInstallSeed(args, os.Getenv, log.Default()); err != nil {
			log.Printf("install-seed: %v", err)
			return 1
		}
		return 0
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

// runtimeVersion valida a versão de build do lançador. Ver o comentário do
// pacote sobre o build "dev".
func runtimeVersion(v string, getenv func(string) string) (rt string, dev bool, err error) {
	if _, err := release.ParseSemver(v); err == nil {
		return v, false, nil
	}
	if v == "dev" {
		if getenv("FFCOM_RUNTIME_DEV") == "1" {
			return devRuntimeVersion, true, nil
		}
		return "", false, errors.New(`build sem versão ("dev"): compile com -ldflags "-X main.version=X.Y.Z" ou, só em desenvolvimento, defina FFCOM_RUNTIME_DEV=1 (runtime tratado como ` + devRuntimeVersion + `)`)
	}
	return "", false, fmt.Errorf("versão do runtime %q não é X.Y.Z", v)
}
