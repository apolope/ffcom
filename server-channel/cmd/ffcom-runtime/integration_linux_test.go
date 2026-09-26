//go:build linux

package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Testes com processo filho de verdade: o próprio binário de teste faz o
// papel de server-channel (ver fakeChild em helper_test.go).

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

func healthVersion(port string) string {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var body struct{ Version string }
	json.NewDecoder(resp.Body).Decode(&body)
	return body.Version
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool, logs *syncBuffer) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("tempo esgotado esperando %s; log do lançador:\n%s", what, logs.String())
}

func readTrim(path string) string {
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}

// writeSeed copia o binário de teste como semente da versão v.
func writeSeed(t *testing.T, dir, v string) []byte {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, binName), bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte(v+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestSwapAndRollback(t *testing.T) {
	srv := newIndexServer(t)
	l, logs := newTestLauncher(t, srv)
	bin := writeSeed(t, l.cfg.SeedDir, "0.7.0")
	srv.publish(t, pubVersion{version: "0.7.0", bin: bin}, pubVersion{version: "0.7.1", bin: bin})

	port := freePort(t)
	t.Setenv("FFCOM_RUNTIME_FAKE_CHILD", "1")
	t.Setenv("SERVER_PORT", port)
	t.Setenv("FFCOM_FAKE_BAD_VERSIONS", "0.7.2")
	l.childEnv = os.Environ()
	l.cfg.HealthURL = "http://127.0.0.1:" + port + "/healthz"
	l.stopTimeout, l.healthTimeout = 5*time.Second, 5*time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- l.run(ctx) }()

	// Boot pela semente 0.7.0 e troca imediata para 0.7.1 (trilha padrão 0.7).
	waitFor(t, 30*time.Second, "troca para 0.7.1", func() bool {
		return readTrim(l.store.currentPath()) == "0.7.1" && healthVersion(port) == "0.7.1"
	}, logs)
	if !strings.Contains(logs.String(), "troca concluída: 0.7.0 → 0.7.1") {
		t.Errorf("log sem a troca:\n%s", logs.String())
	}

	// 0.7.2 sobe mas responde /healthz com a versão errada: rollback.
	srv.publish(t, pubVersion{version: "0.7.0", bin: bin}, pubVersion{version: "0.7.1", bin: bin}, pubVersion{version: "0.7.2", bin: bin})
	l.hup <- struct{}{}
	waitFor(t, 30*time.Second, "rollback de 0.7.2", func() bool {
		return strings.Contains(logs.String(), "rollback concluído") && healthVersion(port) == "0.7.1"
	}, logs)
	if bad := readTrim(l.store.badPath()); bad != "0.7.2" {
		t.Errorf("bad = %q, quer 0.7.2", bad)
	}
	if cur := readTrim(l.store.currentPath()); cur != "0.7.1" {
		t.Errorf("current = %q depois do rollback", cur)
	}
	entries, _ := os.ReadDir(l.store.versionsDir())
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "0.7.0,0.7.1" {
		t.Errorf("versions/ = %v, quer atual e anterior", names)
	}

	// Nova checagem não tenta 0.7.2 de novo.
	l.hup <- struct{}{}
	waitFor(t, 10*time.Second, "checagem ignorando bad", func() bool {
		return strings.Contains(logs.String(), "já falhou numa troca")
	}, logs)

	// Encerramento: SIGTERM no filho e saída 0.
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("run saiu com %d, quer 0", code)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run não terminou depois do cancelamento")
	}
	if v := healthVersion(port); v != "" {
		t.Errorf("filho ainda respondendo depois do encerramento (%q)", v)
	}
}

func TestBootPrefersNewerSeed(t *testing.T) {
	srv := newIndexServer(t) // índice vazio: checagem falha e só loga
	l, logs := newTestLauncher(t, srv)
	bin := writeSeed(t, l.cfg.SeedDir, "0.7.3")
	// current 0.7.1 instalada de uma imagem anterior.
	if err := l.store.install("0.7.1", strings.NewReader(string(bin)), shaOf(bin)); err != nil {
		t.Fatal(err)
	}
	if err := l.store.writeCurrent("0.7.1"); err != nil {
		t.Fatal(err)
	}

	port := freePort(t)
	t.Setenv("FFCOM_RUNTIME_FAKE_CHILD", "1")
	t.Setenv("SERVER_PORT", port)
	l.childEnv = os.Environ()
	l.cfg.HealthURL = "http://127.0.0.1:" + port + "/healthz"
	l.stopTimeout, l.healthTimeout = 5*time.Second, 5*time.Second

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- l.run(ctx) }()
	waitFor(t, 20*time.Second, "boot pela semente 0.7.3", func() bool {
		return readTrim(l.store.currentPath()) == "0.7.3" && healthVersion(port) == "0.7.3"
	}, logs)
	cancel()
	if code := <-done; code != 0 {
		t.Errorf("run saiu com %d", code)
	}
}

func TestCrashLoopGivesUp(t *testing.T) {
	srv := newIndexServer(t)
	l, logs := newTestLauncher(t, srv)
	writeSeed(t, l.cfg.SeedDir, "0.7.0")
	t.Setenv("FFCOM_RUNTIME_FAKE_CHILD", "1")
	t.Setenv("FFCOM_FAKE_CRASH", "1")
	t.Setenv("SERVER_PORT", freePort(t))
	l.childEnv = os.Environ()
	l.crashes.base = 10 * time.Millisecond

	done := make(chan int, 1)
	go func() { done <- l.run(context.Background()) }()
	select {
	case code := <-done:
		if code == 0 {
			t.Error("laço de crash deveria sair com código != 0")
		}
		if !strings.Contains(logs.String(), "saindo para o Docker reiniciar") {
			t.Errorf("log:\n%s", logs.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("run não desistiu; log:\n%s", logs.String())
	}
}

// Versão que passa no /healthz na troca e depois cai a cada poucos
// segundos: depois de maxCrashes saídas desde a troca, volta para a
// anterior sem sair e marca a nova em bad. stableAfter curto faz cada
// saída contar como "não seguida" para crashPolicy, cobrindo o caso de
// uma versão que cai minutos depois de subir.
func TestRollbackAfterLateCrashes(t *testing.T) {
	srv := newIndexServer(t)
	l, logs := newTestLauncher(t, srv)
	bin := writeSeed(t, l.cfg.SeedDir, "0.7.1")
	srv.publish(t, pubVersion{version: "0.7.1", bin: bin}, pubVersion{version: "0.7.2", bin: bin})

	port := freePort(t)
	t.Setenv("FFCOM_RUNTIME_FAKE_CHILD", "1")
	t.Setenv("SERVER_PORT", port)
	t.Setenv("FFCOM_FAKE_LATE_CRASH_VERSIONS", "0.7.2")
	t.Setenv("FFCOM_FAKE_LATE_CRASH_AFTER", "1500ms")
	l.childEnv = os.Environ()
	l.cfg.HealthURL = "http://127.0.0.1:" + port + "/healthz"
	l.stopTimeout, l.healthTimeout = 5*time.Second, 5*time.Second
	l.crashes.base = 10 * time.Millisecond
	l.crashes.stableAfter = 300 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- l.run(ctx) }()

	waitFor(t, 30*time.Second, "rollback de 0.7.2 por crash loop", func() bool {
		return strings.Contains(logs.String(), "ROLLBACK: versão 0.7.2") && healthVersion(port) == "0.7.1"
	}, logs)
	if !strings.Contains(logs.String(), "troca concluída: 0.7.1 → 0.7.2") {
		t.Errorf("0.7.2 deveria ter passado no /healthz antes de cair:\n%s", logs.String())
	}
	if bad := readTrim(l.store.badPath()); bad != "0.7.2" {
		t.Errorf("bad = %q, quer 0.7.2", bad)
	}
	if cur := readTrim(l.store.currentPath()); cur != "0.7.1" {
		t.Errorf("current = %q, quer 0.7.1", cur)
	}
	if _, err := os.Stat(l.store.lastUpdatePath()); !os.IsNotExist(err) {
		t.Error("last-update deveria ser apagado no rollback")
	}
	// Continua no ar em 0.7.1, sem sair.
	time.Sleep(2 * time.Second)
	select {
	case code := <-done:
		t.Fatalf("run saiu (%d) em vez de seguir na anterior; log:\n%s", code, logs.String())
	default:
	}
	if v := healthVersion(port); v != "0.7.1" {
		t.Errorf("healthz = %q depois do rollback", v)
	}
	cancel()
	if code := <-done; code != 0 {
		t.Errorf("run saiu com %d", code)
	}
}

// Pin exato do operador não é pulado: sem rollback, comportamento antigo.
func TestNoRollbackForExactPin(t *testing.T) {
	srv := newIndexServer(t)
	l, logs := newTestLauncher(t, srv)
	st := l.store
	for _, v := range []string{"0.7.1", "0.7.2"} {
		b := []byte(v)
		if err := st.install(v, strings.NewReader(v), shaOf(b)); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.writeLastUpdate(lastUpdate{From: "0.7.1", To: "0.7.2", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	l.current, l.track = "0.7.2", "0.7.2"
	if l.rollbackAfterCrashes() {
		t.Error("não deveria fazer rollback do pin exato")
	}
	if !strings.Contains(logs.String(), "pin de FFCOM_CHANNEL_VERSION") {
		t.Errorf("log:\n%s", logs.String())
	}
	// Troca antiga (> 24h) também não.
	l.track = "0.7"
	st.writeLastUpdate(lastUpdate{From: "0.7.1", To: "0.7.2", At: time.Now().Add(-25 * time.Hour)})
	if l.rollbackAfterCrashes() {
		t.Error("não deveria fazer rollback de troca antiga")
	}
	// Recente, com a anterior íntegra: faz.
	st.writeLastUpdate(lastUpdate{From: "0.7.1", To: "0.7.2", At: time.Now()})
	if !l.rollbackAfterCrashes() || l.current != "0.7.1" {
		t.Errorf("rollback esperado; current = %q", l.current)
	}
}
