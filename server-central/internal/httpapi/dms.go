package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/realtime"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

const (
	maxDMContentLength = 4000
	defaultDMLimit     = 50
	maxDMLimit         = 200
)

// handleIncomingDM processa um frame "dm.create" recebido no WebSocket de
// presença da conta senderID (ver internal/httpapi/presence.go). Só amigos
// aceitos podem trocar DMs (ver docs/architecture.md, "Decisão: DMs
// restritas a amigos aceitos"). A mensagem é sempre persistida antes de
// entregue; a entrega em tempo real (via realtime.Hub.SendTo) é
// best-effort para quem estiver online agora — o destinatário offline lê a
// mensagem depois via GET /api/dms/{accountId}/messages.
func handleIncomingDM(ctx context.Context, hub *realtime.Hub, friendships *store.FriendshipStore, directMessages *store.DirectMessageStore, senderID string, client *realtime.Client, raw []byte) {
	incoming, err := realtime.DecodeIncomingDM(raw)
	if err != nil {
		client.SendError(err.Error())
		return
	}

	if incoming.RecipientID == senderID {
		client.SendError("não é possível enviar uma DM para si mesmo")
		return
	}

	content := strings.TrimSpace(incoming.Content)
	if content == "" {
		client.SendError("conteúdo da mensagem não pode ser vazio")
		return
	}
	if len(content) > maxDMContentLength {
		client.SendError("conteúdo excede o limite de caracteres")
		return
	}

	areFriends, err := friendships.AreFriends(ctx, senderID, incoming.RecipientID)
	if err != nil {
		log.Printf("server-central: erro ao verificar amizade para DM: %v", err)
		client.SendError("erro ao enviar mensagem")
		return
	}
	if !areFriends {
		client.SendError("só é possível enviar DMs para amigos")
		return
	}

	m, err := directMessages.Create(ctx, senderID, incoming.RecipientID, content)
	if err != nil {
		log.Printf("server-central: erro ao criar DM: %v", err)
		client.SendError("erro ao enviar mensagem")
		return
	}

	payload, err := realtime.EncodeDMCreated(toDMView(m))
	if err != nil {
		log.Printf("server-central: erro ao codificar dm.created: %v", err)
		return
	}
	// Entrega para o destinatário (se online) e ecoa para o próprio
	// remetente, para confirmar id/timestamp atribuídos pelo servidor —
	// mesmo padrão de "message.created" em server-channel.
	hub.SendTo(incoming.RecipientID, payload)
	hub.SendTo(senderID, payload)
}

// GET /api/dms/{accountId}/messages?before=RFC3339&limit=N — histórico
// paginado da conversa entre a conta autenticada e {accountId} (keyset
// pagination por created_at, mais recentes primeiro). Exige amizade aceita
// entre as duas contas, mesma regra do envio via WebSocket.
func handleListDMs(friendships *store.FriendshipStore, directMessages *store.DirectMessageStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		otherAccountID := r.PathValue("accountId")

		areFriends, err := friendships.AreFriends(r.Context(), account.ID, otherAccountID)
		if err != nil {
			http.Error(w, "erro ao verificar amizade", http.StatusInternalServerError)
			return
		}
		if !areFriends {
			http.Error(w, "só é possível ver DMs de amigos", http.StatusForbidden)
			return
		}

		limit := defaultDMLimit
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n <= 0 {
				http.Error(w, "limit inválido", http.StatusBadRequest)
				return
			}
			limit = min(n, maxDMLimit)
		}

		var before *time.Time
		if raw := r.URL.Query().Get("before"); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				http.Error(w, "before inválido: use RFC3339", http.StatusBadRequest)
				return
			}
			before = &t
		}

		rows, err := directMessages.ListConversation(r.Context(), account.ID, otherAccountID, before, limit)
		if err != nil {
			http.Error(w, "erro ao buscar histórico", http.StatusInternalServerError)
			return
		}

		out := make([]realtime.DirectMessageView, len(rows))
		for i, m := range rows {
			out[i] = toDMView(m)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listDMsResponse{Messages: out})
	})
}

type listDMsResponse struct {
	Messages []realtime.DirectMessageView `json:"messages"`
}

func toDMView(m store.DirectMessage) realtime.DirectMessageView {
	return realtime.DirectMessageView{
		ID:          m.ID,
		SenderID:    m.SenderID,
		RecipientID: m.RecipientID,
		Content:     m.Content,
		CreatedAt:   m.CreatedAt,
		EditedAt:    m.EditedAt,
	}
}
