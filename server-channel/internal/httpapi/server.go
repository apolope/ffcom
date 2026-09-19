// Package httpapi monta as rotas HTTP de server-channel.
package httpapi

import (
	"net/http"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// NewRouter monta o mux com todas as rotas protegidas por auth.Middleware.
func NewRouter(verifier *auth.Verifier, db *store.Store) http.Handler {
	mux := http.NewServeMux()

	protected := auth.Middleware(verifier, db.Members)
	mux.Handle("GET /api/me", protected(handleMe()))

	return mux
}
