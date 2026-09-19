// Package httpapi monta as rotas HTTP de server-channel.
package httpapi

import (
	"net/http"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// NewRouter monta o mux com todas as rotas protegidas por auth.Middleware.
// O realtime.Hub de canais de texto vive pelo tempo de vida do processo,
// compartilhado entre a rota de WebSocket e (futuramente) qualquer outro
// ponto que precise fazer broadcast para clients conectados.
func NewRouter(verifier *auth.Verifier, db *store.Store) http.Handler {
	mux := http.NewServeMux()
	hub := realtime.NewHub()

	protected := auth.Middleware(verifier, db.Members)
	mux.Handle("GET /api/me", protected(handleMe()))
	mux.Handle("GET /api/channels/{id}/messages", protected(handleListMessages(db.Channels, db.Messages)))
	mux.Handle("GET /api/channels/{id}/ws", protected(handleChannelWS(hub, db.Channels, db.Messages)))

	return mux
}
