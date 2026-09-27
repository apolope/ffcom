// Package httpapi monta as rotas HTTP de server-central.
package httpapi

import (
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/realtime"
	"a3sitsolutions.com/ffcom/server-central/internal/storage"
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
//
// ideasCfg configura as sugestões de melhoria da home (ver
// internal/httpapi/ideas.go) e signupCfg os pedidos de cadastro (ver
// internal/httpapi/signups.go).
func NewRouter(verifier *auth.Verifier, db *store.Store, avatarFiles *storage.AvatarStore, avatarMaxBytes int64, allowedOrigins []string, version string, rateLimitRPM, rateLimitBurst int, requireTLS bool, ideasCfg IdeasConfig, signupCfg SignupConfig) http.Handler {
	mux := http.NewServeMux()
	hub := realtime.NewHub()

	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}
	upgrader := newPresenceUpgrader(allowed)

	mux.HandleFunc("GET /healthz", handleHealthz(version))

	protected := auth.Middleware(verifier, db.Accounts)
	mux.Handle("GET /api/me", protected(handleMe(db)))
	mux.Handle("PUT /api/me/e2e-public-key", protected(handleSetE2EPublicKey(db.Accounts)))
	mux.Handle("GET /api/me/e2e-key-backup", protected(handleGetE2EKeyBackup(db.Accounts)))
	mux.Handle("PUT /api/me/e2e-key-backup", protected(handleSetE2EKeyBackup(db.Accounts)))
	mux.Handle("PUT /api/me/status", protected(handleSetPresenceStatus(hub, db.Accounts, db.Friendships)))
	mux.Handle("POST /api/accounts/lookup", protected(handleLookupAccounts(db.Accounts)))
	mux.Handle("POST /api/me/avatar", protected(handleUploadAvatar(db.Profiles, avatarFiles, avatarMaxBytes)))
	mux.Handle("DELETE /api/me/avatar", protected(handleDeleteAvatar(db.Profiles, avatarFiles)))
	mux.Handle("GET /api/avatars/{id}", protected(handleGetAvatar(avatarFiles)))
	mux.Handle("GET /api/servers", protected(handleListServers(db.KnownServers)))
	mux.Handle("POST /api/servers", protected(handleAddServer(db.KnownServers)))
	mux.Handle("PUT /api/servers/order", protected(handleReorderServers(db.KnownServers)))
	mux.Handle("DELETE /api/servers/{id}", protected(handleRemoveServer(db.KnownServers)))
	mux.Handle("GET /api/presence", protected(handlePresenceSnapshot(hub, db.Friendships)))
	mux.Handle("GET /api/presence/ws", protected(handlePresenceWS(hub, db.Friendships, db.DirectMessages, upgrader)))
	mux.Handle("GET /api/friends", protected(handleListFriends(db.Friendships, db.Profiles, db.Accounts, db.DirectMessages)))
	mux.Handle("POST /api/friends/invites", protected(handleCreateFriendInvite(db.FriendInvites)))
	mux.Handle("POST /api/friends/invites/{code}/redeem", protected(handleRedeemFriendInvite(hub, db.FriendInvites, db.Friendships, db.Profiles)))
	mux.Handle("GET /api/friends/requests", protected(handleListFriendRequests(db.Friendships, db.Profiles)))
	mux.Handle("POST /api/friends/requests", protected(handleCreateFriendRequest(hub, db.Friendships, db.Accounts, db.Profiles)))
	mux.Handle("POST /api/friends/requests/{id}/accept", protected(handleAcceptFriendRequest(hub, db.Friendships, db.Profiles)))
	mux.Handle("DELETE /api/friends/requests/{id}", protected(handleDeleteFriendRequest(hub, db.Friendships)))
	mux.Handle("GET /api/dms/{accountId}/messages", protected(handleListDMs(db.Friendships, db.DirectMessages)))

	optional := auth.OptionalAccount(verifier, db.Accounts)
	mux.Handle("GET /api/ideas", optional(handleListIdeas(db.Ideas, ideasCfg)))
	mux.Handle("GET /api/ideas/me", protected(handleIdeasMe(db, ideasCfg)))
	mux.Handle("POST /api/ideas", protected(handleCreateIdea(db, ideasCfg)))
	mux.Handle("POST /api/ideas/assist", protected(handleCreateAssist(db, ideasCfg)))
	mux.Handle("GET /api/ideas/assist/{id}", protected(handleGetAssist(db, ideasCfg)))
	mux.Handle("PUT /api/ideas/{id}/vote", protected(handleVoteIdea(db, ideasCfg)))
	mux.Handle("PATCH /api/ideas/{id}", protected(handleModerateIdea(db, ideasCfg)))
	mux.Handle("DELETE /api/ideas/{id}", protected(handleDeleteIdea(db, ideasCfg)))

	// Público: quem pede cadastro ainda não tem conta.
	mux.Handle("POST /api/signup-requests", handleCreateSignup(db, signupCfg))

	limiter := newRateLimiter(rateLimitRPM, rateLimitBurst)
	identify := func(r *http.Request) (*http.Request, string, string, bool) { return auth.IdentifyRequest(verifier, r) }
	return withRequireTLS(requireTLS, withCORS(allowed, withRateLimit(limiter, identify, mux)))
}
