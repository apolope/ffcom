package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// GET /api/servers — lista o diretório de server-channel conhecidos pela
// conta autenticada (ver docs/architecture.md, "Decisão: descoberta de
// server-channel" — sem descoberta automática, só convite/endereço manual).
func handleListServers(servers *store.KnownServerStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		rows, err := servers.ListForAccount(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "erro ao buscar servidores", http.StatusInternalServerError)
			return
		}

		out := make([]knownServerView, len(rows))
		for i, k := range rows {
			out[i] = toKnownServerView(k)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listServersResponse{Servers: out})
	})
}

// POST /api/servers — adiciona (ou atualiza nome/ícone de) um server-channel
// ao diretório da conta autenticada, via endereço informado manualmente ou
// resolvido a partir de um convite.
func handleAddServer(servers *store.KnownServerStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		var body addServerRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		body.Address = strings.TrimSpace(body.Address)
		body.Name = strings.TrimSpace(body.Name)
		if body.Address == "" || body.Name == "" {
			http.Error(w, "address e name são obrigatórios", http.StatusBadRequest)
			return
		}

		k, err := servers.Add(r.Context(), account.ID, body.Address, body.Name, body.IconURL)
		if err != nil {
			http.Error(w, "erro ao adicionar servidor", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(toKnownServerView(k))
	})
}

// DELETE /api/servers/{id} — remove um server-channel do diretório da conta
// autenticada. Não afeta o server-channel em si (só a entrada local).
func handleRemoveServer(servers *store.KnownServerStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		id := r.PathValue("id")
		err := servers.Remove(r.Context(), account.ID, id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "servidor não encontrado", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "erro ao remover servidor", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

type reorderServersRequest struct {
	IDs []string `json:"ids"`
}

// PUT /api/servers/order — grava a ordem do rail da conta autenticada
// (arrastar e soltar os ícones). ids precisa ser a lista completa; se ela
// mudou em outra aba ou dispositivo, 409 e o client relê e tenta de novo.
func handleReorderServers(servers *store.KnownServerStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}
		var body reorderServersRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		err := servers.Reorder(r.Context(), account.ID, body.IDs)
		if errors.Is(err, store.ErrOrderMismatch) {
			http.Error(w, "a lista de servidores mudou; recarregue e tente de novo", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "erro ao reordenar servidores", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

type listServersResponse struct {
	Servers []knownServerView `json:"servers"`
}

type addServerRequest struct {
	Address string  `json:"address"`
	Name    string  `json:"name"`
	IconURL *string `json:"iconUrl,omitempty"`
}

type knownServerView struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	Name     string    `json:"name"`
	IconURL  *string   `json:"iconUrl,omitempty"`
	Position int       `json:"position"`
	AddedAt  time.Time `json:"addedAt"`
}

func toKnownServerView(k store.KnownServer) knownServerView {
	return knownServerView{
		ID:       k.ID,
		Address:  k.Address,
		Name:     k.Name,
		IconURL:  k.IconURL,
		Position: k.Position,
		AddedAt:  k.AddedAt,
	}
}
