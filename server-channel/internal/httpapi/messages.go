package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
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
func handleListMessages(channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore, attachments *store.AttachmentStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID := r.PathValue("id")

		channel, err := channels.GetByID(r.Context(), channelID)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "channels.not_found", "canal não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "channels.fetch_failed", "erro ao buscar canal")
			return
		}
		if channel.Type != store.ChannelText {
			apierr.Write(w, http.StatusBadRequest, "channels.not_text", "canal não é de texto")
			return
		}

		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
			return
		}
		effective, err := channelPermission(r.Context(), roles, overwrites, member, channelID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
			return
		}
		if !permissions.Has(effective, permissions.ViewChannels) {
			apierr.Write(w, http.StatusForbidden, "channels.view_denied", "sem permissão para ver este canal")
			return
		}

		limit := defaultHistoryLimit
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n <= 0 {
				apierr.Write(w, http.StatusBadRequest, "common.limit_invalid", "limit inválido")
				return
			}
			limit = min(n, maxHistoryLimit)
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

		rows, err := messages.ListForChannel(r.Context(), channelID, nil, before, limit)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "messages.history_fetch_failed", "erro ao buscar histórico")
			return
		}

		messageIDs := make([]string, len(rows))
		for i, m := range rows {
			messageIDs[i] = m.ID
		}
		byMessage, err := attachments.ListForMessages(r.Context(), messageIDs)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "messages.attachments_fetch_failed", "erro ao buscar anexos")
			return
		}

		out := make([]realtime.MessageView, len(rows))
		for i, m := range rows {
			view := toMessageView(m)
			view.Attachments = toAttachmentViews(byMessage[m.ID])
			out[i] = view
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
