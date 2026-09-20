package httpapi

import "net/http"

// withCORS libera Access-Control-Allow-Origin para as origens em allowed
// (configuradas via CORS_ALLOWED_ORIGINS, ver docs/architecture.md — "vai
// bloquear o client... nenhuma rota de server-channel libera CORS/Origin
// cruzado hoje"). Sem isso o fetch() do client para outra origem (ex. web/
// PWA em localhost:5173 chamando server-channel em localhost:8080) falha no
// navegador mesmo com o backend respondendo 200.
func withCORS(allowed map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
