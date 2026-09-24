// Package realtime mantém as conexões WebSocket do gateway de presença e
// permite empurrar eventos "presence.update" para contas específicas. Não
// há fanout por canal aqui (diferente do Hub de server-channel): o
// particionamento é por account_id, e uma conta pode ter mais de um client
// conectado ao mesmo tempo (múltiplas abas/dispositivos) — ela só sai do
// hub, e é considerada offline, quando o último client desconecta.
package realtime

import "sync"

// Status que os amigos veem (ver Hub.Effective). "invisible" nunca sai daqui:
// para os outros, é "offline".
const (
	StatusOnline  = "online"
	StatusBusy    = "busy"
	StatusAway    = "away"
	StatusOffline = "offline"

	chosenInvisible = "invisible"
)

// account é o estado em memória de uma conta com pelo menos uma conexão: o
// status que ela escolheu e, por conexão, se aquela conexão está ociosa
// (ausente automático, informado pelo client com "presence.idle").
type account struct {
	chosen  string
	clients map[*Client]bool
}

// Hub agrupa os clients conectados, particionados por account_id.
type Hub struct {
	mu       sync.Mutex
	accounts map[string]*account
}

// NewHub cria um Hub vazio.
func NewHub() *Hub {
	return &Hub{accounts: make(map[string]*account)}
}

// effective calcula o status que os amigos veem. Chamar com h.mu travado.
func (h *Hub) effective(accountID string) string {
	a, ok := h.accounts[accountID]
	if !ok || len(a.clients) == 0 || a.chosen == chosenInvisible {
		return StatusOffline
	}
	if a.chosen != StatusOnline {
		return a.chosen
	}
	// Online com todas as conexões ociosas vira ausente: basta um dispositivo
	// em uso para a pessoa continuar online.
	for _, idle := range a.clients {
		if !idle {
			return StatusOnline
		}
	}
	return StatusAway
}

// Effective devolve o status de accountID como os amigos o veem.
func (h *Hub) Effective(accountID string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.effective(accountID)
}

// Register associa client à conta accountID, com o status escolhido lido do
// banco na conexão. Devolve o status visível antes e depois: se mudou, o
// chamador emite presence.update para os amigos.
func (h *Hub) Register(accountID string, client *Client, chosen string) (before, after string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	before = h.effective(accountID)
	a, ok := h.accounts[accountID]
	if !ok {
		a = &account{clients: make(map[*Client]bool)}
		h.accounts[accountID] = a
	}
	a.chosen = chosen
	a.clients[client] = false
	return before, h.effective(accountID)
}

// Unregister remove client da conta accountID. Devolve o status visível
// antes e depois (a última conexão caindo vira offline; uma conexão ativa
// caindo pode deixar a conta ausente).
func (h *Hub) Unregister(accountID string, client *Client) (before, after string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	before = h.effective(accountID)
	a, ok := h.accounts[accountID]
	if !ok {
		return before, before
	}
	delete(a.clients, client)
	if len(a.clients) == 0 {
		delete(h.accounts, accountID)
	}
	return before, h.effective(accountID)
}

// SetIdle marca uma conexão como ociosa ou em uso. Devolve o status visível
// antes e depois.
func (h *Hub) SetIdle(accountID string, client *Client, idle bool) (before, after string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	before = h.effective(accountID)
	if a, ok := h.accounts[accountID]; ok {
		if _, registered := a.clients[client]; registered {
			a.clients[client] = idle
		}
	}
	return before, h.effective(accountID)
}

// SetChosen troca o status escolhido de uma conta conectada (no-op se ela
// não tiver conexão: o banco já guarda a escolha para a próxima). Devolve o
// status visível antes e depois.
func (h *Hub) SetChosen(accountID, chosen string) (before, after string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	before = h.effective(accountID)
	if a, ok := h.accounts[accountID]; ok {
		a.chosen = chosen
	}
	return before, h.effective(accountID)
}

// SendTo entrega payload a todos os clients conectados de accountID (no-op
// se a conta não estiver conectada). Um client cuja fila de envio estiver
// cheia é considerado travado e desconectado, em vez de bloquear o envio
// para os demais.
func (h *Hub) SendTo(accountID string, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	a, ok := h.accounts[accountID]
	if !ok {
		return
	}
	for client := range a.clients {
		select {
		case client.send <- payload:
		default:
			close(client.send)
			delete(a.clients, client)
		}
	}
}
