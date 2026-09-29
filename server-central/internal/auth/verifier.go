// Package auth valida access tokens (Bearer JWT) emitidos pela instância
// central de Authentik. server-central é Resource Server puro — não guarda
// client_secret nem faz o fluxo de login em si (isso é do client, ver
// docs/architecture.md).
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Verifier confere assinatura, emissor e expiração de um access token
// contra o JWKS publicado pelo issuer. Aceita mais de um issuer porque o
// mesmo provider do Authentik responde por dois domínios, e o "iss" do token
// é o domínio em que o login foi feito: auth.ffcom (brand do FFCom, usado
// pelo app) e authentik.abs (domínio padrão da instância, de antes da troca).
// Ver docs/architecture.md, "Decisão: autenticação em server-central".
type Verifier struct {
	verifiers map[string]*oidc.IDTokenVerifier
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

// NewVerifier busca o discovery document de cada issuer (JWKS incluso) e
// prepara os verificadores. A checagem de audiência (client_id) é desligada de
// propósito: o access token do Authentik não é escopado por client_id de
// forma diferente do padrão já usado pelos backends Spring da organização
// (issuer-uri valida iss/exp/assinatura, não aud — ver
// D:\Dev3s-network\docs\procedures\integrar-app-com-authentik.md).
func NewVerifier(ctx context.Context, issuerURLs []string) (*Verifier, error) {
	if len(issuerURLs) == 0 {
		return nil, fmt.Errorf("auth: nenhum issuer OIDC configurado")
	}
	verifiers := make(map[string]*oidc.IDTokenVerifier, len(issuerURLs))
	for _, issuerURL := range issuerURLs {
		provider, err := oidc.NewProvider(ctx, issuerURL)
		if err != nil {
			return nil, fmt.Errorf("auth: descobrir provider OIDC em %q: %w", issuerURL, err)
		}
		verifiers[issuerURL] = provider.Verifier(&oidc.Config{SkipClientIDCheck: true})
	}
	return &Verifier{verifiers: verifiers}, nil
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

// Claims são os campos do access token que server-central de fato usa.
type Claims struct {
	Subject string `json:"sub"`
	// SessionID é o "sid" que o Authentik põe no access token: hash da
	// sessão de login do navegador/app que pediu o token. Cada dispositivo
	// loga separado e tem o seu; abas do mesmo navegador dividem o mesmo.
	// Usado só pelo rate limit, que conta por usuário+dispositivo (ver
	// docs/rate-limits.md). Pode vir vazio (token emitido sem sessão), e aí
	// o rate limit cai para o balde só do usuário.
	SessionID string `json:"sid"`
	// Name e PreferredUsername vêm do scope profile. ProfileName escolhe o
	// nome exibido de quem não preencheu um no perfil do FFCom.
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	// Groups vem do scope ffcom-groups (mapeamento próprio do FFCom no
	// Authentik, que só devolve os grupos ffcom-*). Hoje só decide quem
	// modera as sugestões da home (grupo ffcom-admins); tokens pedidos sem
	// esse scope, como os do app, chegam sem a claim.
	Groups []string `json:"groups"`
}

// InGroup diz se o token traz o grupo name.
func (c Claims) InGroup(name string) bool {
	for _, g := range c.Groups {
		if g == name {
			return true
		}
	}
	return false
}

// ProfileName devolve o nome do perfil do Authentik (name, ou
// preferred_username sem ele), ou nil se o token não trouxer nenhum dos dois.
// Mesmo critério do client ao gravar o nome em server-channel.
func (c Claims) ProfileName() *string {
	for _, name := range []string{c.Name, c.PreferredUsername} {
		if name = strings.TrimSpace(name); name != "" {
			return &name
		}
	}
	return nil
}

// Verify valida rawToken e devolve as claims relevantes.
func (v *Verifier) Verify(ctx context.Context, rawToken string) (Claims, error) {
	issuer, err := unverifiedIssuer(rawToken)
	if err != nil {
		return Claims{}, err
	}
	verifier, ok := v.verifiers[issuer]
	if !ok {
		return Claims{}, fmt.Errorf("auth: token de issuer não aceito: %q", issuer)
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
