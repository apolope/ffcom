// Package realtime mantém as conexões WebSocket do gateway de presença e
// permite empurrar eventos "presence.update" para contas específicas. Não
// há fanout por canal aqui (diferente do Hub de server-channel): o
// particionamento é por account_id, e uma conta pode ter mais de um client
// conectado ao mesmo tempo (múltiplas abas/dispositivos) — ela só sai do
// hub, e é considerada offline, quando o último client desconecta.
package realtime

import "sync"

// Hub agrupa os clients conectados, particionados por account_id.
type Hub struct {
	mu      sync.Mutex
	clients map[string]map[*Client]struct{}
}

// NewHub cria um Hub vazio.
func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[*Client]struct{})}
}

// Register associa client à conta accountID. wasOffline indica se essa
// conta não tinha nenhum client conectado antes deste registro — sinal
// para o chamador emitir presence.update online=true para os amigos.
func (h *Hub) Register(accountID string, client *Client) (wasOffline bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients, ok := h.clients[accountID]
	if !ok {
		clients = make(map[*Client]struct{})
		h.clients[accountID] = clients
	}
	wasOffline = len(clients) == 0
	clients[client] = struct{}{}
	return wasOffline
}

// Unregister remove client da conta accountID. wentOffline indica se essa
// era a última conexão da conta — sinal para o chamador emitir
// presence.update online=false para os amigos.
func (h *Hub) Unregister(accountID string, client *Client) (wentOffline bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients, ok := h.clients[accountID]
	if !ok {
		return false
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(h.clients, accountID)
		return true
	}
	return false
}

// IsOnline diz se accountID tem pelo menos um client conectado agora.
func (h *Hub) IsOnline(accountID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	return len(h.clients[accountID]) > 0
}

// SendTo entrega payload a todos os clients conectados de accountID (no-op
// se a conta não estiver online). Um client cuja fila de envio estiver
// cheia é considerado travado e desconectado, em vez de bloquear o envio
// para os demais.
func (h *Hub) SendTo(accountID string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.clients[accountID] {
		select {
		case client.send <- payload:
		default:
			close(client.send)
			delete(h.clients[accountID], client)
		}
	}
}
