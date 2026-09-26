// Package relay fala com o a3s-claude-relay da infra a3s-network: um serviço
// na rede Docker a3s-services que recebe um prompt, roda o Claude Code (sem
// ferramentas, só texto) e devolve o resultado por webhook. Ver
// D:\Dev\a3s-network\docs\services\a3s-claude-relay.md.
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client envia prompts ao relay. O zero value não serve: use New.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New devolve um Client para o relay em baseURL (ex.
// http://a3s-claude-relay:3000), autenticado por apiKey (header X-API-KEY).
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Submit enfileira prompt no relay, que chama webhookURL com o resultado
// quando terminar. Devolve o jobId do relay (202 Accepted).
func (c *Client) Submit(ctx context.Context, prompt, webhookURL string) (string, error) {
	body, err := json.Marshal(map[string]string{"prompt": prompt, "webhookUrl": webhookURL})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/prompts", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("relay: enviar prompt: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("relay: resposta %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var accepted struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(raw, &accepted); err != nil || accepted.JobID == "" {
		return "", errors.New("relay: resposta sem jobId")
	}
	return accepted.JobID, nil
}

// Callback é o corpo que o relay manda ao webhookUrl: status "success" com
// result, ou "error"/"timeout" com error.
type Callback struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
	Result string `json:"result"`
	Error  string `json:"error"`
}

// OK diz se o relay terminou com sucesso.
func (c Callback) OK() bool {
	return c.Status == "success"
}
