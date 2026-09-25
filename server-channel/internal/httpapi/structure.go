package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// visibleChannels devolve, dos canais informados, só os que member consegue
// ver (ViewChannels efetivo, já considerando overwrites de canal) — ver
// docs/architecture.md, "Sistema de permissões/roles por servidor e por
// canal". Dono do servidor sempre vê tudo, sem consultar overwrites.
func visibleChannels(ctx context.Context, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, member store.Member, channels []store.Channel) ([]store.Channel, error) {
	if member.IsOwner {
		return channels, nil
	}

	base, roleIDs, err := memberBasePermission(ctx, roles, member)
	if err != nil {
		return nil, err
	}

	rows, err := overwrites.ListForRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	byChannel := make(map[string][]permissions.Overwrite, len(rows))
	for _, o := range rows {
		byChannel[o.ChannelID] = append(byChannel[o.ChannelID], permissions.Overwrite{RoleID: o.RoleID, Allow: o.Allow, Deny: o.Deny})
	}

	visible := make([]store.Channel, 0, len(channels))
	for _, c := range channels {
		effective := permissions.Effective(base, roleIDs, byChannel[c.ID])
		if permissions.Has(effective, permissions.ViewChannels) {
			visible = append(visible, c)
		}
	}
	return visible, nil
}

// GET /api/categories — lista as categorias do servidor que tenham pelo
// menos um canal visível ao membro autenticado (ver visibleChannels), na
// mesma ordem por posição. Categoria sem nenhum canal visível não aparece —
// do contrário o nome de uma categoria privada vazaria mesmo com todo canal
// dentro dela restrito. Exceção: quem administra a estrutura (qualquer bit de
// permissions.StructureBits, ou é dono) vê
// todas, inclusive vazias, senão uma categoria recém-criada ficaria
// invisível justamente para quem precisa pôr o primeiro canal nela (ver
// "Decisão: gerenciar categorias e canais").
func handleListCategories(categories *store.CategoryStore, channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}

		allCategories, err := categories.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao buscar categorias", http.StatusInternalServerError)
			return
		}
		allChannels, err := channels.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao buscar canais", http.StatusInternalServerError)
			return
		}
		visible, err := visibleChannels(r.Context(), roles, overwrites, member, allChannels)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}

		base, _, err := memberBasePermission(r.Context(), roles, member)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}
		showAll := permissions.Has(base, permissions.StructureBits)

		withVisibleChannel := make(map[string]bool, len(visible))
		for _, c := range visible {
			if c.CategoryID != nil {
				withVisibleChannel[*c.CategoryID] = true
			}
		}

		out := make([]categoryView, 0, len(allCategories))
		for _, c := range allCategories {
			if showAll || withVisibleChannel[c.ID] {
				out = append(out, toCategoryView(c))
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listCategoriesResponse{Categories: out})
	})
}

// GET /api/channels — lista os canais visíveis ao membro autenticado (todos
// os tipos), ordenados por categoria e posição. O client agrupa por
// categoryId; canais sem categoria vêm com categoryId nulo. lastMessageAt
// (ver MessageStore.LastMessageAtByChannel) alimenta o indicador de não lida
// no client — comparado contra um cursor "última leitura" guardado local ao
// dispositivo (localStorage), não há conceito de "lido" no servidor (ver
// docs/architecture.md, "Decisão: indicador de não lida").
func handleListChannels(channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			http.Error(w, "membro não encontrado no contexto", http.StatusInternalServerError)
			return
		}

		rows, err := channels.List(r.Context())
		if err != nil {
			http.Error(w, "erro ao buscar canais", http.StatusInternalServerError)
			return
		}
		visible, err := visibleChannels(r.Context(), roles, overwrites, member, rows)
		if err != nil {
			http.Error(w, "erro ao resolver permissões", http.StatusInternalServerError)
			return
		}
		lastMessageAt, err := messages.LastMessageAtByChannel(r.Context())
		if err != nil {
			http.Error(w, "erro ao buscar atividade dos canais", http.StatusInternalServerError)
			return
		}

		out := make([]channelView, len(visible))
		for i, c := range visible {
			view := toChannelView(c)
			if at, ok := lastMessageAt[c.ID]; ok {
				view.LastMessageAt = &at
			}
			out[i] = view
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
	ID            string     `json:"id"`
	CategoryID    *string    `json:"categoryId,omitempty"`
	Name          string     `json:"name"`
	Type          string     `json:"type"`
	Position      int        `json:"position"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastMessageAt *time.Time `json:"lastMessageAt,omitempty"`
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
