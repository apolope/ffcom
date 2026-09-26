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
	groupsContextKey
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
			groups, _ := r.Context().Value(groupsContextKey).([]string)
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
				groups = claims.Groups
			}

			account, err := accounts.GetOrCreateBySubject(r.Context(), subject, profileName)
			if err != nil {
				http.Error(w, "erro ao resolver conta", http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(r.Context(), accountContextKey, account)
			ctx = context.WithValue(ctx, groupsContextKey, groups)
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
	ctx = context.WithValue(ctx, groupsContextKey, claims.Groups)
	return r.WithContext(ctx), claims.Subject, claims.SessionID, true
}

// InGroupFromContext diz se o token da requisição trazia o grupo name (ver
// Claims.Groups). Falso para requisição sem token.
func InGroupFromContext(ctx context.Context, name string) bool {
	groups, _ := ctx.Value(groupsContextKey).([]string)
	for _, g := range groups {
		if g == name {
			return true
		}
	}
	return false
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

// OptionalAccount é o Middleware para rotas públicas que mudam de acordo com
// quem pede (ex. a lista de ideias mostra o voto de quem está logado): com
// token válido anexa a conta ao contexto, sem token (ou com token inválido)
// segue como visitante, sem 401.
func OptionalAccount(verifier *Verifier, accounts *store.AccountStore) func(http.Handler) http.Handler {
	required := Middleware(verifier, accounts)
	return func(next http.Handler) http.Handler {
		withAccount := required(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := r.Context().Value(subjectContextKey).(string); ok {
				withAccount.ServeHTTP(w, r)
				return
			}
			if rawToken, hasToken := bearerToken(r); hasToken {
				if _, err := verifier.Verify(r.Context(), rawToken); err == nil {
					withAccount.ServeHTTP(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AccountFromContext devolve a conta autenticada anexada pelo Middleware.
func AccountFromContext(ctx context.Context) (store.Account, bool) {
	account, ok := ctx.Value(accountContextKey).(store.Account)
	return account, ok
}

// WithAccount anexa account (e os grupos do token) ao contexto, como o
// Middleware faria. Usado pelos testes dos handlers, que não têm um
// Authentik para emitir token.
func WithAccount(ctx context.Context, account store.Account, groups ...string) context.Context {
	ctx = context.WithValue(ctx, accountContextKey, account)
	return context.WithValue(ctx, groupsContextKey, groups)
}
