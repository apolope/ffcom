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
	closed   bool
}

// NewHub cria um Hub vazio.
func NewHub() *Hub {
	return &Hub{channels: make(map[string]map[*Client]struct{})}
}

// Register associa client ao canal channelID, passando a receber o
// broadcast de mensagens desse canal. Depois de Close, o client é
// encerrado na hora em vez de registrado (conexão que terminou o upgrade
// durante o graceful shutdown).
func (h *Hub) Register(channelID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		client.closeSend()
		return
	}
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
		if !client.trySend(payload) {
			client.closeSend()
			delete(h.channels[channelID], client)
		}
	}
}

// SetViewing guarda se a conexão client está "vendo" o canal agora: a
// página visível, a janela em foco e a pessoa sem ficar ausente, como o
// client informa no frame "channel.viewing" (ver docs/protocol.md). Só
// isso tira a pessoa do push da mensagem (ViewingMemberIDs); uma conexão
// aberta numa aba escondida ou num app minimizado não conta. Client que
// não está registrado no canal é ignorado.
func (h *Hub) SetViewing(channelID string, client *Client, viewing bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.channels[channelID][client]; ok {
		client.viewing = viewing
	}
}

// ViewingMemberIDs devolve os membros com pelo menos uma conexão no canal
// channelID marcada como vendo (SetViewing). Usado para não mandar
// notificação push a quem já está com o canal na frente dos olhos (ver
// internal/httpapi/push.go). Conexão nova começa sem ver: client antigo,
// que nunca manda "channel.viewing", não bloqueia o push de ninguém.
func (h *Hub) ViewingMemberIDs(channelID string) map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make(map[string]bool)
	for client := range h.channels[channelID] {
		if client.viewing && client.MemberID != "" {
			out[client.MemberID] = true
		}
	}
	return out
}

// Close encerra todas as conexões registradas no graceful shutdown (ver
// main.go): fecha a fila de saída de cada client, o que faz o WritePump
// mandar um close frame e fechar o conn, e o ReadPump do handler retorna
// em seguida. Necessário porque http.Server.Shutdown não acompanha
// conexões sequestradas pelo upgrade do WebSocket.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.closed = true
	for channelID, clients := range h.channels {
		for client := range clients {
			client.closeSend()
		}
		delete(h.channels, channelID)
	}
}
