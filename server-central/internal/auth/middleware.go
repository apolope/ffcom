package auth

import (
	"context"
	"net/http"
	"strings"

	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

type contextKey int

const (
	accountContextKey contextKey = iota
	subjectContextKey
	sessionContextKey
	profileNameContextKey
)

// Middleware exige um Bearer token válido em cada requisição e garante (via
// AccountStore.GetOrCreateBySubject, que já faz upsert por "sub") que a
// conta local exista, anexando-a ao contexto da requisição. Não há endpoint
// de "cadastro" separado: a primeira requisição autenticada de um "sub"
// novo já cria a conta.
func Middleware(verifier *Verifier, accounts *store.AccountStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// O rate limit (internal/httpapi/ratelimit.go) já verificou o
			// token para descobrir de quem é a requisição; não verifica de
			// novo.
			subject, ok := r.Context().Value(subjectContextKey).(string)
			profileName, _ := r.Context().Value(profileNameContextKey).(*string)
			if !ok {
				rawToken, hasToken := bearerToken(r)
				if !hasToken {
					http.Error(w, "token ausente ou mal formatado", http.StatusUnauthorized)
					return
				}

				claims, err := verifier.Verify(r.Context(), rawToken)
				if err != nil {
					http.Error(w, "token inválido", http.StatusUnauthorized)
					return
				}
				subject = claims.Subject
				profileName = claims.ProfileName()
			}

			account, err := accounts.GetOrCreateBySubject(r.Context(), subject, profileName)
			if err != nil {
				http.Error(w, "erro ao resolver conta", http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), accountContextKey, account)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IdentifyRequest verifica o token da requisição, se houver, e devolve o
// "sub" e o "sid" (sessão do dispositivo) junto com a requisição já com os
// dois no contexto (Middleware, mais
// adiante na cadeia, reaproveita em vez de verificar de novo). Sem token ou
// com token inválido devolve ok=false e a requisição intacta: quem decide
// o 401 continua sendo Middleware. Usado pelo rate limit, que conta por
// usuário+dispositivo e só cai para IP quando não sabe quem é (ver
// docs/rate-limits.md).
func IdentifyRequest(verifier *Verifier, r *http.Request) (*http.Request, string, string, bool) {
	rawToken, ok := bearerToken(r)
	if !ok {
		return r, "", "", false
	}
	claims, err := verifier.Verify(r.Context(), rawToken)
	if err != nil {
		return r, "", "", false
	}
	ctx := context.WithValue(r.Context(), subjectContextKey, claims.Subject)
	ctx = context.WithValue(ctx, sessionContextKey, claims.SessionID)
	ctx = context.WithValue(ctx, profileNameContextKey, claims.ProfileName())
	return r.WithContext(ctx), claims.Subject, claims.SessionID, true
}

// SessionFromContext devolve o "sid" do token anexado por IdentifyRequest
// (vazio se o token não trouxe "sid" ou a requisição não passou por ali).
func SessionFromContext(ctx context.Context) string {
	session, _ := ctx.Value(sessionContextKey).(string)
	return session
}

// wsAuthSubprotocol é o subprotocolo usado para carregar o access token no
// handshake de WebSocket (rota de presença) — a API WebSocket do navegador
// não permite setar headers customizados, então o client manda o token via
// `new WebSocket(url, [wsAuthSubprotocol, token])`, que vira o header
// Sec-WebSocket-Protocol. Mesmo mecanismo já usado em server-channel (ver
// docs/architecture.md, "Decisão: canal de texto em server-channel"). O
// upgrader (ver internal/httpapi/presence.go) precisa declarar este mesmo
// nome em Upgrader.Subprotocols para o handshake ser aceito.
const wsAuthSubprotocol = "access_token"

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if strings.HasPrefix(header, prefix) {
		token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
		if token != "" {
			return token, true
		}
	}
	return wsProtocolToken(r)
}

// wsProtocolToken extrai o token de "Sec-WebSocket-Protocol: access_token,
// <token>" — usado só pela rota de WebSocket de presença.
func wsProtocolToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Sec-WebSocket-Protocol")
	if header == "" {
		return "", false
	}
	parts := strings.Split(header, ",")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) != wsAuthSubprotocol {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	return token, token != ""
}

// AccountFromContext devolve a conta autenticada anexada pelo Middleware.
func AccountFromContext(ctx context.Context) (store.Account, bool) {
	account, ok := ctx.Value(accountContextKey).(store.Account)
	return account, ok
}
