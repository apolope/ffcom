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

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}

// AccountFromContext devolve a conta autenticada anexada pelo Middleware.
func AccountFromContext(ctx context.Context) (store.Account, bool) {
	account, ok := ctx.Value(accountContextKey).(store.Account)
	return account, ok
}
