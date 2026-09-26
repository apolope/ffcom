package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/realtime"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// Pedidos de amizade explícitos, feitos a partir da lista de membros de um
// server-channel (ver docs/architecture.md, "Decisão: pedido de amizade pela
// lista de membros"). Convivem com o convite por código: o pedido precisa do
// accountId do outro lado, que o client só conhece para quem divide um
// servidor com ele (via POST /api/accounts/lookup), então não abre um
// diretório de contas pesquisável.
//
// Cada mudança avisa as duas pontas pelo WebSocket de presença
// (friend.request, friend.request.removed, friend.accepted), para a lista de
// pedidos atualizar sem polling em todas as abas abertas.

// POST /api/friends/requests — manda um pedido de amizade para accountId.
// Se o outro lado já tinha mandado um pedido para a conta autenticada, o
// pedido dele é aceito na hora em vez de criar um segundo (os dois querem a
// amizade).
func handleCreateFriendRequest(hub *realtime.Hub, friendships *store.FriendshipStore, accounts *store.AccountStore, profiles *store.ProfileStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		var body createFriendRequestBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.AccountID == "" {
			http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
			return
		}
		if body.AccountID == account.ID {
			http.Error(w, "não é possível adicionar a si mesmo", http.StatusBadRequest)
			return
		}

		target, err := accounts.GetManyByIDs(r.Context(), []string{body.AccountID})
		if err != nil {
			// Um id que não é UUID também cai aqui (erro de cast no Postgres).
			http.Error(w, "conta não encontrada", http.StatusNotFound)
			return
		}
		if _, ok := target[body.AccountID]; !ok {
			http.Error(w, "conta não encontrada", http.StatusNotFound)
			return
		}

		existing, err := friendships.Between(r.Context(), account.ID, body.AccountID)
		switch {
		case errors.Is(err, store.ErrNotFound):
			// Nenhuma relação ainda: cria o pedido abaixo.
		case err != nil:
			http.Error(w, "erro ao buscar amizade", http.StatusInternalServerError)
			return
		case existing.Status == store.FriendshipAccepted:
			http.Error(w, "vocês já são amigos", http.StatusConflict)
			return
		case existing.Status == store.FriendshipPending && existing.RequesterID == account.ID:
			http.Error(w, "pedido de amizade já enviado", http.StatusConflict)
			return
		case existing.Status == store.FriendshipPending:
			accepted, err := friendships.Accept(r.Context(), existing.ID, account.ID)
			if err != nil {
				http.Error(w, "erro ao aceitar pedido de amizade", http.StatusInternalServerError)
				return
			}
			notifyFriendAccepted(r.Context(), hub, profiles, accepted)
			writeFriendRequestResult(r.Context(), w, http.StatusOK, accepted, body.AccountID, profiles)
			return
		default:
			http.Error(w, "não é possível adicionar esta conta", http.StatusConflict)
			return
		}

		created, err := friendships.Request(r.Context(), account.ID, body.AccountID)
		if errors.Is(err, store.ErrConflict) {
			http.Error(w, "já existe um pedido entre vocês", http.StatusConflict)
			return
		}
		if err != nil {
			http.Error(w, "erro ao criar pedido de amizade", http.StatusInternalServerError)
			return
		}

		// Quem recebe vê o pedido do ponto de vista dele: o outro lado é quem
		// mandou.
		if payload, err := realtime.EncodeFriendRequest(requestViewFor(r.Context(), profiles, created, account.ID)); err == nil {
			hub.SendTo(body.AccountID, payload)
		} else {
			log.Printf("server-central: erro ao codificar friend.request: %v", err)
		}

		writeFriendRequestResult(r.Context(), w, http.StatusCreated, created, body.AccountID, profiles)
	})
}

// GET /api/friends/requests — pedidos pendentes da conta autenticada,
// separados em recebidos (para aprovar ou recusar) e enviados (aguardando).
func handleListFriendRequests(friendships *store.FriendshipStore, profiles *store.ProfileStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		pending, err := friendships.PendingForAccount(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "erro ao buscar pedidos de amizade", http.StatusInternalServerError)
			return
		}

		otherIDs := make([]string, len(pending))
		for i, f := range pending {
			otherIDs[i] = otherSide(f, account.ID)
		}
		profileByAccount, err := profiles.GetManyByAccountIDs(r.Context(), otherIDs)
		if err != nil {
			http.Error(w, "erro ao buscar perfis", http.StatusInternalServerError)
			return
		}

		out := listFriendRequestsResponse{
			Incoming: []realtime.FriendRequestView{},
			Outgoing: []realtime.FriendRequestView{},
		}
		for _, f := range pending {
			view := realtime.FriendRequestView{ID: f.ID, AccountID: otherSide(f, account.ID), CreatedAt: f.CreatedAt}
			if profile, ok := profileByAccount[view.AccountID]; ok {
				view.DisplayName = &profile.DisplayName
				view.AvatarURL = profile.AvatarURL
			}
			if f.AddresseeID == account.ID {
				out.Incoming = append(out.Incoming, view)
			} else {
				out.Outgoing = append(out.Outgoing, view)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
}

// POST /api/friends/requests/{id}/accept — só quem recebeu o pedido aceita.
func handleAcceptFriendRequest(hub *realtime.Hub, friendships *store.FriendshipStore, profiles *store.ProfileStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		accepted, err := friendships.Accept(r.Context(), r.PathValue("id"), account.ID)
		if err != nil {
			// Id que não é UUID também cai aqui; para quem chamou, é igual a
			// não encontrado.
			http.Error(w, "pedido de amizade não encontrado", http.StatusNotFound)
			return
		}

		notifyFriendAccepted(r.Context(), hub, profiles, accepted)
		writeFriendRequestResult(r.Context(), w, http.StatusOK, accepted, accepted.RequesterID, profiles)
	})
}

// DELETE /api/friends/requests/{id} — recusa (quem recebeu) ou cancela
// (quem mandou) um pedido pendente. Os dois lados recebem
// friend.request.removed.
func handleDeleteFriendRequest(hub *realtime.Hub, friendships *store.FriendshipStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		removed, err := friendships.DeletePending(r.Context(), r.PathValue("id"), account.ID)
		if err != nil {
			http.Error(w, "pedido de amizade não encontrado", http.StatusNotFound)
			return
		}

		if payload, err := realtime.EncodeFriendRequestRemoved(removed.ID); err == nil {
			hub.SendTo(removed.RequesterID, payload)
			hub.SendTo(removed.AddresseeID, payload)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// notifyFriendAccepted avisa as duas pontas da amizade nova, cada uma com o
// outro lado como accountId. Presença não precisa de broadcast extra: o
// client recarrega amigos e o snapshot de GET /api/presence ao receber o
// frame.
func notifyFriendAccepted(ctx context.Context, hub *realtime.Hub, profiles *store.ProfileStore, f store.Friendship) {
	names, err := profiles.GetManyByAccountIDs(ctx, []string{f.RequesterID, f.AddresseeID})
	if err != nil {
		names = map[string]store.Profile{}
	}
	send := func(to, other string) {
		var name *string
		if p, ok := names[other]; ok {
			name = &p.DisplayName
		}
		payload, err := realtime.EncodeFriendAccepted(f.ID, other, name)
		if err != nil {
			log.Printf("server-central: erro ao codificar friend.accepted: %v", err)
			return
		}
		hub.SendTo(to, payload)
	}
	send(f.RequesterID, f.AddresseeID)
	send(f.AddresseeID, f.RequesterID)
}

// requestViewFor monta o pedido f como viewerID o enxerga: o outro lado da
// relação, com nome e avatar quando houver perfil.
func requestViewFor(ctx context.Context, profiles *store.ProfileStore, f store.Friendship, otherID string) realtime.FriendRequestView {
	view := realtime.FriendRequestView{ID: f.ID, AccountID: otherID, CreatedAt: f.CreatedAt}
	if profile, err := profiles.GetByAccountID(ctx, otherID); err == nil {
		view.DisplayName = &profile.DisplayName
		view.AvatarURL = profile.AvatarURL
	}
	return view
}

func writeFriendRequestResult(ctx context.Context, w http.ResponseWriter, code int, f store.Friendship, otherID string, profiles *store.ProfileStore) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(friendRequestResult{
		Status:  string(f.Status),
		Request: requestViewFor(ctx, profiles, f, otherID),
	})
}

func otherSide(f store.Friendship, accountID string) string {
	if f.RequesterID == accountID {
		return f.AddresseeID
	}
	return f.RequesterID
}

type createFriendRequestBody struct {
	AccountID string `json:"accountId"`
}

// friendRequestResult: status "pending" (pedido criado) ou "accepted" (o
// outro lado já tinha pedido, ou o pedido foi aceito agora).
type friendRequestResult struct {
	Status  string                     `json:"status"`
	Request realtime.FriendRequestView `json:"request"`
}

type listFriendRequestsResponse struct {
	Incoming []realtime.FriendRequestView `json:"incoming"`
	Outgoing []realtime.FriendRequestView `json:"outgoing"`
}
