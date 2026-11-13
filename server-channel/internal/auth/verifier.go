// Package auth valida access tokens (Bearer JWT) emitidos pela instância
// central de Authentik — mesmo issuer usado por server-central (ver
// docs/architecture.md, "modelo de dados de server-channel"). Não há canal
// direto com server-central: cada server-channel valida o token de forma
// independente contra o mesmo issuer.
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// ErrIssuerUnavailable é devolvido por Verify quando o token é de um issuer
// configurado cujo discovery ainda não deu certo (Authentik fora do ar desde
// o boot). Não diz nada sobre o token: quem chama responde 503 com
// Retry-After, não 401, porque o client trata 401 como sessão recusada.
var ErrIssuerUnavailable = errors.New("auth: issuer OIDC indisponível no momento")

// Espera entre tentativas de discovery: começa em discoveryBackoffBase e
// dobra até discoveryBackoffMax. discoveryTimeout limita cada requisição ao
// Authentik (discovery e, depois, o JWKS), para uma conexão pendurada não
// segurar a goroutine para sempre.
const (
	discoveryBackoffBase = 2 * time.Second
	discoveryBackoffMax  = 60 * time.Second
	discoveryTimeout     = 10 * time.Second
)

// Verifier confere assinatura, emissor e expiração de um access token
// contra o JWKS publicado pelo issuer. Aceita mais de um issuer porque o
// mesmo provider do Authentik responde por dois domínios, e o "iss" do token
// é o domínio em que o login foi feito: auth.ffcom (brand do FFCom, usado
// pelo app) e authentik.abs (domínio padrão da instância, de antes da troca).
// Ver docs/architecture.md, "Decisão: autenticação em server-central".
//
// Cada issuer tem estado próprio e fica pronto sozinho, em segundo plano (ver
// NewVerifier): um issuer fora do ar não impede o boot nem atrapalha os
// tokens dos outros.
type Verifier struct {
	issuers map[string]*issuerState
}

// issuerState é um issuer de OIDC_ISSUER_URL. verifier fica nil até o
// discovery dar certo; depois disso não muda mais (o go-oidc renova o JWKS
// sozinho quando aparece um kid novo).
type issuerState struct {
	url      string
	verifier atomic.Pointer[oidc.IDTokenVerifier]
}

// ParseIssuerURLs lê OIDC_ISSUER_URL, que aceita vários issuers separados
// por vírgula.
func ParseIssuerURLs(raw string) []string {
	var urls []string
	for _, u := range strings.Split(raw, ",") {
		if u = strings.TrimSpace(u); u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

// NewVerifier prepara um verificador por issuer e dispara o discovery de
// cada um (JWKS incluso) em segundo plano, com nova tentativa até dar certo
// (ver discover). Só falha por configuração inválida (lista vazia, URL
// malformada), nunca por rede: com o Authentik fora do ar o processo sobe
// mesmo assim, o /healthz responde, e os tokens daquele issuer recebem
// ErrIssuerUnavailable até ele voltar. Antes o boot morria aqui e o container
// entrava em loop de restart (ver docs/architecture.md, "Boot tolerante a
// Authentik fora do ar"). Cancelar ctx encerra as tentativas pendentes.
//
// SkipClientIDCheck segue o mesmo raciocínio já registrado em
// server-central/internal/auth/verifier.go. O mesmo desenho de boot tolerante
// existe lá; os dois módulos são separados, então o código é duplicado.
func NewVerifier(ctx context.Context, issuerURLs []string) (*Verifier, error) {
	return newVerifier(ctx, issuerURLs, discoveryBackoff)
}

// newVerifier é NewVerifier com a espera entre tentativas injetável, para os
// testes não esperarem segundos.
func newVerifier(ctx context.Context, issuerURLs []string, backoff func(attempt int) time.Duration) (*Verifier, error) {
	if len(issuerURLs) == 0 {
		return nil, fmt.Errorf("auth: nenhum issuer OIDC configurado")
	}
	issuers := make(map[string]*issuerState, len(issuerURLs))
	for _, issuerURL := range issuerURLs {
		if err := validateIssuerURL(issuerURL); err != nil {
			return nil, err
		}
		issuers[issuerURL] = &issuerState{url: issuerURL}
	}
	// O cliente HTTP vai no contexto do discovery e o go-oidc o guarda no
	// Provider: vale também para as buscas de JWKS depois de pronto.
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: discoveryTimeout})
	for _, issuer := range issuers {
		go issuer.discover(ctx, backoff)
	}
	return &Verifier{issuers: issuers}, nil
}

// validateIssuerURL recusa o que nunca vai funcionar, por mais que se tente
// de novo: URL sem esquema http(s) ou sem host.
func validateIssuerURL(issuerURL string) error {
	u, err := url.Parse(issuerURL)
	if err != nil {
		return fmt.Errorf("auth: issuer OIDC inválido %q: %w", issuerURL, err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("auth: issuer OIDC inválido %q: precisa ser uma URL http(s) com host", issuerURL)
	}
	return nil
}

// discover busca o discovery document do issuer até dar certo, esperando
// backoff(tentativa) entre as falhas, e então publica o verificador. Uma
// linha de log por falha e uma quando fica pronto. Volta sem publicar nada
// se ctx for cancelado.
func (s *issuerState) discover(ctx context.Context, backoff func(attempt int) time.Duration) {
	for attempt := 1; ; attempt++ {
		provider, err := oidc.NewProvider(ctx, s.url)
		if err == nil {
			s.verifier.Store(provider.Verifier(&oidc.Config{SkipClientIDCheck: true}))
			log.Printf("auth: issuer OIDC %q pronto (tentativa %d)", s.url, attempt)
			return
		}
		if ctx.Err() != nil {
			return
		}
		wait := backoff(attempt)
		log.Printf("auth: discovery OIDC em %q falhou (tentativa %d), de novo em %s: %v", s.url, attempt, wait.Round(time.Millisecond), err)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// discoveryBackoff é a espera antes da tentativa attempt+1: teto
// discoveryBackoffBase·2^(attempt-1), limitado a discoveryBackoffMax, com
// metade fixa e metade aleatória, para as réplicas não baterem no Authentik
// no mesmo instante quando ele volta.
func discoveryBackoff(attempt int) time.Duration {
	ceiling := discoveryBackoffMax
	if attempt < 8 {
		ceiling = min(discoveryBackoffBase<<(attempt-1), discoveryBackoffMax)
	}
	return ceiling/2 + rand.N(ceiling/2+1)
}

// unverifiedIssuer lê o "iss" do payload sem validar nada, só para escolher
// o verificador; quem confere assinatura e o próprio "iss" é o go-oidc.
func unverifiedIssuer(rawToken string) (string, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("auth: token malformado")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("auth: payload do token: %w", err)
	}
	var claims struct {
		Issuer string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("auth: payload do token: %w", err)
	}
	return claims.Issuer, nil
}

// Claims são os campos do access token que server-channel de fato usa.
type Claims struct {
	Subject string `json:"sub"`
	// SessionID é o "sid" que o Authentik põe no access token: hash da
	// sessão de login do navegador/app que pediu o token. Cada dispositivo
	// loga separado e tem o seu; abas do mesmo navegador dividem o mesmo.
	// Usado só pelo rate limit, que conta por usuário+dispositivo (ver
	// docs/rate-limits.md). Pode vir vazio (token emitido sem sessão), e aí
	// o rate limit cai para o balde só do usuário.
	SessionID string `json:"sid"`
}

// Verify valida rawToken e devolve as claims relevantes. Token de issuer
// configurado mas ainda sem discovery devolve um erro que casa com
// ErrIssuerUnavailable (errors.Is).
func (v *Verifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	issuer, err := unverifiedIssuer(rawToken)
	if err != nil {
		return Claims{}, err
	}
	state, ok := v.issuers[issuer]
	if !ok {
		return Claims{}, fmt.Errorf("auth: token de issuer não aceito: %q", issuer)
	}
	verifier := state.verifier.Load()
	if verifier == nil {
		return Claims{}, fmt.Errorf("%w: %q", ErrIssuerUnavailable, issuer)
	}
	token, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return Claims{}, fmt.Errorf("auth: verificar token: %w", err)
	}

	var claims Claims
	if err := token.Claims(&claims); err != nil {
		return Claims{}, fmt.Errorf("auth: extrair claims do token: %w", err)
	}
	if claims.Subject == "" {
		return Claims{}, fmt.Errorf("auth: token sem claim 'sub'")
	}
	return claims, nil
}
