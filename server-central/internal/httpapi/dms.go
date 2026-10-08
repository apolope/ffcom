package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/push"
	"a3sitsolutions.com/ffcom/server-central/internal/realtime"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

const (
	dmNonceLength = 24
	// maxDMCiphertextLength é um limite de tamanho bruto, não de conteúdo --
	// o servidor não consegue mais validar "vazio" ou contar caracteres
	// depois que o payload virou ciphertext opaco (criptografia
	// ponta-a-ponta, ver docs/architecture.md). Generoso o bastante para o
	// overhead de 16 bytes do NaCl box sobre uma mensagem de texto razoável.
	maxDMCiphertextLength = 8192
	defaultDMLimit        = 50
	maxDMLimit            = 200
)

// handleIncomingDM processa um frame "dm.create" recebido no WebSocket de
// presença da conta senderID (ver internal/httpapi/presence.go). Só amigos
// aceitos podem trocar DMs (ver docs/architecture.md, "Decisão: DMs
// restritas a amigos aceitos"). A mensagem é sempre persistida antes de
// entregue; a entrega em tempo real (via realtime.Hub.SendTo) é
// best-effort para quem estiver online agora — o destinatário offline lê a
// mensagem depois via GET /api/dms/{accountId}/messages.
func handleIncomingDM(ctx context.Context, hub *realtime.Hub, friendships *store.FriendshipStore, directMessages *store.DirectMessageStore, profiles *store.ProfileStore, dispatcher *push.Dispatcher, senderID string, client *realtime.Client, raw []byte) {
	incoming, prob := realtime.DecodeIncomingDM(raw)
	if prob != nil {
		client.SendError(prob)
		return
	}

	if incoming.RecipientID == senderID {
		client.SendError(apierr.New("dm.send_self", "não é possível enviar uma DM para si mesmo"))
		return
	}

	if len(incoming.Nonce) != dmNonceLength {
		client.SendError(apierr.New("dm.nonce_invalid", "nonce inválido"))
		return
	}
	if len(incoming.Ciphertext) == 0 || len(incoming.Ciphertext) > maxDMCiphertextLength {
		client.SendError(apierr.New("dm.ciphertext_invalid", "ciphertext inválido"))
		return
	}

	areFriends, err := friendships.AreFriends(ctx, senderID, incoming.RecipientID)
	if err != nil {
		log.Printf("server-central: erro ao verificar amizade para DM: %v", err)
		client.SendError(apierr.New("dm.send_failed", "erro ao enviar mensagem"))
		return
	}
	if !areFriends {
		client.SendError(apierr.New("dm.send_friends_only", "só é possível enviar DMs para amigos"))
		return
	}

	m, err := directMessages.Create(ctx, senderID, incoming.RecipientID, incoming.Ciphertext, incoming.Nonce)
	if err != nil {
		log.Printf("server-central: erro ao criar DM: %v", err)
		client.SendError(apierr.New("dm.send_failed", "erro ao enviar mensagem"))
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
	// Push para os aparelhos do destinatário: só quem mandou, porque o
	// conteúdo é cifrado de ponta a ponta e o servidor não o conhece.
	pushFromAccount(dispatcher, profiles, push.TypeDM, incoming.RecipientID, senderID, "")
}

// GET /api/dms/{accountId}/messages?before=RFC3339&limit=N — histórico
// paginado da conversa entre a conta autenticada e {accountId} (keyset
// pagination por created_at, mais recentes primeiro). Exige amizade aceita
// entre as duas contas, mesma regra do envio via WebSocket.
func handleListDMs(friendships *store.FriendshipStore, directMessages *store.DirectMessageStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}

		otherAccountID := r.PathValue("accountId")

		areFriends, err := friendships.AreFriends(r.Context(), account.ID, otherAccountID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "friends.check_failed", "erro ao verificar amizade")
			return
		}
		if !areFriends {
			apierr.Write(w, http.StatusForbidden, "dm.friends_only", "só é possível ver DMs de amigos")
			return
		}

		limit := defaultDMLimit
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n <= 0 {
				apierr.Write(w, http.StatusBadRequest, "common.limit_invalid", "limit inválido")
				return
			}
			limit = min(n, maxDMLimit)
		}

		var before *time.Time
		if raw := r.URL.Query().Get("before"); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				apierr.Write(w, http.StatusBadRequest, "common.before_invalid", "before inválido: use RFC3339")
				return
			}
			before = &t
		}

		rows, err := directMessages.ListConversation(r.Context(), account.ID, otherAccountID, before, limit)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "dm.history_fetch_failed", "erro ao buscar histórico")
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
		Ciphertext:  m.Ciphertext,
		Nonce:       m.Nonce,
		CreatedAt:   m.CreatedAt,
		EditedAt:    m.EditedAt,
	}
}
