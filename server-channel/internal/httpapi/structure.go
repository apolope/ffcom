package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// GET /api/categories — lista todas as categorias do servidor, ordenadas por
// posição. Ver TODO.md ("API REST em server-channel para o client listar
// categorias/canais reais").
func handleListCategories(categories *store.CategoryStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := categories.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao buscar categorias", http.StatusInternalServerError)
			return
		}

		out := make([]categoryView, len(rows))
		for i, c := range rows {
			out[i] = toCategoryView(c)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listCategoriesResponse{Categories: out})
	})
}

// GET /api/channels — lista todos os canais do servidor (todos os tipos),
// ordenados por categoria e posição. O client agrupa por categoryId; canais
// sem categoria vêm com categoryId nulo.
func handleListChannels(channels *store.ChannelStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows, err := channels.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao buscar canais", http.StatusInternalServerError)
			return
		}

		out := make([]channelView, len(rows))
		for i, c := range rows {
			out[i] = toChannelView(c)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listChannelsResponse{Channels: out})
	})
}

type listCategoriesResponse struct {
	Categories []categoryView `json:"categories"`
}

type listChannelsResponse struct {
	Channels []channelView `json:"channels"`
}

type categoryView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"createdAt"`
}

type channelView struct {
	ID         string    `json:"id"`
	CategoryID *string   `json:"categoryId,omitempty"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Position   int       `json:"position"`
	CreatedAt  time.Time `json:"createdAt"`
}

func toCategoryView(c store.Category) categoryView {
	return categoryView{
		ID:        c.ID,
		Name:      c.Name,
		Position:  c.Position,
		CreatedAt: c.CreatedAt,
	}
}

func toChannelView(c store.Channel) channelView {
	return channelView{
		ID:         c.ID,
		CategoryID: c.CategoryID,
		Name:       c.Name,
		Type:       string(c.Type),
		Position:   c.Position,
		CreatedAt:  c.CreatedAt,
	}
}
