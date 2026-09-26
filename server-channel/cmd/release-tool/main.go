// release-tool gera o par de chaves, monta e assina o índice de versões do
// server-channel usado pelo container evergreen (ver docs/architecture.md,
// "Decisão: container evergreen em `server-channel`", e o pacote
// internal/release). Roda no workflow .github/workflows/deploy-ffcom-channel.yml
// e, uma única vez, na máquina do autor para gerar as chaves.
//
// Gerar o par de chaves (uma vez só, fora do repositório):
//
//	go run ./cmd/release-tool keygen -pub release.pub -priv release.key
//
// Depois:
//  1. copie release.pub para server-channel/cmd/ffcom-runtime/release.pub e
//     commite (é a chave que o lançador embute via go:embed e que o workflow
//     usa para verificar o índice);
//  2. cadastre o conteúdo de release.key (uma linha base64) como secret
//     CHANNEL_RELEASE_SIGNING_KEY no environment channel-release do GitHub
//     (Settings > Environments, com aprovação manual);
//  3. apague release.key da máquina ou guarde num cofre offline. Nunca
//     commite a privada.
//
// Subcomandos:
//
//	keygen -pub release.pub -priv release.key
//	index add -index index.json -version 0.7.0 -min-runtime 1.0.0 \
//	    -artifact linux/amd64=<url>=<arquivo local> [-artifact ...]
//	sign -index index.json -key-env CHANNEL_RELEASE_SIGNING_KEY -out index.json.sig
//	verify -index index.json -sig index.json.sig -pub release.pub
//
// A chave privada de sign vem só de variável de ambiente, nunca de argumento
// de linha de comando (que fica visível em ps e nos logs do runner).
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

const usage = `uso:
  release-tool keygen -pub release.pub -priv release.key
  release-tool index add -index index.json -version X.Y.Z -min-runtime X.Y.Z -artifact os/arch=<url>=<arquivo> [...]
  release-tool sign -index index.json -key-env CHANNEL_RELEASE_SIGNING_KEY -out index.json.sig
  release-tool verify -index index.json -sig index.json.sig -pub release.pub
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "release-tool:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return errors.New("subcomando ausente")
	}
	switch args[0] {
	case "keygen":
		return cmdKeygen(args[1:], stdout, stderr)
	case "index":
		if len(args) < 2 || args[1] != "add" {
			fmt.Fprint(stderr, usage)
			return errors.New(`subcomando de index ausente (só "add" existe)`)
		}
		return cmdIndexAdd(args[2:], stdout)
	case "sign":
		return cmdSign(args[1:], stdout)
	case "verify":
		return cmdVerify(args[1:], stdout)
	default:
		fmt.Fprint(stderr, usage)
		return fmt.Errorf("subcomando desconhecido %q", args[0])
	}
}

func cmdKeygen(args []string, stdout, stderr io.Writer) error {
	fl := flag.NewFlagSet("keygen", flag.ContinueOnError)
	pubPath := fl.String("pub", "release.pub", "arquivo da chave pública")
	privPath := fl.String("priv", "release.key", "arquivo da chave privada (criado com 0600, não sobrescreve)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	// A privada primeiro: se ela já existir, falha antes de trocar a pública.
	if err := release.WritePrivateKey(*privPath, priv); err != nil {
		return fmt.Errorf("gravar chave privada: %w", err)
	}
	if err := release.WritePublicKey(*pubPath, pub); err != nil {
		return fmt.Errorf("gravar chave pública: %w", err)
	}
	fmt.Fprintf(stdout, "chave pública: %s\nchave privada: %s\n", *pubPath, *privPath)
	fmt.Fprintf(stderr, "AVISO: NUNCA commite %s. Cadastre o conteúdo como secret CHANNEL_RELEASE_SIGNING_KEY no environment channel-release e apague o arquivo local ou guarde-o offline.\n", *privPath)
	return nil
}

// artifactFlags acumula -artifact os/arch=<url>=<arquivo>.
type artifactFlags []string

func (a *artifactFlags) String() string     { return strings.Join(*a, " ") }
func (a *artifactFlags) Set(v string) error { *a = append(*a, v); return nil }

func cmdIndexAdd(args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("index add", flag.ContinueOnError)
	indexPath := fl.String("index", "index.json", "índice a criar ou atualizar")
	version := fl.String("version", "", "versão do serviço (X.Y.Z)")
	minRuntime := fl.String("min-runtime", "", "versão mínima do runtime (X.Y.Z)")
	var artifacts artifactFlags
	fl.Var(&artifacts, "artifact", "os/arch=<url>=<arquivo local para o sha256> (repetível)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if _, err := release.ParseSemver(*version); err != nil {
		return fmt.Errorf("-version: %w", err)
	}
	if _, err := release.ParseSemver(*minRuntime); err != nil {
		return fmt.Errorf("-min-runtime: %w", err)
	}
	if len(artifacts) == 0 {
		return errors.New("pelo menos um -artifact é obrigatório")
	}

	now := time.Now().UTC().Truncate(time.Second)
	entry := release.Version{
		Version:     *version,
		MinRuntime:  *minRuntime,
		PublishedAt: now,
		Artifacts:   map[string]release.Artifact{},
	}
	for _, spec := range artifacts {
		platform, url, file, err := splitArtifact(spec)
		if err != nil {
			return err
		}
		if _, dup := entry.Artifacts[platform]; dup {
			return fmt.Errorf("-artifact: plataforma %s repetida", platform)
		}
		sum, err := sha256File(file)
		if err != nil {
			return err
		}
		entry.Artifacts[platform] = release.Artifact{URL: url, SHA256: sum}
	}

	// O índice existente não é verificado aqui: quem chama (o workflow)
	// verifica a assinatura do índice baixado antes de estendê-lo.
	idx := &release.Index{Component: release.Component}
	data, err := os.ReadFile(*indexPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// índice novo
	case err != nil:
		return err
	default:
		if idx, err = release.ParseIndex(data); err != nil {
			return err
		}
	}
	if err := idx.Upsert(entry); err != nil {
		return err
	}
	idx.GeneratedAt = now
	if err := idx.Validate(); err != nil {
		return err
	}
	out, err := idx.Marshal()
	if err != nil {
		return err
	}
	if err := writeFileAtomic(*indexPath, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s: versão %s adicionada (%d versões no índice)\n", *indexPath, *version, len(idx.Versions))
	return nil
}

// splitArtifact separa "os/arch=<url>=<arquivo>": a plataforma vai até o
// primeiro "=", o arquivo começa depois do último, e o meio é a URL (que
// pode conter "=" numa query string).
func splitArtifact(spec string) (platform, url, file string, err error) {
	first := strings.Index(spec, "=")
	last := strings.LastIndex(spec, "=")
	if first < 0 || first == last {
		return "", "", "", fmt.Errorf("-artifact %q: esperado os/arch=<url>=<arquivo>", spec)
	}
	platform, url, file = spec[:first], spec[first+1:last], spec[last+1:]
	if platform == "" || url == "" || file == "" || !strings.Contains(platform, "/") {
		return "", "", "", fmt.Errorf("-artifact %q: esperado os/arch=<url>=<arquivo>", spec)
	}
	return platform, url, file, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func cmdSign(args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("sign", flag.ContinueOnError)
	indexPath := fl.String("index", "index.json", "índice a assinar")
	keyEnv := fl.String("key-env", "CHANNEL_RELEASE_SIGNING_KEY", "variável de ambiente com a chave privada (base64)")
	outPath := fl.String("out", "index.json.sig", "arquivo da assinatura")
	if err := fl.Parse(args); err != nil {
		return err
	}
	keyData := os.Getenv(*keyEnv)
	if keyData == "" {
		return fmt.Errorf("variável de ambiente %s vazia ou ausente", *keyEnv)
	}
	priv, err := release.ParsePrivateKey([]byte(keyData))
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*indexPath)
	if err != nil {
		return err
	}
	// Não assina um índice malformado: o lançador recusaria depois.
	if _, err := release.ParseIndex(data); err != nil {
		return err
	}
	if err := writeFileAtomic(*outPath, []byte(release.Sign(priv, data)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s assinado em %s\n", *indexPath, *outPath)
	return nil
}

func cmdVerify(args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("verify", flag.ContinueOnError)
	indexPath := fl.String("index", "index.json", "índice")
	sigPath := fl.String("sig", "index.json.sig", "assinatura")
	pubPath := fl.String("pub", "release.pub", "chave pública")
	if err := fl.Parse(args); err != nil {
		return err
	}
	pub, err := release.ReadPublicKey(*pubPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*indexPath)
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(*sigPath)
	if err != nil {
		return err
	}
	if err := release.Verify(pub, data, string(sig)); err != nil {
		return err
	}
	idx, err := release.ParseIndex(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s: assinatura válida, %d versões\n", *indexPath, len(idx.Versions))
	return nil
}

// writeFileAtomic grava num temporário ao lado e renomeia, para um erro no
// meio não deixar o índice ou a assinatura truncados.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
