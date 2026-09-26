package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

// cmdInstallSeed é o subcomando usado no build da imagem para gravar o
// binário semente (ver docs/architecture.md, item 6 da decisão do container
// evergreen): mesma verificação do auto-update (índice assinado, min_runtime
// deste lançador, sha256), resultado em <dest>/server-channel e
// <dest>/VERSION.
func cmdInstallSeed(args []string, getenv func(string) string, logger *log.Logger) error {
	fl := flag.NewFlagSet("install-seed", flag.ContinueOnError)
	ver := fl.String("version", "", "versão do server-channel (X.Y.Z)")
	platform := fl.String("platform", runtime.GOOS+"/"+runtime.GOARCH, "plataforma do artefato (os/arch)")
	dest := fl.String("dest", defaultSeedDir, "diretório da semente")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if fl.NArg() > 0 {
		return fmt.Errorf("argumentos inesperados: %v", fl.Args())
	}
	if _, err := release.ParseSemver(*ver); err != nil {
		return fmt.Errorf("-version: %w", err)
	}
	cfg, err := loadConfig(getenv, version)
	if err != nil {
		return err
	}
	for _, w := range cfg.Warnings {
		logger.Print(w)
	}
	f := fetcher{client: &http.Client{}, indexURL: cfg.IndexURL, pub: cfg.PubKey}
	return installSeed(context.Background(), f, cfg.RuntimeVersion, *ver, *platform, *dest, 5*time.Second, logger)
}

func installSeed(ctx context.Context, f fetcher, runtimeVer, ver, platform, dest string, retryDelay time.Duration, logger *log.Logger) error {
	// Três tentativas: a troca dos assets de channel-stable deixa janelas de
	// segundos com 404 ou assinatura divergente.
	var idx *release.Index
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if idx, err = f.fetchIndex(ctx); err == nil {
			break
		}
		if attempt < 3 {
			logger.Printf("baixar índice: %v; tentando de novo em %s", err, retryDelay)
			time.Sleep(retryDelay)
		}
	}
	if err != nil {
		return err
	}

	chosen, skipped, err := release.Resolve(idx, ver, runtimeVer, platform)
	if err != nil {
		if skipped != nil {
			return fmt.Errorf("versão %s exige runtime >= %s, este lançador é %s", skipped.Version, skipped.MinRuntime, runtimeVer)
		}
		return fmt.Errorf("versão %s para %s: %w", ver, platform, err)
	}
	art := chosen.Artifacts[platform]

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, artifactTimeout)
	defer cancel()
	body, err := f.open(ctx, art.URL)
	if err != nil {
		return err
	}
	defer body.Close()
	tmp, err := downloadVerified(dest, body, art.SHA256)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	bin := filepath.Join(dest, binName)
	if err := os.Rename(tmp, bin); err != nil {
		return err
	}
	// Só dá para executar o binário se ele for desta plataforma (num build
	// cruzado, -platform difere da do lançador).
	if platform == runtime.GOOS+"/"+runtime.GOARCH {
		if err := checkBinaryVersion(ctx, bin, ver, os.Environ()); err != nil {
			os.Remove(bin)
			return err
		}
	}
	if err := writeFileAtomic(filepath.Join(dest, "VERSION"), []byte(ver+"\n"), 0o644); err != nil {
		return err
	}
	logger.Printf("semente server-channel %s (%s) instalada em %s, sha256 %s", ver, platform, dest, art.SHA256)
	return nil
}
