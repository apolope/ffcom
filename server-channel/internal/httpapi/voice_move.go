package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/livekit"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// VoiceMoveTopic é o tópico da mensagem do LiveKit que manda o client trocar
// de sala; o client só obedece quando ela vem do servidor (sem participante
// de origem). Mesmo valor em client/src/hooks/useVoiceChannel.ts.
const VoiceMoveTopic = "ffcom.voice.move"

// voiceDataSender é o que a rota de mover precisa do LiveKit além da
// listagem; interface só para o teste trocar por um fake.
type voiceDataSender interface {
	SendData(ctx context.Context, room string, identities []string, topic string, data []byte) error
}

type moveVoiceRequest struct {
	MemberID  string `json:"memberId"`
	ChannelID string `json:"channelId"`
}

// voiceMoveMessage vai para o client de quem é movido. Leva o token da sala
// de destino pronto (mesmo formato de POST /api/channels/{id}/voice/token):
// quem é puxado não precisa ter Voice no destino, então pedir o token
// sozinho daria 403.
type voiceMoveMessage struct {
	ChannelID   string `json:"channelId"`
	ChannelName string `json:"channelName"`
	Token       string `json:"token"`
	URL         string `json:"url"`
}

// POST /api/voice/move — leva um membro que está numa sala de voz para
// outra. Quem move precisa de MoveMembers na sala de origem e na de destino
// e de Voice no destino (só puxa para onde ele mesmo pode estar). A
// permissão de quem é movido no destino não conta, como no Discord: é o
// jeito de puxar alguém para uma sala fechada. O servidor não mexe na
// conexão: manda pelo LiveKit uma mensagem só para a pessoa, com o token do
// destino, e o client dela sai da sala atual e entra na nova. Ver
// docs/architecture.md, "Decisão: mover membro entre salas de voz".
func handleMoveVoiceParticipant(presence *voicePresence, sender voiceDataSender, members *store.MemberStore, channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, apiKey, apiSecret, publicURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mover, ok := auth.MemberFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
			return
		}

		var req moveVoiceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MemberID == "" || req.ChannelID == "" {
			apierr.Write(w, http.StatusBadRequest, "voice.move_invalid", "memberId e channelId são obrigatórios")
			return
		}

		dest, err := channels.GetByID(r.Context(), req.ChannelID)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "channels.not_found", "canal não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "channels.fetch_failed", "erro ao buscar canal")
			return
		}
		if dest.Type != store.ChannelVoice {
			apierr.Write(w, http.StatusBadRequest, "channels.not_voice", "canal não é de voz")
			return
		}

		canMove := func(channelID string, extra int64) (bool, error) {
			effective, err := channelPermission(r.Context(), roles, overwrites, mover, channelID)
			return permissions.Has(effective, permissions.MoveMembers) && (extra == 0 || permissions.Has(effective, extra)), err
		}
		allowed, err := canMove(dest.ID, permissions.Voice)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
			return
		}
		if !allowed {
			apierr.Write(w, http.StatusForbidden, "voice.move_denied", "sem permissão MoveMembers e Voice no canal de destino")
			return
		}

		target, err := members.GetByID(r.Context(), req.MemberID)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "members.not_found", "membro não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "members.fetch_failed", "erro ao buscar membro")
			return
		}

		// Foto nova, sem o cache de 5s: a pessoa pode ter acabado de entrar
		// ou trocar de sala.
		presence.invalidate()
		snapshot, err := presence.get(r.Context())
		if err != nil {
			log.Printf("server-channel: erro ao consultar salas do LiveKit: %v", err)
			apierr.Write(w, http.StatusBadGateway, "voice.livekit_unavailable", "LiveKit indisponível")
			return
		}
		source := roomOf(snapshot, target.ID)
		if source == "" {
			apierr.Write(w, http.StatusConflict, "voice.move_not_connected", "o membro não está em nenhum canal de voz")
			return
		}
		if source == dest.ID {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		allowed, err = canMove(source, 0)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
			return
		}
		if !allowed {
			apierr.Write(w, http.StatusForbidden, "voice.move_denied", "sem permissão MoveMembers no canal em que o membro está")
			return
		}

		token, err := livekit.NewAccessToken(apiKey, apiSecret, target.ID, target.DisplayName(), dest.ID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "voice.token_failed", "erro ao gerar token de voz")
			return
		}
		payload, _ := json.Marshal(voiceMoveMessage{ChannelID: dest.ID, ChannelName: dest.Name, Token: token, URL: publicURL})
		if err := sender.SendData(r.Context(), source, []string{target.ID}, VoiceMoveTopic, payload); err != nil {
			log.Printf("server-channel: erro ao mandar o aviso de mover para a sala %s: %v", source, err)
			apierr.Write(w, http.StatusBadGateway, "voice.livekit_unavailable", "LiveKit indisponível")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// roomOf devolve a sala em que identity está na foto, ou "" se em nenhuma.
func roomOf(snapshot map[string][]livekit.Participant, identity string) string {
	for room, participants := range snapshot {
		for _, p := range participants {
			if p.Identity == identity {
				return room
			}
		}
	}
	return ""
}
