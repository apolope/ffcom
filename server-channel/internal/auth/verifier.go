// Package auth valida access tokens (Bearer JWT) emitidos pela instância
// central de Authentik — mesmo issuer usado por server-central (ver
// docs/architecture.md, "modelo de dados de server-channel"). Não há canal
// direto com server-central: cada server-channel valida o token de forma
// independente contra o mesmo issuer.
package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Verifier confere assinatura, emissor e expiração de um access token
// contra o JWKS publicado pelo issuer.
type Verifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewVerifier busca o discovery document do issuer (JWKS incluso) e prepara
// o verificador. SkipClientIDCheck segue o mesmo raciocínio já registrado em
// server-central/internal/auth/verifier.go.
func NewVerifier(ctx context.Context, issuerURL string) (*Verifier, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("auth: descobrir provider OIDC em %q: %w", issuerURL, err)
	}

	return &Verifier{
		verifier: provider.Verifier(&oidc.Config{SkipClientIDCheck: true}),
	}, nil
}

// Claims são os campos do access token que server-channel de fato usa.
type Claims struct {
	Subject string `json:"sub"`
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
