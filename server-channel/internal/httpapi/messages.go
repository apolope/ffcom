package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 200
)

// GET /api/channels/{id}/messages?before=RFC3339&limit=N — histórico
// paginado de um canal de texto (keyset pagination por created_at, mais
// recentes primeiro). Ver docs/architecture.md, "Decisão: protocolo entre
// client, server-central e server-channel": histórico é REST, tempo real é
// WebSocket.
func handleListMessages(channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore) http.Handler {
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
		effective, err := channelPermission(r.Context(), roles, overwrites, member, channelID)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}
		if !permissions.Has(effective, permissions.ViewChannels) {
			http.Error(w, "sem permissão para ver este canal", http.StatusForbidden)
			return
		}

		limit := defaultHistoryLimit
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n <= 0 {
				http.Error(w, "limit inválido", http.StatusBadRequest)
				return
			}
			limit = min(n, maxHistoryLimit)
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

		rows, err := messages.ListForChannel(r.Context(), channelID, nil, before, limit)
		if err != nil {
			http.Error(w, "erro ao buscar histórico", http.StatusInternalServerError)
			return
		}

		out := make([]realtime.MessageView, len(rows))
		for i, m := range rows {
			out[i] = toMessageView(m)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listMessagesResponse{Messages: out})
	})
}

type listMessagesResponse struct {
	Messages []realtime.MessageView `json:"messages"`
}

func toMessageView(m store.Message) realtime.MessageView {
	return realtime.MessageView{
		ID:             m.ID,
		ChannelID:      m.ChannelID,
		ThreadID:       m.ThreadID,
		AuthorMemberID: m.AuthorMemberID,
		Content:        m.Content,
		CreatedAt:      m.CreatedAt,
		EditedAt:       m.EditedAt,
	}
}
