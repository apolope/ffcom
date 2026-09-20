package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/livekit"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// POST /api/channels/{id}/voice/token — emite um access token de LiveKit
// para o membro autenticado entrar na sala do canal de voz identificado por
// id. Não há chamada de rede ao LiveKit aqui (só assinatura local do JWT,
// ver internal/livekit/token.go): a sala é criada implicitamente pelo
// próprio LiveKit no primeiro participante que entrar com um token válido
// para aquele nome de sala — ver docs/architecture.md, "Decisão: integração
// de voz com LiveKit".
func handleVoiceToken(channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, apiKey, apiSecret, publicURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID := r.PathValue("id")

		channel, err := channels.GetByID(r.Context(), channelID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "canal não encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao buscar canal", http.StatusInternalServerError)
			return
		}
		if channel.Type != store.ChannelVoice {
			http.Error(w, "canal não é de voz", http.StatusBadRequest)
			return
		}

		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não autenticado", http.StatusUnauthorized)
			return
		}

		// Único nível de acesso hoje é o bit Voice (join + publicar áudio
		// juntos) — ver docs/architecture.md, "Sistema de permissões/roles
		// por servidor e por canal".
		effective, err := channelPermission(r.Context(), roles, overwrites, member, channelID)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}
		if !permissions.Has(effective, permissions.Voice) {
			http.Error(w, "sem permissão para entrar neste canal de voz", http.StatusForbidden)
			return
		}

		displayName := member.ID
		if member.Nickname != nil {
			displayName = *member.Nickname
		}

		token, err := livekit.NewAccessToken(apiKey, apiSecret, member.ID, displayName, channel.ID)
		if err != nil {
			http.Error(w, "erro ao gerar token de voz", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(voiceTokenResponse{Token: token, RoomName: channel.ID, URL: publicURL})
	})
}

// URL é o endereço público (ws:// ou wss://) do LiveKit desta instância de
// server-channel (LIVEKIT_PUBLIC_URL), para o client saber a quem se
// conectar — pode ser diferente do endereço do próprio server-channel (ex.
// subdomínio/porta dedicados), então não dá pra derivar de baseUrl no
// client (ver docs/architecture.md).
type voiceTokenResponse struct {
	Token    string `json:"token"`
	RoomName string `json:"roomName"`
	URL      string `json:"url"`
}
