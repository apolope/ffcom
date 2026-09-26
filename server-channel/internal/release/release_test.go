package release

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sha = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func ver(v, minRT string, platforms ...string) Version {
	arts := map[string]Artifact{}
	for _, p := range platforms {
		arts[p] = Artifact{URL: "https://example.invalid/" + v + "/" + p, SHA256: sha}
	}
	return Version{Version: v, MinRuntime: minRT, PublishedAt: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC), Artifacts: arts}
}

func TestSemver(t *testing.T) {
	for _, bad := range []string{"", "1", "1.2", "v1.2.3", "1.2.3-rc1", "01.2.3", "1.2.x", "1..3", "1.2.3.4"} {
		if _, err := ParseSemver(bad); err == nil {
			t.Errorf("ParseSemver(%q) deveria falhar", bad)
		}
	}
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"0.9.0", "0.10.0", -1},
		{"1.0.10", "1.0.9", 1},
		{"2.0.0", "1.99.99", 1},
	}
	for _, c := range cases {
		a, _ := ParseSemver(c.a)
		b, _ := ParseSemver(c.b)
		if got := a.Compare(b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, quer %d", c.a, c.b, got, c.want)
		}
	}
	if s, _ := ParseSemver("0.10.3"); s.String() != "0.10.3" {
		t.Errorf("String() = %q", s.String())
	}
}

func TestSignVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	idx := &Index{Component: Component, GeneratedAt: time.Now().UTC().Truncate(time.Second), Versions: []Version{ver("0.7.0", "1.0.0", "linux/amd64")}}
	data, err := idx.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	sig := Sign(priv, data)
	if err := Verify(pub, data, sig+"\n"); err != nil {
		t.Fatalf("assinatura válida recusada: %v", err)
	}

	tampered := bytes.Replace(data, []byte("0.7.0"), []byte("0.7.1"), 1)
	if err := Verify(pub, tampered, sig); !errors.Is(err, ErrBadSignature) {
		t.Errorf("índice adulterado: err = %v, quer ErrBadSignature", err)
	}
	if err := Verify(pub, append(append([]byte{}, data...), '\n'), sig); !errors.Is(err, ErrBadSignature) {
		t.Errorf("byte a mais: err = %v, quer ErrBadSignature", err)
	}
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := Verify(otherPub, data, sig); !errors.Is(err, ErrBadSignature) {
		t.Errorf("outra chave: err = %v, quer ErrBadSignature", err)
	}
	if err := Verify(pub, data, "não é base64"); !errors.Is(err, ErrBadSignature) {
		t.Errorf("base64 inválido: err = %v, quer ErrBadSignature", err)
	}
	if err := Verify(pub, data, "AAAA"); !errors.Is(err, ErrBadSignature) {
		t.Errorf("assinatura curta: err = %v, quer ErrBadSignature", err)
	}
}

func TestKeyFiles(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	pubPath, privPath := filepath.Join(dir, "release.pub"), filepath.Join(dir, "release.key")
	if err := WritePublicKey(pubPath, pub); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateKey(privPath, priv); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateKey(privPath, priv); err == nil {
		t.Error("WritePrivateKey deveria recusar sobrescrever")
	}
	gotPub, err := ReadPublicKey(pubPath)
	if err != nil || !gotPub.Equal(pub) {
		t.Fatalf("ReadPublicKey: %v", err)
	}
	gotPriv, err := ReadPrivateKey(privPath)
	if err != nil || !gotPriv.Equal(priv) {
		t.Fatalf("ReadPrivateKey: %v", err)
	}
	// Semente de 32 bytes também é aceita.
	fromSeed, err := ParsePrivateKey(EncodeKey(priv.Seed()))
	if err != nil || !fromSeed.Equal(priv) {
		t.Fatalf("ParsePrivateKey(semente): %v", err)
	}
	// Metade pública trocada é recusada.
	broken := append(ed25519.PrivateKey{}, priv...)
	broken[40] ^= 0xff
	if _, err := ParsePrivateKey(EncodeKey(broken)); err == nil {
		t.Error("chave privada inconsistente deveria ser recusada")
	}
	if _, err := ParsePublicKey([]byte("AAAA\n")); err == nil {
		t.Error("chave pública curta deveria ser recusada")
	}
	if _, err := ParsePublicKey([]byte("  \n")); err == nil {
		t.Error("chave pública vazia deveria ser recusada")
	}
}

func TestParseIndex(t *testing.T) {
	good := `{"component":"server-channel","generated_at":"2026-09-26T12:00:00Z","versions":[{"version":"0.7.0","min_runtime":"1.0.0","published_at":"2026-09-26T12:00:00Z","artifacts":{"linux/amd64":{"url":"https://x/a","sha256":"` + sha + `"}},"extra":1}]}`
	idx, err := ParseIndex([]byte(good))
	if err != nil {
		t.Fatalf("índice válido recusado: %v", err)
	}
	if len(idx.Versions) != 1 || idx.Versions[0].Artifacts["linux/amd64"].URL != "https://x/a" {
		t.Errorf("índice lido errado: %+v", idx)
	}

	bad := map[string]string{
		"json quebrado":    `{"component":`,
		"component errado": strings.Replace(good, `"server-channel"`, `"server-central"`, 1),
		"versão inválida":  strings.Replace(good, `"version":"0.7.0"`, `"version":"v0.7.0"`, 1),
		"min_runtime ruim": strings.Replace(good, `"min_runtime":"1.0.0"`, `"min_runtime":"1.0"`, 1),
		"sha curto":        strings.Replace(good, sha, sha[:63], 1),
		"sha não hex":      strings.Replace(good, sha, "z"+sha[1:], 1),
		"sha maiúsculo":    strings.Replace(good, sha, strings.ToUpper(sha), 1),
		"url vazia":        strings.Replace(good, `"https://x/a"`, `""`, 1),
		"plataforma ruim":  strings.Replace(good, `"linux/amd64"`, `"linux"`, 1),
		"data inválida":    strings.Replace(good, `"2026-09-26T12:00:00Z","versions"`, `"ontem","versions"`, 1),
		"versão duplicada": strings.Replace(good, `]}`, `,{"version":"0.7.0","min_runtime":"1.0.0","published_at":"2026-09-26T12:00:00Z","artifacts":{}}]}`, 1),
	}
	for name, s := range bad {
		if _, err := ParseIndex([]byte(s)); err == nil {
			t.Errorf("%s: ParseIndex deveria falhar", name)
		}
	}
}

func TestUpsertAndMarshalRoundTrip(t *testing.T) {
	idx := &Index{Component: Component, GeneratedAt: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)}
	for _, v := range []Version{ver("0.6.0", "1.0.0", "linux/amd64"), ver("0.10.0", "1.0.0", "linux/amd64"), ver("0.7.0", "1.0.0", "linux/amd64")} {
		if err := idx.Upsert(v); err != nil {
			t.Fatal(err)
		}
	}
	// Substitui 0.7.0 mantendo uma entrada só.
	if err := idx.Upsert(ver("0.7.0", "1.1.0", "linux/arm64")); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range idx.Versions {
		got = append(got, v.Version)
	}
	if strings.Join(got, ",") != "0.10.0,0.7.0,0.6.0" {
		t.Errorf("ordem = %v", got)
	}
	if idx.Versions[1].MinRuntime != "1.1.0" {
		t.Errorf("0.7.0 não foi substituída: %+v", idx.Versions[1])
	}
	if err := idx.Upsert(ver("0.7", "1.0.0")); err == nil {
		t.Error("Upsert com versão inválida deveria falhar")
	}

	data, err := idx.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"generated_at": "2026-09-26T12:00:00Z"`)) || !bytes.HasSuffix(data, []byte("}\n")) {
		t.Errorf("formato inesperado:\n%s", data)
	}
	back, err := ParseIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := back.Marshal()
	if !bytes.Equal(data, again) {
		t.Error("Marshal não é estável num round trip")
	}
}

func TestResolve(t *testing.T) {
	idx := &Index{Component: Component, Versions: []Version{
		ver("1.0.0", "2.0.0", "linux/amd64", "linux/arm64"), // exige runtime 2
		ver("0.8.1", "1.0.0", "linux/amd64"),                // sem arm64
		ver("0.8.0", "1.0.0", "linux/amd64", "linux/arm64"),
		ver("0.7.2", "1.1.0", "linux/amd64", "linux/arm64"), // exige runtime 1.1
		ver("0.7.1", "1.0.0", "linux/amd64", "linux/arm64"),
		ver("0.7.0", "1.0.0", "linux/amd64", "linux/arm64"),
	}}

	cases := []struct {
		name, track, runtime, platform string
		want, skipped                  string // "" = nenhuma
		wantErr                        bool
	}{
		{"latest pula a que exige runtime novo", "latest", "1.0.0", "linux/amd64", "0.8.1", "1.0.0", false},
		{"latest com runtime novo", "latest", "2.0.0", "linux/amd64", "1.0.0", "", false},
		{"latest ignora plataforma ausente", "latest", "1.0.0", "linux/arm64", "0.8.0", "1.0.0", false},
		{"minor pega maior patch compatível", "0.7", "1.0.0", "linux/amd64", "0.7.1", "0.7.2", false},
		{"minor com runtime suficiente", "0.7", "1.1.0", "linux/amd64", "0.7.2", "", false},
		{"minor 0.8 em arm64", "0.8", "1.0.0", "linux/arm64", "0.8.0", "", false},
		{"exata", "0.7.0", "1.0.0", "linux/amd64", "0.7.0", "", false},
		{"exata bloqueada por runtime", "0.7.2", "1.0.0", "linux/amd64", "", "0.7.2", true},
		{"exata sem plataforma", "0.8.1", "1.0.0", "linux/arm64", "", "", true},
		{"minor inexistente", "0.9", "1.0.0", "linux/amd64", "", "", true},
		{"plataforma inexistente", "latest", "9.0.0", "windows/amd64", "", "", true},
		{"trilha inválida", "0", "1.0.0", "linux/amd64", "", "", true},
		{"trilha inválida 2", "v0.7", "1.0.0", "linux/amd64", "", "", true},
		{"runtime inválido", "latest", "dev", "linux/amd64", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, skipped, err := Resolve(idx, c.track, c.runtime, c.platform)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, quer erro = %v", err, c.wantErr)
			}
			if !c.wantErr && got.Version != c.want {
				t.Errorf("versão = %q, quer %q", got.Version, c.want)
			}
			gotSkipped := ""
			if skipped != nil {
				gotSkipped = skipped.Version
			}
			if gotSkipped != c.skipped {
				t.Errorf("skippedForRuntime = %q, quer %q", gotSkipped, c.skipped)
			}
		})
	}

	_, _, err := Resolve(idx, "0.9", "1.0.0", "linux/amd64")
	if !errors.Is(err, ErrNoVersion) {
		t.Errorf("err = %v, quer ErrNoVersion", err)
	}
}
