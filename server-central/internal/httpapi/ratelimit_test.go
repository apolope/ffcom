package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToBurstThenBlocks(t *testing.T) {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  1,
		burst:          3,
		idleExpiration: time.Minute,
	}

	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("requisição %d deveria ser permitida (dentro do burst)", i+1)
		}
	}
	if l.allow("1.2.3.4") {
		t.Fatal("4ª requisição deveria ser bloqueada (burst esgotado)")
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  1,
		burst:          1,
		idleExpiration: time.Minute,
	}

	if !l.allow("1.2.3.4") {
		t.Fatal("primeira requisição deveria ser permitida")
	}
	if l.allow("1.2.3.4") {
		t.Fatal("segunda requisição imediata deveria ser bloqueada")
	}

	// Simula 1s de espera recuando lastSeen, em vez de dormir de verdade.
	l.buckets["1.2.3.4"].lastSeen = time.Now().Add(-1 * time.Second)

	if !l.allow("1.2.3.4") {
		t.Fatal("requisição após 1s deveria ser permitida (bucket recarregado)")
	}
}

func TestRateLimiterTracksKeysIndependently(t *testing.T) {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  1,
		burst:          1,
		idleExpiration: time.Minute,
	}

	if !l.allow("1.2.3.4") {
		t.Fatal("primeiro IP deveria ser permitido")
	}
	if !l.allow("5.6.7.8") {
		t.Fatal("segundo IP não deveria ser afetado pelo bucket do primeiro")
	}
}

// identifyByHeader faz o papel de auth.IdentifyRequest sem token OIDC real:
// o "sub" vem do header X-Test-Subject.
func identifyByHeader(r *http.Request) (*http.Request, string, bool) {
	subject := r.Header.Get("X-Test-Subject")
	return r, subject, subject != ""
}

func rateLimitedStatus(h http.Handler, subject string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.RemoteAddr = "10.0.0.1:5000"
	if subject != "" {
		req.Header.Set("X-Test-Subject", subject)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestWithRateLimitKeysByUserNotSharedIP(t *testing.T) {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  0,
		burst:          1,
		idleExpiration: time.Minute,
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := withRateLimit(l, identifyByHeader, ok)

	if got := rateLimitedStatus(h, "alice"); got != http.StatusOK {
		t.Fatalf("alice: status %d, esperado 200", got)
	}
	if got := rateLimitedStatus(h, "alice"); got != http.StatusTooManyRequests {
		t.Fatalf("alice de novo: status %d, esperado 429", got)
	}
	// Mesmo IP, outro usuário: não pode herdar o bucket esgotado de alice.
	if got := rateLimitedStatus(h, "bob"); got != http.StatusOK {
		t.Fatalf("bob no mesmo IP: status %d, esperado 200", got)
	}
}

func TestWithRateLimitFallsBackToIPWithoutToken(t *testing.T) {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  0,
		burst:          1,
		idleExpiration: time.Minute,
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := withRateLimit(l, identifyByHeader, ok)

	if got := rateLimitedStatus(h, ""); got != http.StatusOK {
		t.Fatalf("anônimo: status %d, esperado 200", got)
	}
	if got := rateLimitedStatus(h, ""); got != http.StatusTooManyRequests {
		t.Fatalf("anônimo de novo no mesmo IP: status %d, esperado 429", got)
	}
	// Usuário autenticado no mesmo IP não é afetado pelo flood anônimo.
	if got := rateLimitedStatus(h, "alice"); got != http.StatusOK {
		t.Fatalf("alice no mesmo IP: status %d, esperado 200", got)
	}
}
