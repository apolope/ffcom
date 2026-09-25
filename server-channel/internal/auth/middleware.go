package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

type contextKey int

const (
	subjectContextKey contextKey = iota
	memberContextKey
)

// VerifyToken exige um Bearer token válido, sem exigir que o "sub" já seja
// membro deste server-channel — usado por POST /api/join (que precisa
// autenticar antes de saber se o "sub" ainda vai entrar) e, encadeado com
// RequireMember, por toda rota que exige associação de fato (ver
// docs/architecture.md, "Convites obrigatórios para entrar em
// server-channel").
func VerifyToken(verifier *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// O rate limit (internal/httpapi/ratelimit.go) já verificou o
			// token para descobrir de quem é a requisição; não verifica de
			// novo.
			if _, ok := SubjectFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}

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

			ctx := context.WithValue(r.Context(), subjectContextKey, claims.Subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IdentifyRequest verifica o token da requisição, se houver, e devolve o
// "sub" junto com a requisição já com o "sub" no contexto (VerifyToken, mais
// adiante na cadeia, reaproveita em vez de verificar de novo). Sem token ou
// com token inválido devolve ok=false e a requisição intacta: quem decide
// o 401 continua sendo VerifyToken. Usado pelo rate limit, que conta por
// usuário e só cai para IP quando não sabe quem é (ver docs/rate-limits.md).
func IdentifyRequest(verifier *Verifier, r *http.Request) (*http.Request, string, bool) {
	rawToken, ok := bearerToken(r)
	if !ok {
		return r, "", false
	}
	claims, err := verifier.Verify(r.Context(), rawToken)
	if err != nil {
		return r, "", false
	}
	ctx := context.WithValue(r.Context(), subjectContextKey, claims.Subject)
	return r.WithContext(ctx), claims.Subject, true
}

// RequireMember exige que o "sub" autenticado (anexado por VerifyToken, que
// precisa vir antes na cadeia) já tenha entrado neste server-channel via
// POST /api/join. Ao contrário do comportamento antigo, não cria o membro
// implicitamente: um "sub" válido no Authentik central mas que nunca resgatou
// um convite (ou não foi o primeiro a entrar, ver handleJoin) recebe 403.
func RequireMember(members *store.MemberStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject, ok := SubjectFromContext(r.Context())
			if !ok {
				http.Error(w, "subject ausente no contexto", http.StatusInternalServerError)
				return
			}

			member, err := members.GetByOIDCSubject(r.Context(), subject)
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, "é preciso entrar neste servidor (POST /api/join) antes", http.StatusForbidden)
				return
			}
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

// SubjectFromContext devolve o "sub" anexado pelo VerifyToken.
func SubjectFromContext(ctx context.Context) (string, bool) {
	subject, ok := ctx.Value(subjectContextKey).(string)
	return subject, ok
}

// MemberFromContext devolve o membro autenticado anexado pelo RequireMember.
func MemberFromContext(ctx context.Context) (store.Member, bool) {
	member, ok := ctx.Value(memberContextKey).(store.Member)
	return member, ok
}

// WithMember anexa member ao contexto do mesmo jeito que RequireMember.
// Usado pelos testes de handler em internal/httpapi, que não passam por um
// token OIDC real.
func WithMember(ctx context.Context, member store.Member) context.Context {
	return context.WithValue(ctx, memberContextKey, member)
}
