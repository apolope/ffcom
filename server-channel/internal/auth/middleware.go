package auth

import (
	"context"
	"net/http"
	"strings"

	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

type contextKey int

const memberContextKey contextKey = iota

// Middleware exige um Bearer token válido em cada requisição e garante (via
// MemberStore.GetOrCreateByOIDCSubject, que já faz upsert por "sub") que o
// membro local exista, anexando-o ao contexto da requisição. Não há
// endpoint de "entrar no servidor" separado: a primeira requisição
// autenticada de um "sub" novo já cria o membro.
func Middleware(verifier *Verifier, members *store.MemberStore) func(http.Handler) http.Handler {
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

			member, err := members.GetOrCreateByOIDCSubject(r.Context(), claims.Subject)
			if err != nil {
				http.Error(w, "erro ao resolver membro", http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), memberContextKey, member)
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

// MemberFromContext devolve o membro autenticado anexado pelo Middleware.
func MemberFromContext(ctx context.Context) (store.Member, bool) {
	member, ok := ctx.Value(memberContextKey).(store.Member)
	return member, ok
}
