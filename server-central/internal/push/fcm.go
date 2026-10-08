// Package push entrega notificações aos aparelhos pelo Firebase Cloud
// Messaging (API HTTP v1). Ver docs/architecture.md, "Decisão: notificações
// push (fase 6)", e o formato das mensagens em docs/protocol.md,
// "Notificações push".
package push

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ErrUnregistered: o FCM diz que o token não existe mais (app desinstalado,
// dados apagados, token trocado). Quem chamou apaga o token.
var ErrUnregistered = errors.New("push: token FCM não registrado")

// Sender manda uma mensagem só de dados a um aparelho. FCMSender é a
// implementação real; os testes usam uma falsa.
type Sender interface {
	Send(ctx context.Context, deviceToken string, data map[string]string) error
}

const (
	fcmScope        = "https://www.googleapis.com/auth/firebase.messaging"
	fcmBaseURL      = "https://fcm.googleapis.com"
	defaultTokenURI = "https://oauth2.googleapis.com/token"
)

// serviceAccount são os campos usados da chave JSON da conta de serviço do
// Google (a que o console do Firebase baixa).
type serviceAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// FCMSender fala com a API HTTP v1 do FCM. O token de acesso OAuth2 sai de
// um JWT assinado aqui mesmo com a chave da conta de serviço (o mesmo
// esquema do JWT do LiveKit no server-channel, sem SDK do Google) e fica em
// cache até perto de expirar.
type FCMSender struct {
	projectID   string
	clientEmail string
	key         *rsa.PrivateKey
	tokenURI    string
	baseURL     string
	http        *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewFCMSender lê a chave JSON da conta de serviço. raw pode ser o próprio
// JSON (numa linha, como cabe no .env) ou o caminho de um arquivo com ele.
func NewFCMSender(raw string) (*FCMSender, error) {
	raw = strings.TrimSpace(raw)
	data := []byte(raw)
	if !strings.HasPrefix(raw, "{") {
		fileData, err := os.ReadFile(raw)
		if err != nil {
			return nil, fmt.Errorf("push: ler FCM_SERVICE_ACCOUNT_JSON: %w", err)
		}
		data = fileData
	}
	var sa serviceAccount
	if err := json.Unmarshal(data, &sa); err != nil {
		return nil, fmt.Errorf("push: FCM_SERVICE_ACCOUNT_JSON não é JSON válido: %w", err)
	}
	if sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("push: FCM_SERVICE_ACCOUNT_JSON sem project_id, client_email ou private_key")
	}
	key, err := parseRSAKey(sa.PrivateKey)
	if err != nil {
		return nil, err
	}
	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = defaultTokenURI
	}
	return &FCMSender{
		projectID:   sa.ProjectID,
		clientEmail: sa.ClientEmail,
		key:         key,
		tokenURI:    tokenURI,
		baseURL:     fcmBaseURL,
		http:        &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func parseRSAKey(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errors.New("push: private_key da conta de serviço não é PEM")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("push: private_key da conta de serviço não é RSA")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("push: private_key da conta de serviço inválida: %w", err)
	}
	return key, nil
}

// token devolve o token de acesso em cache ou troca um JWT novo por outro.
func (s *FCMSender) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accessToken != "" && time.Now().Before(s.expiresAt) {
		return s.accessToken, nil
	}

	now := time.Now()
	assertion, err := s.signJWT(now)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("push: pedir token OAuth2: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("push: token OAuth2 recusado (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", errors.New("push: resposta de token OAuth2 sem access_token")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	s.accessToken = out.AccessToken
	// Renova um minuto antes, para nenhum envio sair com token vencendo.
	s.expiresAt = now.Add(time.Duration(out.ExpiresIn)*time.Second - time.Minute)
	return s.accessToken, nil
}

// signJWT monta a asserção RS256 da conta de serviço (RFC 7523).
func (s *FCMSender) signJWT(now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss":   s.clientEmail,
		"scope": fcmScope,
		"aud":   s.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	signingInput := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("push: assinar JWT: %w", err)
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

// Send manda uma mensagem só de dados (sem "notification"): quem monta a
// notificação na tela é o FirebaseMessagingService do app, que agrupa por
// canal. Prioridade alta para chegar com o aparelho em repouso (Doze).
func (s *FCMSender) Send(ctx context.Context, deviceToken string, data map[string]string) error {
	accessToken, err := s.token(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"token": deviceToken,
			"data":  data,
			"android": map[string]any{
				"priority": "HIGH",
				"ttl":      "86400s",
			},
		},
	})
	if err != nil {
		return err
	}
	endpoint := s.baseURL + "/v1/projects/" + url.PathEscape(s.projectID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("push: enviar ao FCM: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if isUnregistered(resp.StatusCode, respBody) {
		return ErrUnregistered
	}
	if resp.StatusCode == http.StatusUnauthorized {
		// Token de acesso revogado antes da hora: o próximo envio pede outro.
		s.mu.Lock()
		s.accessToken = ""
		s.mu.Unlock()
	}
	return fmt.Errorf("push: FCM respondeu %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

// isUnregistered reconhece o erro do FCM para token que não existe mais:
// errorCode UNREGISTERED nos detalhes (normalmente com status 404).
func isUnregistered(status int, body []byte) bool {
	var out struct {
		Error struct {
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &out) != nil {
		return false
	}
	for _, d := range out.Error.Details {
		if d.ErrorCode == "UNREGISTERED" {
			return true
		}
	}
	return status == http.StatusNotFound && out.Error.Status == "NOT_FOUND"
}
