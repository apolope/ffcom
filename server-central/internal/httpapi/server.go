// Package httpapi monta as rotas HTTP de server-central.
package httpapi

import (
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// NewRouter monta o mux com todas as rotas protegidas por auth.Middleware.
//
// allowedOrigins vem de CORS_ALLOWED_ORIGINS (mesmo padrão de
// server-channel, ver docs/architecture.md, "Decisão: CORS em
// server-channel") — origens do client (web/PWA, Electron) autorizadas a
// chamar esta instância de uma origem diferente.
func NewRouter(verifier *auth.Verifier, db *store.Store, allowedOrigins []string) http.Handler {
	mux := http.NewServeMux()

	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}

	protected := auth.Middleware(verifier, db.Accounts)
	mux.Handle("GET /api/me", protected(handleMe(db)))
	mux.Handle("GET /api/servers", protected(handleListServers(db.KnownServers)))
	mux.Handle("POST /api/servers", protected(handleAddServer(db.KnownServers)))
	mux.Handle("DELETE /api/servers/{id}", protected(handleRemoveServer(db.KnownServers)))

	return withCORS(allowed, mux)
}
