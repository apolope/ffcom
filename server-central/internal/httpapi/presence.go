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

// GET /api/presence — snapshot inicial do status visível (online, busy, away
// ou offline) de cada amigo aceito da conta autenticada. O client usa isso para popular a
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
			status := hub.Effective(friendID)
			out[i] = presenceView{AccountID: friendID, Status: status, Online: status != realtime.StatusOffline}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(presenceSnapshotResponse{Friends: out})
	})
}

// GET /api/presence/ws — o client mantém esta conexão aberta enquanto
// online. Sempre que o status visível da conta muda (primeira conexão,
// última desconexão, ociosidade, troca de status), o servidor emite
// "presence.update" para cada amigo aceito conectado, via
// realtime.Hub.SendTo. Frames aceitos do client: "presence.idle" (a pessoa
// ficou ociosa ou voltou nesta conexão, ver docs/architecture.md, "Decisão:
// status de presença e avatar nas listas de membros") e "dm.create" (DMs em
// tempo real na mesma conexão, ver "Decisão: DMs entregues no WebSocket de
// presença"), processado em handleIncomingDM (internal/httpapi/dms.go).
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
		// account vem do auth.Middleware no handshake, já com o status
		// escolhido lido do banco.
		broadcastIfChanged(hub, friendships, account.ID)(hub.Register(account.ID, client, account.PresenceStatus))

		go client.WritePump()
		client.ReadPump(func(raw []byte) {
			frameType, err := realtime.FrameType(raw)
			if err != nil {
				client.SendError(err.Error())
				return
			}
			if frameType == realtime.TypePresenceIdle {
				idle, err := realtime.DecodeIncomingPresenceIdle(raw)
				if err != nil {
					client.SendError(err.Error())
					return
				}
				broadcastIfChanged(hub, friendships, account.ID)(hub.SetIdle(account.ID, client, idle.Idle))
				return
			}
			handleIncomingDM(r.Context(), hub, friendships, directMessages, account.ID, client, raw)
		})

		broadcastIfChanged(hub, friendships, account.ID)(hub.Unregister(account.ID, client))
	})
}

// broadcastIfChanged devolve uma função que recebe o (antes, depois) de uma
// operação do Hub e só avisa os amigos quando o status visível mudou, para
// uma segunda aba, uma troca de "invisible" para offline ou uma ociosidade
// com a pessoa ocupada não gerarem evento nenhum.
func broadcastIfChanged(hub *realtime.Hub, friendships *store.FriendshipStore, accountID string) func(before, after string) {
	return func(before, after string) {
		if before != after {
			broadcastPresence(hub, friendships, accountID, after)
		}
	}
}

// broadcastPresence notifica cada amigo aceito de accountID (que estiver
// conectado) sobre o novo status visível de accountID. Usa um contexto novo,
// em vez do r.Context() do handshake, porque a chamada de desconexão
// acontece depois que a conexão HTTP original já terminou.
func broadcastPresence(hub *realtime.Hub, friendships *store.FriendshipStore, accountID, status string) {
	friendIDs, err := friendships.AcceptedFriendIDs(context.Background(), accountID)
	if err != nil {
		log.Printf("server-central: erro ao buscar amigos para broadcast de presença: %v", err)
		return
	}

	payload, err := realtime.EncodePresenceUpdate(accountID, status)
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
	Status    string `json:"status"`
	// Mantido para clients anteriores ao status.
	Online bool `json:"online"`
}

// PUT /api/me/status — grava o status escolhido (online, busy, away ou
// invisible) e avisa os amigos se o status visível mudou. A escolha
// persiste entre sessões; o status visível continua dependendo de haver
// conexão aberta (ver realtime.Hub.Effective).
func handleSetPresenceStatus(hub *realtime.Hub, accounts *store.AccountStore, friendships *store.FriendshipStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			http.Error(w, "conta não encontrada no contexto", http.StatusInternalServerError)
			return
		}

		var body setPresenceStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "corpo da requisição inválido", http.StatusBadRequest)
			return
		}
		if !store.ValidPresenceStatus(body.Status) {
			http.Error(w, "status precisa ser online, busy, away ou invisible", http.StatusBadRequest)
			return
		}

		if err := accounts.SetPresenceStatus(r.Context(), account.ID, body.Status); err != nil {
			http.Error(w, "erro ao salvar status", http.StatusInternalServerError)
			return
		}
		broadcastIfChanged(hub, friendships, account.ID)(hub.SetChosen(account.ID, body.Status))

		w.WriteHeader(http.StatusNoContent)
	})
}

type setPresenceStatusRequest struct {
	Status string `json:"status"`
}

type presenceSnapshotResponse struct {
	Friends []presenceView `json:"friends"`
}
