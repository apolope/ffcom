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

// wsAuthSubprotocol é o subprotocolo usado para carregar o access token no
// handshake de WebSocket. A API WebSocket do navegador não permite setar
// headers customizados (logo não dá pra mandar "Authorization: Bearer ..."
// na abertura da conexão) mas permite escolher subprotocolos via
// `new WebSocket(url, [wsAuthSubprotocol, token])`, que viram o header
// Sec-WebSocket-Protocol. O upgrader (ver internal/httpapi/channel_ws.go)
// precisa declarar este mesmo nome em Upgrader.Subprotocols para o
// handshake ser aceito.
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
// <token>" — usado só pela rota de WebSocket, onde o client não consegue
// setar o header Authorization (ver wsAuthSubprotocol).
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

// MemberFromContext devolve o membro autenticado anexado pelo Middleware.
func MemberFromContext(ctx context.Context) (store.Member, bool) {
	member, ok := ctx.Value(memberContextKey).(store.Member)
	return member, ok
}
