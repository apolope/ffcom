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

// GET /api/channels/{id}/threads — lista as threads de um canal forum, mais
// recentes primeiro. Abrir/postar numa thread acontece pelo WebSocket do
// próprio canal (ver internal/httpapi/channel_ws.go); REST aqui cobre só a
// listagem, mesmo split já usado em canal de texto (ver
// docs/architecture.md, "Canal forum: threads/posts").
func handleListThreads(channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore) http.Handler {
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
		if channel.Type != store.ChannelForum {
			http.Error(w, "canal não é forum", http.StatusBadRequest)
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

		rows, err := messages.ListThreads(r.Context(), channelID)
		if err != nil {
			http.Error(w, "erro ao buscar threads", http.StatusInternalServerError)
			return
		}

		out := make([]realtime.ThreadView, len(rows))
		for i, t := range rows {
			out[i] = toThreadView(t)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listThreadsResponse{Threads: out})
	})
}

// GET /api/threads/{id}/messages?before=RFC3339&limit=N — histórico paginado
// dos posts de uma thread (mesmo esquema de keyset pagination do canal de
// texto, ver handleListMessages). A permissão checada é ViewChannels do
// canal forum dono da thread, resolvido via MessageStore.GetThread.
func handleListThreadMessages(roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		threadID := r.PathValue("id")

		thread, err := messages.GetThread(r.Context(), threadID)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "thread não encontrada", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao buscar thread", http.StatusInternalServerError)
			return
		}

		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}
		effective, err := channelPermission(r.Context(), roles, overwrites, member, thread.ChannelID)
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

		rows, err := messages.ListForChannel(r.Context(), thread.ChannelID, &threadID, before, limit)
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

type listThreadsResponse struct {
	Threads []realtime.ThreadView `json:"threads"`
}

func toThreadView(t store.Thread) realtime.ThreadView {
	return realtime.ThreadView{
		ID:             t.ID,
		ChannelID:      t.ChannelID,
		Title:          t.Title,
		AuthorMemberID: t.AuthorMemberID,
		CreatedAt:      t.CreatedAt,
	}
}
