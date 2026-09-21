package httpapi

import "net/http"

// withRequireTLS recusa requisições que não chegaram via TLS, quando required
// é true. server-central nunca termina TLS no próprio binário (ver
// docs/architecture.md, "Decisão: guia de self-hosting... TLS via proxy
// reverso"), então a única forma de saber se o tráfego original era https é o
// header X-Forwarded-Proto setado pelo proxy reverso na frente. Ausência do
// header conta como inseguro — evita o caso de alguém expor a porta
// diretamente, sem proxy nenhum.
//
// required vem de REQUIRE_TLS (padrão desligado, ver docs/architecture.md,
// "Criptografia em trânsito obrigatória"): em dev local (`go run .` direto,
// sem proxy na frente, fluxo já descrito em server-central/.env.example) a
// exigência quebraria o padrão atual; instâncias reais atrás de
// Caddy/Nginx Proxy Manager/Traefik (que já setam X-Forwarded-Proto) devem
// ligar a flag.
//
// /healthz fica isento, mesmo padrão de withRateLimit — o HEALTHCHECK do
// Docker chama http://127.0.0.1:8080/healthz de dentro do próprio container,
// sem passar pelo proxy.
func withRequireTLS(required bool, next http.Handler) http.Handler {
	if !required {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("X-Forwarded-Proto") != "https" {
			http.Error(w, "TLS obrigatório: conecte via https", http.StatusUpgradeRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}
