package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

const maxMessageContentLength = 4000

// newUpgrader monta o upgrader do WebSocket de canal de texto. Subprotocols
// precisa incluir "access_token" (ver internal/auth.wsAuthSubprotocol) para
// o gorilla aceitar e ecoar de volta o subprotocolo que o client usa para
// carregar o token no handshake, já que a API WebSocket do navegador não
// permite setar o header Authorization diretamente.
//
// CheckOrigin reproduz o fallback padrão do gorilla (sem header Origin, ou
// Origin igual ao Host, sempre passa) e adicionalmente aceita qualquer
// origem em allowedOrigins (mesma lista usada pelo CORS REST em cors.go) —
// ver docs/architecture.md, "Decisão: CORS em server-channel".
func newUpgrader(allowedOrigins map[string]bool) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"access_token"},
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			if allowedOrigins[origin] {
				return true
			}
			u, err := url.Parse(origin)
			return err == nil && u.Host == r.Host
		},
	}
}

// GET /api/channels/{id}/ws — WebSocket de um canal de texto. O client
// autenticado envia frames "message.create"; o servidor persiste a
// mensagem via MessageStore e distribui "message.created" via
// realtime.Hub para todos os clients conectados a este canal (broadcast,
// inclusive para o autor, para confirmar id/timestamp atribuídos pelo
// servidor).
func handleChannelWS(hub *realtime.Hub, channels *store.ChannelStore, messages *store.MessageStore, upgrader websocket.Upgrader) http.Handler {
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
		if channel.Type != store.ChannelText {
			http.Error(w, "canal não é de texto", http.StatusBadRequest)
			return
		}

		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// upgrader já escreveu a resposta HTTP de erro.
			return
		}

		client := realtime.NewClient(conn)
		hub.Register(channelID, client)
		defer hub.Unregister(channelID, client)

		go client.WritePump()
		client.ReadPump(func(raw []byte) {
			handleIncomingMessage(r.Context(), hub, messages, channelID, member.ID, client, raw)
		})
	})
}

func handleIncomingMessage(ctx context.Context, hub *realtime.Hub, messages *store.MessageStore, channelID, authorMemberID string, client *realtime.Client, raw []byte) {
	incoming, err := realtime.DecodeIncoming(raw)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	content := strings.TrimSpace(incoming.Content)
	if content == "" {
		client.SendError("conteúdo da mensagem não pode ser vazio")
		return
	}
	if len(content) > maxMessageContentLength {
		client.SendError(fmt.Sprintf("conteúdo excede o limite de %d caracteres", maxMessageContentLength))
		return
	}

	m, err := messages.Create(ctx, channelID, nil, authorMemberID, content)
	if err != nil {
		log.Printf("server-channel: erro ao criar mensagem: %v", err)
		client.SendError("erro ao enviar mensagem")
		return
	}

	payload, err := realtime.EncodeMessageCreated(toMessageView(m))
	if err != nil {
		log.Printf("server-channel: erro ao codificar mensagem criada: %v", err)
		return
	}
	hub.Broadcast(channelID, payload)
}
