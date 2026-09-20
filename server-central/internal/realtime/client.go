package realtime

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = pongWait * 9 / 10
	maxMessageSize = 4096
)

// Client é uma conexão WebSocket registrada no gateway de presença. send é
// o buffer de saída: ReadPump e WritePump rodam em goroutines separadas por
// conexão, seguindo o mesmo padrão gorilla/websocket usado em
// server-channel (internal/realtime).
type Client struct {
	conn *websocket.Conn
	send chan []byte
}

// NewClient prepara um Client em torno de uma conexão já upada.
func NewClient(conn *websocket.Conn) *Client {
	return &Client{conn: conn, send: make(chan []byte, 8)}
}

// WritePump escreve mensagens da fila de saída na conexão e envia pings
// periódicos para detectar conexões mortas. Encerra quando send é fechado
// (ver Hub.SendTo) ou a conexão falha, fechando o conn em seguida.
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
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
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

// ReadPump lê frames de texto da conexão e chama onMessage para cada um —
// hoje só frames "dm.create" (ver internal/httpapi/dms.go), já que o
// gateway de presença em si não aceita nenhum frame vindo do client. Além
// disso, detecta a desconexão do client e responde aos pings do WritePump
// com pong (mantendo o read deadline vivo). Bloqueia até a conexão fechar.
// O chamador deve rodar ReadPump na goroutine que fez o Upgrade e garantir
// Hub.Unregister quando ela retornar.
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
	payload, err := EncodeError(message)
	if err != nil {
		return
	}
	select {
	case c.send <- payload:
	default:
	}
}
