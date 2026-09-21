// Package httpapi monta as rotas HTTP de server-central.
package httpapi

import (
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/realtime"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// NewRouter monta o mux com todas as rotas protegidas por auth.Middleware.
// O realtime.Hub de presença vive pelo tempo de vida do processo,
// compartilhado entre a rota de WebSocket e o snapshot REST (ver
// internal/httpapi/presence.go).
//
// allowedOrigins vem de CORS_ALLOWED_ORIGINS (mesmo padrão de
// server-channel, ver docs/architecture.md, "Decisão: CORS em
// server-channel") — origens do client (web/PWA, Electron) autorizadas a
// chamar esta instância de uma origem diferente.
func NewRouter(verifier *auth.Verifier, db *store.Store, allowedOrigins []string) http.Handler {
	mux := http.NewServeMux()
	hub := realtime.NewHub()

	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}
	upgrader := newPresenceUpgrader(allowed)

	mux.HandleFunc("GET /healthz", handleHealthz)

	protected := auth.Middleware(verifier, db.Accounts)
	mux.Handle("GET /api/me", protected(handleMe(db)))
	mux.Handle("GET /api/servers", protected(handleListServers(db.KnownServers)))
	mux.Handle("POST /api/servers", protected(handleAddServer(db.KnownServers)))
	mux.Handle("DELETE /api/servers/{id}", protected(handleRemoveServer(db.KnownServers)))
	mux.Handle("GET /api/presence", protected(handlePresenceSnapshot(hub, db.Friendships)))
	mux.Handle("GET /api/presence/ws", protected(handlePresenceWS(hub, db.Friendships, db.DirectMessages, upgrader)))
	mux.Handle("GET /api/friends", protected(handleListFriends(db.Friendships, db.Profiles)))
	mux.Handle("POST /api/friends/invites", protected(handleCreateFriendInvite(db.FriendInvites)))
	mux.Handle("POST /api/friends/invites/{code}/redeem", protected(handleRedeemFriendInvite(db.FriendInvites, db.Friendships)))
	mux.Handle("GET /api/dms/{accountId}/messages", protected(handleListDMs(db.Friendships, db.DirectMessages)))

	return withCORS(allowed, mux)
}
