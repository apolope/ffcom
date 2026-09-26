// Package auth valida access tokens (Bearer JWT) emitidos pela instância
// central de Authentik. server-central é Resource Server puro — não guarda
// client_secret nem faz o fluxo de login em si (isso é do client, ver
// docs/architecture.md).
package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Verifier confere assinatura, emissor e expiração de um access token
// contra o JWKS publicado pelo issuer.
type Verifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewVerifier busca o discovery document do issuer (JWKS incluso) e prepara
// o verificador. A checagem de audiência (client_id) é desligada de
// propósito: o access token do Authentik não é escopado por client_id de
// forma diferente do padrão já usado pelos backends Spring da organização
// (issuer-uri valida iss/exp/assinatura, não aud — ver
// D:\Dev\a3s-network\docs\procedures\integrar-app-com-authentik.md).
func NewVerifier(ctx context.Context, issuerURL string) (*Verifier, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("auth: descobrir provider OIDC em %q: %w", issuerURL, err)
	}

	return &Verifier{
		verifier: provider.Verifier(&oidc.Config{SkipClientIDCheck: true}),
	}, nil
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
	token, err := v.verifier.Verify(ctx, rawToken)
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
