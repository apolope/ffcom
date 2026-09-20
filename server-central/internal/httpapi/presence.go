package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/realtime"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// newPresenceUpgrader monta o upgrader do WebSocket de presença. Segue o
// mesmo CheckOrigin/Subprotocols de server-channel (ver
// internal/httpapi/channel_ws.go daquele componente e
// docs/architecture.md, "Decisão: CORS em server-channel").
func newPresenceUpgrader(allowedOrigins map[string]bool) websocket.Upgrader {
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

// GET /api/presence — snapshot inicial de quem, entre os amigos aceitos da
// conta autenticada, está online agora. O client usa isso para popular a
// lista de amigos ao abrir; depois disso, GET /api/presence/ws entrega as
// mudanças em tempo real.
func handlePresenceSnapshot(hub *realtime.Hub, friendships *store.FriendshipStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		friendIDs, err := friendships.AcceptedFriendIDs(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "erro ao buscar amigos", http.StatusInternalServerError)
			return
		}

		out := make([]presenceView, len(friendIDs))
		for i, friendID := range friendIDs {
			out[i] = presenceView{AccountID: friendID, Online: hub.IsOnline(friendID)}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(presenceSnapshotResponse{Friends: out})
	})
}

// GET /api/presence/ws — o client mantém esta conexão aberta enquanto
// online. Ao conectar (primeira conexão da conta) e ao desconectar (última
// conexão da conta), o servidor emite "presence.update" para cada amigo
// aceito que estiver online agora, via realtime.Hub.SendTo. Além de
// presença, esta mesma conexão carrega as DMs em tempo real (ver
// docs/architecture.md, "Decisão: DMs entregues no WebSocket de
// presença"): o único frame aceito vindo do client é "dm.create",
// processado em handleIncomingDM (internal/httpapi/dms.go).
func handlePresenceWS(hub *realtime.Hub, friendships *store.FriendshipStore, directMessages *store.DirectMessageStore, upgrader websocket.Upgrader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// upgrader já escreveu a resposta HTTP de erro.
			return
		}

		client := realtime.NewClient(conn)
		wasOffline := hub.Register(account.ID, client)
		if wasOffline {
			broadcastPresence(hub, friendships, account.ID, true)
		}

		go client.WritePump()
		client.ReadPump(func(raw []byte) {
			handleIncomingDM(r.Context(), hub, friendships, directMessages, account.ID, client, raw)
		})

		wentOffline := hub.Unregister(account.ID, client)
		if wentOffline {
			broadcastPresence(hub, friendships, account.ID, false)
		}
	})
}

// broadcastPresence notifica cada amigo aceito de accountID (que estiver
// online) sobre a mudança de status de accountID. Usa um contexto novo, em
// vez do r.Context() do handshake, porque a chamada de desconexão acontece
// depois que a conexão HTTP original já terminou.
func broadcastPresence(hub *realtime.Hub, friendships *store.FriendshipStore, accountID string, online bool) {
	friendIDs, err := friendships.AcceptedFriendIDs(context.Background(), accountID)
	if err != nil {
		log.Printf("server-central: erro ao buscar amigos para broadcast de presença: %v", err)
		return
	}

	payload, err := realtime.EncodePresenceUpdate(accountID, online)
	if err != nil {
		log.Printf("server-central: erro ao codificar presence.update: %v", err)
		return
	}

	for _, friendID := range friendIDs {
		hub.SendTo(friendID, payload)
	}
}

type presenceView struct {
	AccountID string `json:"accountId"`
	Online    bool   `json:"online"`
}

type presenceSnapshotResponse struct {
	Friends []presenceView `json:"friends"`
}
