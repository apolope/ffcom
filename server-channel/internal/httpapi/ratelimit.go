package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter é um token bucket por chave, em memória — sem dependência
// externa (golang.org/x/time/rate ou Redis), mesma implementação usada em
// server-central/internal/httpapi/ratelimit.go (módulos separados, sem
// pacote compartilhado entre os dois — mesmo padrão de duplicação já
// existente em requiretls.go/cors.go).
//
// server-channel usa dois limiters com chaves diferentes (ver
// docs/architecture.md, "Decisão: rate limiting em server-channel"): um por
// IP sobre toda a API REST (mesmo padrão de server-central), e um por membro
// sobre frames recebidos numa conexão WebSocket já aberta — um limite só de
// conexão/IP não protege contra spam de mensagens dentro de uma conexão que
// já passou pelo handshake.
type rateLimiter struct {
	mu             sync.Mutex
	buckets        map[string]*bucket
	ratePerSecond  float64
	burst          float64
	idleExpiration time.Duration
}

type bucket struct {
	tokens   float64
	lastSeen time.Time
}

// newRateLimiter cria um limiter com "burst" tokens por chave, recarregando
// à taxa de requestsPerMinute/60 tokens por segundo.
func newRateLimiter(requestsPerMinute, burst int) *rateLimiter {
	l := &rateLimiter{
		buckets:        make(map[string]*bucket),
		ratePerSecond:  float64(requestsPerMinute) / 60.0,
		burst:          float64(burst),
		idleExpiration: 10 * time.Minute,
	}
	go l.cleanupLoop()
	return l
}

// cleanupLoop remove buckets ociosos periodicamente, para não crescer sem
// limite com chaves (IPs ou membros) que passaram e não voltam.
func (l *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-l.idleExpiration)
		l.mu.Lock()
		for key, b := range l.buckets {
			if b.lastSeen.Before(cutoff) {
				delete(l.buckets, key)
			}
		}
		l.mu.Unlock()
	}
}

// allow consome um token da chave, recarregando a bucket proporcionalmente
// ao tempo decorrido desde a última chamada. Devolve false se não há token
// disponível (requisição ou frame deve ser rejeitado).
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, lastSeen: now}
		l.buckets[key] = b
	} else {
		elapsed := now.Sub(b.lastSeen).Seconds()
		b.tokens += elapsed * l.ratePerSecond
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastSeen = now
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// withRateLimit aplica o limiter (por IP) a todas as rotas REST, exceto
// /healthz (usado pelo HEALTHCHECK do Docker e por monitoramento externo —
// não deve competir por orçamento de requisições com tráfego de cliente
// real). O upgrade de WebSocket em GET /api/channels/{id}/ws também passa
// por aqui (é só mais uma requisição HTTP do ponto de vista deste
// middleware); o limite de mensagens dentro da conexão já aberta é outro
// limiter, aplicado em handleChannelWS.
func withRateLimit(limiter *rateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		if !limiter.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "muitas requisições, tente novamente em instantes", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// clientIP extrai o IP do cliente. server-channel roda atrás de um proxy
// reverso na implantação de referência (ver docs/architecture.md, "Decisão:
// primeira implantação de teste"), então X-Forwarded-For (primeiro IP da
// lista, o mais próximo do cliente original) tem prioridade sobre
// RemoteAddr; sem esse header (dev local, chamada direta), cai para
// RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
