// Package realtime mantém as conexões WebSocket de canais de texto e faz o
// fanout de mensagens novas para todos os membros conectados a um mesmo
// canal. Não há estado compartilhado entre canais: cada canal tem seu
// próprio conjunto de clients.
package realtime

import "sync"

// Hub agrupa os clients conectados, particionados por canal.
type Hub struct {
	mu       sync.Mutex
	channels map[string]map[*Client]struct{}
}

// NewHub cria um Hub vazio.
func NewHub() *Hub {
	return &Hub{channels: make(map[string]map[*Client]struct{})}
}

// Register associa client ao canal channelID, passando a receber o
// broadcast de mensagens desse canal.
func (h *Hub) Register(channelID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients, ok := h.channels[channelID]
	if !ok {
		clients = make(map[*Client]struct{})
		h.channels[channelID] = clients
	}
	clients[client] = struct{}{}
}

// Unregister remove client do canal channelID e limpa o canal do mapa
// quando fica vazio.
func (h *Hub) Unregister(channelID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients, ok := h.channels[channelID]
	if !ok {
		return
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(h.channels, channelID)
	}
}

// Broadcast envia payload a todos os clients conectados ao canal
// channelID. Um client cuja fila de envio estiver cheia é considerado
// travado e desconectado, em vez de bloquear o broadcast para os demais.
func (h *Hub) Broadcast(channelID string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.channels[channelID] {
		select {
		case client.send <- payload:
		default:
			close(client.send)
			delete(h.channels[channelID], client)
		}
	}
}
