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
// usuário (IP só sem token válido) sobre toda a API REST (mesmo padrão de
// server-central), e um por membro
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

// requestIdentifier descobre de quem é a requisição (auth.IdentifyRequest
// na montagem real; os testes passam um falso): o "sub" e o "sid" (sessão do
// dispositivo, pode vir vazio). Devolve a requisição com os dois já no
// contexto, para a autenticação não verificar o token de novo.
type requestIdentifier func(r *http.Request) (*http.Request, string, string, bool)

// withRateLimit aplica o limiter a todas as rotas, exceto /healthz (usado
// pelo HEALTHCHECK do Docker e por monitoramento externo, não deve competir
// por orçamento de requisições com tráfego de cliente real).
//
// A chave é o par usuário+dispositivo ("sub" e "sid" do token verificado),
// não o IP: várias pessoas atrás do mesmo NAT (escritório, casa, CGNAT de
// operadora) saem com o mesmo IP e dividiriam um orçamento só, e a mesma
// pessoa com celular, PC e notebook abertos não deve esgotar o orçamento de
// um aparelho com o uso do outro. O "sid" vem assinado no token (não é header
// que o client escolhe), então não dá para inventar dispositivos à vontade;
// token sem "sid" cai no balde só do usuário. Requisição sem token ou com
// token inválido não tem dono conhecido e cai no bucket do IP, o que
// continua segurando flood anônimo. Ver docs/rate-limits.md.
func withRateLimit(limiter *rateLimiter, identify requestIdentifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		key := "ip:" + clientIP(r)
		if identified, subject, session, ok := identify(r); ok {
			r = identified
			key = userDeviceKey(subject, session)
		}

		if !limiter.allow(key) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "muitas requisições, tente novamente em instantes", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// userDeviceKey monta a chave de bucket de um usuário autenticado: um balde
// por dispositivo quando o token traz "sid", um balde só do usuário quando
// não traz.
func userDeviceKey(subject, session string) string {
	if session == "" {
		return "sub:" + subject
	}
	return "sub:" + subject + "|sid:" + session
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
