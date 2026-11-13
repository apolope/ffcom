package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Testes do boot tolerante (ver NewVerifier): issuers falsos em httptest,
// com discovery e JWKS, e tokens RS256 assinados aqui mesmo.

// fakeIssuer é um Authentik de mentira: serve o discovery e o JWKS de uma
// chave RSA, ou 503 em tudo enquanto down for true.
type fakeIssuer struct {
	srv  *httptest.Server
	key  *rsa.PrivateKey
	down atomic.Bool
}

func newFakeIssuer(t *testing.T, down bool) *fakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIssuer{key: key}
	f.down.Store(down)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.down.Load() {
			http.Error(w, "fora do ar", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/o/ffcom/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                f.issuer(),
				"jwks_uri":                              f.srv.URL + "/jwks",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": "teste",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// issuer termina em "/", como os issuers do Authentik (…/o/ffcom/).
func (f *fakeIssuer) issuer() string { return f.srv.URL + "/o/ffcom/" }

// token assina um JWT RS256 com iss do issuer falso (ou iss, se não vazio).
func (f *fakeIssuer) token(t *testing.T, iss string) string {
	t.Helper()
	if iss == "" {
		iss = f.issuer()
	}
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signingInput := enc(map[string]string{"alg": "RS256", "typ": "JWT", "kid": "teste"}) + "." +
		enc(map[string]any{"iss": iss, "sub": "sub-teste", "sid": "sid-teste", "exp": time.Now().Add(time.Hour).Unix()})
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// fastBackoff faz os testes tentarem de novo quase na hora.
func fastBackoff(int) time.Duration { return 5 * time.Millisecond }

func newTestVerifier(t *testing.T, issuers ...string) *Verifier {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	v, err := newVerifier(ctx, issuers, fastBackoff)
	if err != nil {
		t.Fatalf("newVerifier: %v", err)
	}
	return v
}

// waitVerify repete Verify até não dar ErrIssuerUnavailable (o discovery
// roda em segundo plano) ou estourar o prazo.
func waitVerify(t *testing.T, v *Verifier, token string) (Claims, error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		claims, err := v.Verify(context.Background(), token)
		if !errors.Is(err, ErrIssuerUnavailable) || time.Now().After(deadline) {
			return claims, err
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNewVerifierRejectsInvalidConfig(t *testing.T) {
	for _, issuers := range [][]string{nil, {"auth.ffcom/o/ffcom/"}, {"https:///sem-host"}, {"ftp://x/o/ffcom/"}} {
		if _, err := newVerifier(context.Background(), issuers, fastBackoff); err == nil {
			t.Errorf("newVerifier(%q) deveria falhar", issuers)
		}
	}
}

// (a) Issuer fora no boot: NewVerifier não falha, o token recebe
// ErrIssuerUnavailable e, quando o issuer volta, valida.
func TestIssuerDownThenUp(t *testing.T) {
	issuer := newFakeIssuer(t, true)
	v := newTestVerifier(t, issuer.issuer())
	token := issuer.token(t, "")

	// Dá tempo para algumas tentativas falharem.
	time.Sleep(30 * time.Millisecond)
	if _, err := v.Verify(context.Background(), token); !errors.Is(err, ErrIssuerUnavailable) {
		t.Fatalf("com o issuer fora, Verify = %v, quer ErrIssuerUnavailable", err)
	}

	issuer.down.Store(false)
	claims, err := waitVerify(t, v, token)
	if err != nil {
		t.Fatalf("depois de o issuer voltar, Verify = %v", err)
	}
	if claims.Subject != "sub-teste" || claims.SessionID != "sid-teste" {
		t.Fatalf("claims = %+v", claims)
	}
}

// (b) Dois issuers, um fora: o que está no ar valida normalmente e o outro
// continua indisponível.
func TestOneIssuerDownDoesNotAffectOther(t *testing.T) {
	up := newFakeIssuer(t, false)
	down := newFakeIssuer(t, true)
	v := newTestVerifier(t, down.issuer(), up.issuer())

	if _, err := waitVerify(t, v, up.token(t, "")); err != nil {
		t.Fatalf("issuer no ar: Verify = %v", err)
	}
	if _, err := v.Verify(context.Background(), down.token(t, "")); !errors.Is(err, ErrIssuerUnavailable) {
		t.Fatalf("issuer fora: Verify = %v, quer ErrIssuerUnavailable", err)
	}
}

// O cancelamento do ctx encerra as tentativas: o issuer nunca fica pronto,
// mesmo voltando depois.
func TestDiscoveryStopsOnCancel(t *testing.T) {
	issuer := newFakeIssuer(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	v, err := newVerifier(ctx, []string{issuer.issuer()}, fastBackoff)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)
	issuer.down.Store(false)
	time.Sleep(50 * time.Millisecond)
	if _, err := v.Verify(context.Background(), issuer.token(t, "")); !errors.Is(err, ErrIssuerUnavailable) {
		t.Fatalf("depois do cancelamento, Verify = %v, quer ErrIssuerUnavailable", err)
	}
}

// (c) O middleware responde 503 com Retry-After, não 401, enquanto o issuer
// do token está fora; o mesmo vale para OptionalAccount.
func TestMiddlewareIssuerUnavailable(t *testing.T) {
	issuer := newFakeIssuer(t, true)
	v := newTestVerifier(t, issuer.issuer())
	token := issuer.token(t, "")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("o handler não deveria rodar")
	})

	for name, h := range map[string]http.Handler{
		"Middleware":      Middleware(v, nil)(next),
		"OptionalAccount": OptionalAccount(v, nil)(next),
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, quer 503", name, rec.Code)
		}
		if got := rec.Header().Get("Retry-After"); got != "5" {
			t.Errorf("%s: Retry-After = %q, quer 5", name, got)
		}
		var body struct{ Code string }
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body.Code != "auth.issuer_unavailable" {
			t.Errorf("%s: code = %q", name, body.Code)
		}
	}

	// O handshake de WebSocket (token no Sec-WebSocket-Protocol) passa
	// pelo mesmo Middleware.
	req := httptest.NewRequest(http.MethodGet, "/api/presence/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", wsAuthSubprotocol+", "+token)
	rec := httptest.NewRecorder()
	Middleware(v, nil)(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "5" {
		t.Errorf("handshake WS: status = %d, Retry-After = %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

// (d) Token de issuer que não está na lista continua 401, mesmo com o
// issuer configurado fora do ar.
func TestMiddlewareUnknownIssuerStill401(t *testing.T) {
	configured := newFakeIssuer(t, true)
	other := newFakeIssuer(t, false)
	v := newTestVerifier(t, configured.issuer())
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("o handler não deveria rodar")
	})

	for _, token := range []string{other.token(t, ""), "nao-e-jwt"} {
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		Middleware(v, nil)(next).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("token %.20q…: status = %d, quer 401", token, rec.Code)
		}
		if rec.Header().Get("Retry-After") != "" {
			t.Errorf("401 não deveria ter Retry-After")
		}
	}
}

// Token de issuer pronto mas com assinatura de outra chave: 401, não 503.
func TestBadSignatureStill401(t *testing.T) {
	issuer := newFakeIssuer(t, false)
	forger := newFakeIssuer(t, false)
	v := newTestVerifier(t, issuer.issuer())
	if _, err := waitVerify(t, v, issuer.token(t, "")); err != nil {
		t.Fatalf("token bom: %v", err)
	}
	_, err := v.Verify(context.Background(), forger.token(t, issuer.issuer()))
	if err == nil || errors.Is(err, ErrIssuerUnavailable) {
		t.Fatalf("assinatura errada: Verify = %v, quer erro comum", err)
	}
}
