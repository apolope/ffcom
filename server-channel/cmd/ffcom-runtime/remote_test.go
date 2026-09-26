package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/release"
)

func TestFindUpdate(t *testing.T) {
	srv := newIndexServer(t)
	srv.publish(t,
		pubVersion{version: "0.7.0"},
		pubVersion{version: "0.7.1"},
		pubVersion{version: "0.7.2", minRuntime: "2.0.0"},
		pubVersion{version: "0.8.0"},
	)
	ctx := context.Background()

	cases := []struct {
		name, current, track string
		auto                 bool
		bad                  string
		want                 string // "" = nenhuma troca
		wantLog              string
	}{
		{"patch novo no minor", "0.7.0", "0.7", true, "", "0.7.1", "exige imagem com runtime >= 2.0.0"},
		{"latest", "0.7.0", "latest", true, "", "0.8.0", ""},
		{"já na mais nova", "0.8.0", "latest", true, "", "", ""},
		{"escolhida em bad", "0.7.0", "0.7", true, "0.7.1", "", "já falhou numa troca"},
		{"auto-update desligado", "0.7.0", "0.7", false, "", "", "FFCOM_AUTO_UPDATE=false"},
		{"versão fixa rebaixa", "0.8.0", "0.7.0", true, "", "0.7.0", ""},
		{"trilha sem versão", "0.7.0", "0.9", true, "", "", "nenhuma versão no índice"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, logs := newTestLauncher(t, srv)
			l.cfg.AutoUpdate = c.auto
			l.current, l.track = c.current, c.track
			if c.bad != "" {
				if err := l.store.addBad(c.bad); err != nil {
					t.Fatal(err)
				}
			}
			got, err := l.findUpdate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			gotV := ""
			if got != nil {
				gotV = got.Version
			}
			if gotV != c.want {
				t.Errorf("findUpdate = %q, quer %q", gotV, c.want)
			}
			if c.wantLog != "" && !strings.Contains(logs.String(), c.wantLog) {
				t.Errorf("log sem %q:\n%s", c.wantLog, logs.String())
			}
		})
	}
}

func TestFetchIndexRejects(t *testing.T) {
	ctx := context.Background()

	t.Run("assinatura adulterada", func(t *testing.T) {
		srv := newIndexServer(t)
		srv.publish(t, pubVersion{version: "0.7.0"})
		srv.setSig([]byte(release.Sign(srv.priv, []byte("outro índice"))))
		l, _ := newTestLauncher(t, srv)
		if _, err := l.fetch.fetchIndex(ctx); !errors.Is(err, release.ErrBadSignature) {
			t.Errorf("err = %v, quer ErrBadSignature", err)
		}
	})
	t.Run("outra chave", func(t *testing.T) {
		srv := newIndexServer(t)
		srv.publish(t, pubVersion{version: "0.7.0"})
		l, _ := newTestLauncher(t, srv)
		l.fetch.pub, _, _ = ed25519.GenerateKey(rand.Reader)
		if _, err := l.fetch.fetchIndex(ctx); !errors.Is(err, release.ErrBadSignature) {
			t.Errorf("err = %v, quer ErrBadSignature", err)
		}
	})
	t.Run("404", func(t *testing.T) {
		srv := newIndexServer(t) // nada publicado
		l, logs := newTestLauncher(t, srv)
		if _, err := l.fetch.fetchIndex(ctx); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
			t.Errorf("err = %v, quer HTTP 404", err)
		}
		// Na checagem periódica, só loga e segue.
		l.current, l.track = "0.7.0", "0.7"
		l.checkAndUpdate(ctx)
		if !strings.Contains(logs.String(), "tenta de novo no próximo intervalo") {
			t.Errorf("log inesperado:\n%s", logs.String())
		}
	})
}

func TestDownloadArtifactShaMismatch(t *testing.T) {
	srv := newIndexServer(t)
	good := []byte("binário bom")
	srv.publish(t, pubVersion{version: "0.7.1", bin: []byte("binário trocado"), sha: shaOf(good)})
	l, _ := newTestLauncher(t, srv)
	idx, err := l.fetch.fetchIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	err = l.fetch.downloadArtifact(context.Background(), l.store, idx.Versions[0], l.cfg.Platform)
	if err == nil || !strings.Contains(err.Error(), "sha256 divergente") {
		t.Fatalf("err = %v, quer sha256 divergente", err)
	}
	if _, err := os.Stat(l.store.binPath("0.7.1")); !os.IsNotExist(err) {
		t.Error("binário com sha divergente não deveria ficar instalado")
	}
	// sha divergente não entra em bad: pode ser a janela de troca de assets.
	if bad, _ := l.store.readBad(); bad["0.7.1"] {
		t.Error("sha divergente não deveria marcar a versão em bad")
	}
}

func TestInstallSeed(t *testing.T) {
	srv := newIndexServer(t)
	// Plataforma diferente da do teste, para install-seed não tentar
	// executar o artefato de mentira.
	platform := "plan9/fake"
	bin := []byte("semente")
	idx := &release.Index{Component: release.Component, GeneratedAt: time.Now().UTC().Truncate(time.Second)}
	for _, v := range []release.Version{
		{Version: "0.7.0", MinRuntime: "1.0.0", Artifacts: map[string]release.Artifact{platform: {URL: srv.srv.URL + "/bin/0.7.0", SHA256: shaOf(bin)}}},
		{Version: "0.8.0", MinRuntime: "2.0.0", Artifacts: map[string]release.Artifact{platform: {URL: srv.srv.URL + "/bin/0.8.0", SHA256: shaOf(bin)}}},
	} {
		if err := idx.Upsert(v); err != nil {
			t.Fatal(err)
		}
	}
	data, err := idx.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	srv.index, srv.sig = data, []byte(release.Sign(srv.priv, data))
	srv.artifacts = map[string][]byte{"0.7.0": bin, "0.8.0": bin}
	srv.mu.Unlock()

	l, logs := newTestLauncher(t, srv)
	dest := filepath.Join(t.TempDir(), "seed")
	if err := installSeed(context.Background(), l.fetch, "1.0.0", "0.7.0", platform, dest, time.Millisecond, l.log); err != nil {
		t.Fatal(err)
	}
	if v, err := readSeed(dest); v != "0.7.0" || err != nil {
		t.Errorf("readSeed = %q, %v", v, err)
	}
	if got, _ := sha256File(filepath.Join(dest, binName)); got != shaOf(bin) {
		t.Error("binário da semente diferente do publicado")
	}
	if !strings.Contains(logs.String(), "instalada em") {
		t.Errorf("log:\n%s", logs.String())
	}

	err = installSeed(context.Background(), l.fetch, "1.0.0", "0.8.0", platform, t.TempDir(), time.Millisecond, l.log)
	if err == nil || !strings.Contains(err.Error(), "exige runtime >= 2.0.0") {
		t.Errorf("min_runtime acima: err = %v", err)
	}
	if err := installSeed(context.Background(), l.fetch, "1.0.0", "0.9.0", platform, t.TempDir(), time.Millisecond, l.log); err == nil {
		t.Error("versão inexistente deveria falhar")
	}

	// Trilha: latest e X.Y resolvem para a mais nova aceita pelo runtime
	// (0.8.0 exige 2.0.0), e VERSION recebe a versão resolvida.
	for _, track := range []string{"latest", "0.7"} {
		dest := filepath.Join(t.TempDir(), "seed")
		if err := installSeed(context.Background(), l.fetch, "1.0.0", track, platform, dest, time.Millisecond, l.log); err != nil {
			t.Fatalf("trilha %s: %v", track, err)
		}
		if v, err := readSeed(dest); v != "0.7.0" || err != nil {
			t.Errorf("trilha %s: readSeed = %q, %v", track, v, err)
		}
	}
	if !strings.Contains(logs.String(), "trilha latest resolvida para 0.7.0") {
		t.Errorf("log sem a resolução da trilha:\n%s", logs.String())
	}
	if err := installSeed(context.Background(), l.fetch, "1.0.0", "0.9", platform, t.TempDir(), time.Millisecond, l.log); err == nil {
		t.Error("trilha sem versão deveria falhar")
	}
}
