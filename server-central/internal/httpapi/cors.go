package httpapi

import "net/http"

// withCORS libera Access-Control-Allow-Origin para as origens em allowed
// (configuradas via CORS_ALLOWED_ORIGINS, mesmo padrão de server-channel —
// ver docs/architecture.md, "Decisão: CORS em server-channel"). Sem isso o
// fetch() do client para outra origem (ex. web/PWA em localhost:5173
// chamando server-central em localhost:8081) falha no navegador mesmo com o
// backend respondendo 200.
//
// Access-Control-Allow-Methods lista todo método HTTP usado por qualquer rota
// do mux (não só GET/POST) de propósito: um preflight cross-origin falha se o
// método da requisição real não estiver aqui, mesmo com a origem liberada e o
// backend respondendo 200 (foi o caso de PATCH /api/me em server-channel
// antes desta lista incluir PATCH — mesmo mecanismo aqui). Ao adicionar uma
// rota com um método novo, atualizar aqui.
func withCORS(allowed map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
