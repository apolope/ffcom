package auth

import (
	"context"
	"net/http"
	"strings"

	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

type contextKey int

const accountContextKey contextKey = iota

// Middleware exige um Bearer token válido em cada requisição e garante (via
// AccountStore.GetOrCreateBySubject, que já faz upsert por "sub") que a
// conta local exista, anexando-a ao contexto da requisição. Não há endpoint
// de "cadastro" separado: a primeira requisição autenticada de um "sub"
// novo já cria a conta.
func Middleware(verifier *Verifier, accounts *store.AccountStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken, ok := bearerToken(r)
			if !ok {
				http.Error(w, "token ausente ou mal formatado", http.StatusUnauthorized)
				return
			}

			claims, err := verifier.Verify(r.Context(), rawToken)
			if err != nil {
				http.Error(w, "token inválido", http.StatusUnauthorized)
				return
			}

			account, err := accounts.GetOrCreateBySubject(r.Context(), claims.Subject)
			if err != nil {
				http.Error(w, "erro ao resolver conta", http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), accountContextKey, account)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
