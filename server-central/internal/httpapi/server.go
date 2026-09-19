// Package httpapi monta as rotas HTTP de server-central.
package httpapi

import (
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// NewRouter monta o mux com todas as rotas protegidas por auth.Middleware.
func NewRouter(verifier *auth.Verifier, db *store.Store) http.Handler {
	mux := http.NewServeMux()

	protected := auth.Middleware(verifier, db.Accounts)
	mux.Handle("GET /api/me", protected(handleMe(db)))

	return mux
}
