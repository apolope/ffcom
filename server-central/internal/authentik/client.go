// Package authentik cria contas na instância central de Authentik pela API
// de administração (/api/v3), usada na aprovação dos pedidos de cadastro:
// cria o usuário sem senha, põe no grupo do FFCom e pede ao Authentik o
// e-mail de recuperação, que é por onde a pessoa escolhe a senha. O token é
// de uma conta de serviço com permissão só para isso. Ver
// docs/architecture.md, "Decisão: cadastro com aprovação pelo Telegram".
// O idioma do e-mail sai do Accept-Language da chamada (ver
// SendRecoveryEmail).
package authentik

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUserExists é devolvido quando já há no Authentik um usuário com o nome
// de usuário ou o e-mail pedidos, criado por outro caminho.
var ErrUserExists = errors.New("authentik: usuário já existe")

// Client fala com a API do Authentik. O zero value não serve: use New.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New devolve um Client para a instância em baseURL (ex.
// https://authentik.abs.a3sitsolutions.com.br), autenticado por token.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/") + "/api/v3",
		token:   token,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// User é o recorte do usuário do Authentik que o FFCom usa.
type User struct {
	PK         int64          `json:"pk"`
	Username   string         `json:"username"`
	Email      string         `json:"email"`
	IsActive   bool           `json:"is_active"`
	Type       string         `json:"type"`
	Groups     []string       `json:"groups"`
	Attributes map[string]any `json:"attributes"`
}

// IsServiceAccount diz se a conta é de serviço (token de API, outpost),
// que não é de pessoa e não deve ganhar acesso ao FFCom.
func (u User) IsServiceAccount() bool {
	return u.Type == "service_account" || u.Type == "internal_service_account"
}

// InGroup diz se o usuário já está no grupo groupUUID.
func (u User) InGroup(groupUUID string) bool {
	for _, g := range u.Groups {
		if g == groupUUID {
			return true
		}
	}
	return false
}

// NewUser são os dados de um usuário a criar.
type NewUser struct {
	Username string
	// Name é o nome exibido do Authentik (claim name), que o FFCom mostra
	// em todo lugar; no cadastro vai o apelido escolhido.
	Name       string
	Email      string
	Attributes map[string]any
}

// FindUsername procura o usuário com o nome de usuário username sem
// diferenciar maiúsculas. O Authentik só barra nome repetido idêntico
// (unicidade do Django), então uma conta "Joao" criada pela UI conviveria
// com um "joao" novo, e as duas se confundiriam no FFCom. Devolve nil sem
// erro se não houver.
func (c *Client) FindUsername(ctx context.Context, username string) (*User, error) {
	users, err := c.search(ctx, username)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if strings.EqualFold(u.Username, username) {
			return &u, nil
		}
	}
	return nil, nil
}

// FindByEmail devolve todos os usuários com o e-mail email, sem
// diferenciar maiúsculas. No Authentik o e-mail não é único: a mesma pessoa
// pode ter mais de uma conta.
func (c *Client) FindByEmail(ctx context.Context, email string) ([]User, error) {
	users, err := c.search(ctx, email)
	if err != nil {
		return nil, err
	}
	var out []User
	for _, u := range users {
		if strings.EqualFold(u.Email, email) {
			out = append(out, u)
		}
	}
	return out, nil
}

// search usa a busca livre da lista de usuários, que ignora maiúsculas
// (os filtros por campo, como ?username=, são exatos). Ela também casa
// trechos de nome e atributos, então quem chama filtra o resultado.
func (c *Client) search(ctx context.Context, term string) ([]User, error) {
	var page struct {
		Results []User `json:"results"`
	}
	q := url.Values{"search": {term}, "page_size": {"100"}}
	if err := c.do(ctx, http.MethodGet, "/core/users/?"+q.Encode(), nil, &page); err != nil {
		return nil, err
	}
	return page.Results, nil
}

// GroupUUID devolve o id do grupo name.
func (c *Client) GroupUUID(ctx context.Context, name string) (string, error) {
	var page struct {
		Results []struct {
			PK   string `json:"pk"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := c.do(ctx, http.MethodGet, "/core/groups/?"+url.Values{"name": {name}}.Encode(), nil, &page); err != nil {
		return "", err
	}
	for _, g := range page.Results {
		if g.Name == name {
			return g.PK, nil
		}
	}
	return "", fmt.Errorf("authentik: grupo %q não encontrado", name)
}

// CreateUser cria um usuário ativo, sem senha, no path "users".
func (c *Client) CreateUser(ctx context.Context, n NewUser) (User, error) {
	body := map[string]any{
		"username":   n.Username,
		"name":       n.Name,
		"email":      n.Email,
		"is_active":  true,
		"path":       "users",
		"attributes": n.Attributes,
	}
	var u User
	if err := c.do(ctx, http.MethodPost, "/core/users/", body, &u); err != nil {
		return User{}, err
	}
	return u, nil
}

// AddToGroup põe o usuário no grupo (repetir não é erro).
func (c *Client) AddToGroup(ctx context.Context, groupUUID string, userPK int64) error {
	return c.do(ctx, http.MethodPost, "/core/groups/"+url.PathEscape(groupUUID)+"/add_user/", map[string]any{"pk": userPK}, nil)
}

// SendRecoveryEmail pede ao Authentik o e-mail com o link de definir senha,
// pelo stage de e-mail emailStage, com o link valendo tokenDuration
// (formato do Authentik, ex. "days=3"; vazio usa o padrão do stage), no
// idioma language (pt-BR ou en; vazio usa pt-BR).
//
// O idioma vai no Accept-Language desta chamada, não no settings.locale do
// usuário: o Authentik traduz o e-mail com o idioma ativo na requisição, e o
// LocaleMiddleware dele sempre fixa um (cookie, Accept-Language ou en-us),
// então User.locale() nunca chega a ler settings.locale numa chamada HTTP.
// Ver docs/architecture.md, "Decisão: internacionalização".
func (c *Client) SendRecoveryEmail(ctx context.Context, userPK int64, emailStage, tokenDuration, language string) error {
	body := map[string]any{"email_stage": emailStage}
	if tokenDuration != "" {
		body["token_duration"] = tokenDuration
	}
	if language == "" {
		language = DefaultLanguage
	}
	return c.doLang(ctx, http.MethodPost, fmt.Sprintf("/core/users/%d/recovery_email/", userPK), body, nil, language)
}

// DefaultLanguage é o idioma dos e-mails quando não há um escolhido.
const DefaultLanguage = "pt-BR"

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	return c.doLang(ctx, method, path, body, out, "")
}

// doLang é do com Accept-Language: language (vazio não manda o header).
func (c *Client) doLang(ctx context.Context, method, path string, body, out any, language string) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if language != "" {
		req.Header.Set("Accept-Language", language)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("authentik: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail := strings.TrimSpace(string(raw))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		if method == http.MethodPost && path == "/core/users/" && resp.StatusCode == http.StatusBadRequest && strings.Contains(detail, "username") {
			return fmt.Errorf("%w: %s", ErrUserExists, detail)
		}
		return fmt.Errorf("authentik: %s %s: %d %s", method, path, resp.StatusCode, detail)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("authentik: %s %s: resposta fora do formato: %w", method, path, err)
		}
	}
	return nil
}
