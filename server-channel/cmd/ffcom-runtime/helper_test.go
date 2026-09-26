package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

// TestMain também serve de falso server-channel para o teste de integração
// (padrão helper process): o binário de teste é copiado para
// versions/<v>/server-channel e o lançador o executa com
// FFCOM_RUNTIME_FAKE_CHILD=1 no ambiente.
func TestMain(m *testing.M) {
	if os.Getenv("FFCOM_RUNTIME_FAKE_CHILD") == "1" {
		os.Exit(fakeChild())
	}
	os.Exit(m.Run())
}

// fakeChild imita o contrato do server-channel usado pelo lançador:
// --version, /healthz com a versão e saída limpa em SIGTERM. A versão vem do
// diretório do executável (versions/<v>/) ou do VERSION ao lado (semente).
// FFCOM_FAKE_BAD_VERSIONS lista versões que respondem /healthz com a versão
// errada; FFCOM_FAKE_CRASH=1 faz o processo sair na hora com código 3.
func fakeChild() int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	v := filepath.Base(filepath.Dir(exe))
	if data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "VERSION")); err == nil {
		v = strings.TrimSpace(string(data))
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(v)
		return 0
	}
	if os.Getenv("FFCOM_FAKE_CRASH") == "1" {
		return 3
	}
	healthVersion := v
	if slices.Contains(strings.Split(os.Getenv("FFCOM_FAKE_BAD_VERSIONS"), ","), v) {
		healthVersion = "quebrada"
	}
	srv := &http.Server{
		Addr: "127.0.0.1:" + os.Getenv("SERVER_PORT"),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"status":"ok","version":%q}`, healthVersion)
		}),
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, os.Interrupt)
	go func() {
		<-sig
		srv.Close()
	}()
	// FFCOM_FAKE_LATE_CRASH_VERSIONS: versões que passam no /healthz e caem
	// depois de FFCOM_FAKE_LATE_CRASH_AFTER (ex. panic numa rota).
	if slices.Contains(strings.Split(os.Getenv("FFCOM_FAKE_LATE_CRASH_VERSIONS"), ","), v) {
		after, _ := time.ParseDuration(os.Getenv("FFCOM_FAKE_LATE_CRASH_AFTER"))
		go func() {
			time.Sleep(after)
			fmt.Fprintln(os.Stderr, "fake server-channel: panic simulado")
			os.Exit(2)
		}()
	}
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "fake server-channel:", err)
		return 1
	}
	return 0
}

// syncBuffer é um io.Writer seguro para o logger do launcher nos testes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// pubVersion descreve uma versão publicada pelo indexServer.
type pubVersion struct {
	version    string
	minRuntime string // padrão 1.0.0
	bin        []byte // conteúdo servido
	sha        string // padrão sha256(bin); diferente simula artefato adulterado
}

// indexServer serve index.json assinado com uma chave gerada no teste,
// index.json.sig e os artefatos em /bin/<versão>.
type indexServer struct {
	srv  *httptest.Server
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey

	mu        sync.Mutex
	index     []byte
	sig       []byte
	artifacts map[string][]byte
}

func newIndexServer(t *testing.T) *indexServer {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := &indexServer{pub: pub, priv: priv, artifacts: map[string][]byte{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *indexServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.URL.Path == "/index.json" && s.index != nil:
		w.Write(s.index)
	case r.URL.Path == "/index.json.sig" && s.sig != nil:
		w.Write(s.sig)
	case strings.HasPrefix(r.URL.Path, "/bin/"):
		data, ok := s.artifacts[strings.TrimPrefix(r.URL.Path, "/bin/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	default:
		http.NotFound(w, r)
	}
}

// publish substitui o índice pelas versões dadas, para a plataforma do
// processo de teste, e assina.
func (s *indexServer) publish(t *testing.T, versions ...pubVersion) {
	t.Helper()
	platform := runtime.GOOS + "/" + runtime.GOARCH
	idx := &release.Index{Component: release.Component, GeneratedAt: time.Now().UTC().Truncate(time.Second)}
	arts := map[string][]byte{}
	for _, pv := range versions {
		if pv.minRuntime == "" {
			pv.minRuntime = "1.0.0"
		}
		if pv.bin == nil {
			pv.bin = []byte("binário " + pv.version)
		}
		if pv.sha == "" {
			sum := sha256.Sum256(pv.bin)
			pv.sha = hex.EncodeToString(sum[:])
		}
		arts[pv.version] = pv.bin
		if err := idx.Upsert(release.Version{
			Version:     pv.version,
			MinRuntime:  pv.minRuntime,
			PublishedAt: idx.GeneratedAt,
			Artifacts:   map[string]release.Artifact{platform: {URL: s.srv.URL + "/bin/" + pv.version, SHA256: pv.sha}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := idx.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index, s.artifacts = data, arts
	s.sig = []byte(release.Sign(s.priv, data) + "\n")
}

func (s *indexServer) setSig(sig []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sig = sig
}

// newTestLauncher monta um launcher apontando para srv, com runtime 1.0.0,
// diretórios temporários e log capturado.
func newTestLauncher(t *testing.T, srv *indexServer) (*launcher, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	cfg := config{
		RuntimeVersion: "1.0.0",
		Platform:       runtime.GOOS + "/" + runtime.GOARCH,
		AutoUpdate:     true,
		Interval:       time.Hour,
		IndexURL:       srv.srv.URL,
		PubKey:         srv.pub,
		RuntimeDir:     filepath.Join(t.TempDir(), "runtime"),
		SeedDir:        filepath.Join(t.TempDir(), "seed"),
		HealthURL:      "http://127.0.0.1:1/healthz",
	}
	l := newLauncher(cfg, log.New(logs, "ffcom-runtime: ", log.Lmsgprefix))
	return l, logs
}

func shaOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
