package realtime

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = pongWait * 9 / 10
	maxMessageSize = 1024
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

// ReadPump só existe para detectar a desconexão do client e responder aos
// pings do WritePump com pong (mantendo o read deadline vivo) — o gateway
// de presença não aceita nenhum frame vindo do client, então os payloads
// lidos aqui são descartados. Bloqueia até a conexão fechar. O chamador
// deve rodar ReadPump na goroutine que fez o Upgrade e garantir
// Hub.Unregister quando ela retornar.
func (c *Client) ReadPump() {
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
