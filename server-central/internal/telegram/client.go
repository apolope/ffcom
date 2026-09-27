// Package telegram envia e edita mensagens no grupo "Rede" pela Bot API,
// com o mesmo bot do a3s-network-monitor. Só envia: o webhook do bot aponta
// para o monitor, e é por lá que chegam os cliques nos botões (o monitor
// repassa os do FFCom ao listener interno do server-central). Por isso este
// pacote nunca chama setWebhook, deleteWebhook nem getUpdates, que
// quebrariam o monitor. Ver docs/architecture.md, "Decisão: cadastro com
// aprovação pelo Telegram".
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxRetryAfter limita a espera pedida pelo Telegram num 429: acima disso,
// desiste e deixa o notificador tentar depois, em vez de prender a
// requisição de quem está se cadastrando.
const maxRetryAfter = 10 * time.Second

// Client fala com a Bot API. O zero value não serve: use New.
type Client struct {
	baseURL  string
	token    string
	chatID   string
	threadID int64
	http     *http.Client
}

// New devolve um Client que posta no chat chatID e, com threadID diferente
// de zero, no tópico threadID (grupo com tópicos; sem ele cai no "Geral").
func New(token, chatID string, threadID int64) *Client {
	return &Client{
		baseURL:  "https://api.telegram.org",
		token:    token,
		chatID:   chatID,
		threadID: threadID,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

// WithBaseURL troca o endereço da Bot API (testes).
func (c *Client) WithBaseURL(baseURL string) *Client {
	c.baseURL = strings.TrimRight(baseURL, "/")
	return c
}

// Button é um botão inline; Data volta no callback_query do clique (até 64
// bytes).
type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

// Escape prepara texto vindo de fora para parse_mode HTML: sem isso um "<"
// num nome faz a API devolver 400.
func Escape(s string) string {
	return html.EscapeString(s)
}

// SendMessage envia text (HTML) com uma linha de botões e devolve o
// message_id.
func (c *Client) SendMessage(ctx context.Context, text string, buttons []Button) (int64, error) {
	body := map[string]any{
		"chat_id":                  c.chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if c.threadID != 0 {
		body["message_thread_id"] = c.threadID
	}
	if len(buttons) > 0 {
		body["reply_markup"] = map[string]any{"inline_keyboard": [][]Button{buttons}}
	}
	var result struct {
		MessageID int64 `json:"message_id"`
	}
	if err := c.call(ctx, "sendMessage", body, &result); err != nil {
		return 0, err
	}
	return result.MessageID, nil
}

// EditMessage troca o texto da mensagem messageID. Sem buttons, os botões
// somem, o que impede um segundo clique depois da decisão.
func (c *Client) EditMessage(ctx context.Context, messageID int64, text string, buttons []Button) error {
	body := map[string]any{
		"chat_id":                  c.chatID,
		"message_id":               messageID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if len(buttons) > 0 {
		body["reply_markup"] = map[string]any{"inline_keyboard": [][]Button{buttons}}
	}
	return c.call(ctx, "editMessageText", body, nil)
}

func (c *Client) call(ctx context.Context, method string, body map[string]any, result any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/bot"+c.token+"/"+method, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			// A URL leva o token; o erro do net/http a repete.
			return fmt.Errorf("telegram: %s: %s", method, strings.ReplaceAll(err.Error(), c.token, "***"))
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()

		var parsed struct {
			OK          bool            `json:"ok"`
			Description string          `json:"description"`
			Result      json.RawMessage `json:"result"`
			Parameters  struct {
				RetryAfter int `json:"retry_after"`
			} `json:"parameters"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("telegram: %s: resposta %d fora do formato", method, resp.StatusCode)
		}
		if parsed.OK {
			if result != nil {
				return json.Unmarshal(parsed.Result, result)
			}
			return nil
		}
		wait := time.Duration(parsed.Parameters.RetryAfter) * time.Second
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 && wait > 0 && wait <= maxRetryAfter {
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return fmt.Errorf("telegram: %s: %d %s", method, resp.StatusCode, parsed.Description)
	}
}
