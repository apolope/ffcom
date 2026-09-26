package realtime

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = pongWait * 9 / 10
	maxMessageSize = 4096
)

// Client é uma conexão WebSocket registrada num canal. send é o buffer de
// saída: ReadPump e WritePump rodam em goroutines separadas por conexão,
// seguindo o padrão recomendado pelo gorilla/websocket (uma única goroutine
// escreve no *websocket.Conn por vez).
//
// sendMu/sendClosed protegem o close de send: o Hub fecha a fila (client
// travado ou Hub.Close no shutdown) enquanto o ReadPump ainda pode chamar
// SendError, e mandar num canal fechado derrubaria o processo.
type Client struct {
	conn       *websocket.Conn
	send       chan []byte
	sendMu     sync.Mutex
	sendClosed bool
}

// NewClient prepara um Client em torno de uma conexão já upada.
func NewClient(conn *websocket.Conn) *Client {
	return &Client{conn: conn, send: make(chan []byte, 16)}
}

// WritePump escreve mensagens da fila de saída na conexão e envia pings
// periódicos para detectar conexões mortas (comuns em client atrás de NAT
// ou proxy). Encerra quando send é fechado (ver Hub.Broadcast e Hub.Close)
// ou a conexão falha, fechando o conn em seguida.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case payload, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// 1001 (going away): o client sabe que pode reconectar.
				c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, ""))
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump lê frames de texto da conexão e chama onMessage para cada um.
// Bloqueia até a conexão fechar (client desconectou, erro de rede ou frame
// maior que maxMessageSize). O chamador deve rodar ReadPump na goroutine
// que fez o Upgrade e garantir Hub.Unregister quando ela retornar.
func (c *Client) ReadPump(onMessage func([]byte)) {
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		onMessage(payload)
	}
}

// SendError envia um envelope de erro só para este client, sem broadcast.
func (c *Client) SendError(message string) {
	payload, err := encodeError(message)
	if err != nil {
		return
	}
	c.trySend(payload)
}

// trySend enfileira payload sem bloquear. Devolve false se a fila estiver
// cheia ou já fechada.
func (c *Client) trySend(payload []byte) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if c.sendClosed {
		return false
	}
	select {
	case c.send <- payload:
		return true
	default:
		return false
	}
}

// closeSend fecha a fila de saída uma única vez; o WritePump manda o close
// frame e fecha a conexão ao perceber.
func (c *Client) closeSend() {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if !c.sendClosed {
		c.sendClosed = true
		close(c.send)
	}
}
