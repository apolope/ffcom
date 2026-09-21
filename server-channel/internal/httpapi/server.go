// Package httpapi monta as rotas HTTP de server-channel.
package httpapi

import (
	"net/http"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// NewRouter monta o mux com todas as rotas. O realtime.Hub de canais de
// texto vive pelo tempo de vida do processo, compartilhado entre a rota de
// WebSocket e (futuramente) qualquer outro ponto que precise fazer
// broadcast para clients conectados.
//
// authenticated exige só um Bearer token válido (auth.VerifyToken);
// protected empilha auth.RequireMember em cima, exigindo que o "sub" já
// tenha entrado no servidor via POST /api/join — não existe mais criação
// implícita de membro em toda requisição (ver docs/architecture.md,
// "Convites obrigatórios para entrar em server-channel").
//
// allowedOrigins vem de CORS_ALLOWED_ORIGINS (ver docs/architecture.md,
// "Decisão: CORS em server-channel") — origens do client (web/PWA,
// Electron) autorizadas a chamar esta instância de uma origem diferente.
func NewRouter(verifier *auth.Verifier, db *store.Store, allowedOrigins []string, liveKitAPIKey, liveKitAPISecret, liveKitPublicURL, version string) http.Handler {
	mux := http.NewServeMux()
	hub := realtime.NewHub()

	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}
	upgrader := newUpgrader(allowed)

	mux.HandleFunc("GET /healthz", handleHealthz(version))

	authenticated := auth.VerifyToken(verifier)
	requireMember := auth.RequireMember(db.Members)
	protected := func(h http.Handler) http.Handler {
		return authenticated(requireMember(h))
	}

	mux.Handle("POST /api/join", authenticated(handleJoin(db.Members, db.Invites)))
	mux.Handle("GET /api/me", protected(handleMe(db.Roles)))
	mux.Handle("PATCH /api/me", protected(handleUpdateMe(db.Members, db.Roles)))
	mux.Handle("GET /api/members", protected(handleListMembers(db.Members, db.Roles)))
	mux.Handle("GET /api/categories", protected(handleListCategories(db.Categories, db.Channels, db.Roles, db.ChannelOverwrites)))
	mux.Handle("GET /api/channels", protected(handleListChannels(db.Channels, db.Roles, db.ChannelOverwrites)))
	mux.Handle("GET /api/channels/{id}/messages", protected(handleListMessages(db.Channels, db.Roles, db.ChannelOverwrites, db.Messages)))
	mux.Handle("GET /api/channels/{id}/threads", protected(handleListThreads(db.Channels, db.Roles, db.ChannelOverwrites, db.Messages)))
	mux.Handle("GET /api/threads/{id}/messages", protected(handleListThreadMessages(db.Roles, db.ChannelOverwrites, db.Messages)))
	mux.Handle("GET /api/channels/{id}/ws", protected(handleChannelWS(hub, db.Channels, db.Roles, db.ChannelOverwrites, db.Messages, upgrader)))
	mux.Handle("POST /api/channels/{id}/voice/token", protected(handleVoiceToken(db.Channels, db.Roles, db.ChannelOverwrites, liveKitAPIKey, liveKitAPISecret, liveKitPublicURL)))
	mux.Handle("GET /api/channels/{id}/overwrites", protected(handleListChannelOverwrites(db.Channels, db.Roles, db.ChannelOverwrites)))
	mux.Handle("PUT /api/channels/{id}/overwrites/{roleId}", protected(handleSetChannelOverwrite(db.Channels, db.Roles, db.ChannelOverwrites)))
	mux.Handle("DELETE /api/channels/{id}/overwrites/{roleId}", protected(handleDeleteChannelOverwrite(db.Roles, db.ChannelOverwrites)))
	mux.Handle("POST /api/invites", protected(handleCreateInvite(db.Invites, db.Roles)))
	mux.Handle("GET /api/invites", protected(handleListInvites(db.Invites, db.Roles)))
	mux.Handle("DELETE /api/invites/{id}", protected(handleDeleteInvite(db.Invites, db.Roles)))
	mux.Handle("GET /api/roles", protected(handleListRoles(db.Roles)))
	mux.Handle("POST /api/roles", protected(handleCreateRole(db.Roles)))
	mux.Handle("PATCH /api/roles/{id}", protected(handleUpdateRole(db.Roles)))
	mux.Handle("DELETE /api/roles/{id}", protected(handleDeleteRole(db.Roles)))
	mux.Handle("POST /api/members/{memberId}/roles/{roleId}", protected(handleAssignRole(db.Members, db.Roles)))
	mux.Handle("DELETE /api/members/{memberId}/roles/{roleId}", protected(handleRemoveRole(db.Roles)))

	return withCORS(allowed, mux)
}
