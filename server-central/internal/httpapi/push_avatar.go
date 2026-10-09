package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
	"a3sitsolutions.com/ffcom/server-central/internal/storage"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// Avatar de quem mandou, nas notificações push do app Android. Ver
// docs/architecture.md, "Decisão: notificações push (fase 6)", "Avatar na
// notificação".
//
// O serviço de push do app roda sem a WebView e sem o token de login, então
// não consegue chamar GET /api/avatars/{id} (Bearer). A notificação leva um
// link GET /api/push/avatars/{token} sem login: o token tem 256 bits
// aleatórios (crypto/rand) e o banco guarda só o SHA-256. O link vale para
// quantas leituras forem (o FCM pode entregar a mesma mensagem a vários
// aparelhos, e o app pode baixar de novo) e só vence por tempo.

const (
	// pushAvatarLinkTTL é a validade de um link. O FCM guarda a mensagem não
	// entregue por até 24 h (ttl em fcm.go); com 48 h, e reaproveitando só
	// link com mais de 24 h pela frente, todo link que sai numa notificação
	// ainda vale quando ela chega, mesmo entregue no último minuto.
	pushAvatarLinkTTL = 48 * time.Hour
	// pushAvatarLinkMinRemaining: abaixo disso um link novo é emitido.
	pushAvatarLinkMinRemaining = 24 * time.Hour

	pushAvatarsPathPrefix = "/api/push/avatars/"
	maxPushAvatarToken    = 64
	maxAuthorSubject      = 255
)

// PushAvatarLinks emite e reaproveita os links de avatar das notificações
// (push.AuthorAvatars). O token em claro de cada link ainda útil fica só
// em memória, por conta, para reaproveitar enquanto tiver mais de 24 h;
// depois de reiniciar o processo, o primeiro push de cada pessoa emite um
// link novo (os antigos continuam valendo até vencer).
type PushAvatarLinks struct {
	store   *store.PushStore
	baseURL string
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]cachedAvatarLink
}

type cachedAvatarLink struct {
	token     string
	expiresAt time.Time
}

// NewPushAvatarLinks monta os links a partir de publicURL, o endereço
// público deste server-central (CENTRAL_PUBLIC_URL).
func NewPushAvatarLinks(pushStore *store.PushStore, publicURL string) *PushAvatarLinks {
	return &PushAvatarLinks{
		store:   pushStore,
		baseURL: strings.TrimRight(strings.TrimSpace(publicURL), "/"),
		now:     time.Now,
		cache:   make(map[string]cachedAvatarLink),
	}
}

// ForAccount devolve o link do avatar de accountID e a chave de cache do
// app, ou url vazia se a conta não tem avatar (ou se algo falhou: a
// notificação sai com a letra).
func (l *PushAvatarLinks) ForAccount(ctx context.Context, accountID string) (url, key string) {
	avatar, err := l.store.AvatarByAccountID(ctx, accountID)
	return l.forAvatar(ctx, avatar, err)
}

// ForSubject é ForAccount a partir do "sub" do Authentik que o
// server-channel conhece do autor. Subject sem conta ou sem avatar: vazio.
func (l *PushAvatarLinks) ForSubject(ctx context.Context, subject string) (url, key string) {
	if subject == "" || len(subject) > maxAuthorSubject {
		return "", ""
	}
	avatar, err := l.store.AvatarBySubject(ctx, subject)
	return l.forAvatar(ctx, avatar, err)
}

func (l *PushAvatarLinks) forAvatar(ctx context.Context, avatar store.PushAvatar, err error) (string, string) {
	if errors.Is(err, store.ErrNotFound) {
		return "", ""
	}
	if err != nil {
		log.Printf("server-central: push: avatar do autor: %v", err)
		return "", ""
	}
	token, err := l.token(ctx, avatar.AccountID)
	if err != nil {
		log.Printf("server-central: push: link do avatar: %v", err)
		return "", ""
	}
	return l.baseURL + pushAvatarsPathPrefix + token, pushAvatarKey(avatar)
}

// token devolve um token de link ainda útil da conta, ou emite outro.
func (l *PushAvatarLinks) token(ctx context.Context, accountID string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if c, ok := l.cache[accountID]; ok && c.expiresAt.Sub(now) > pushAvatarLinkMinRemaining {
		return c.token, nil
	}
	token, err := newPushAvatarToken()
	if err != nil {
		return "", err
	}
	expiresAt := now.Add(pushAvatarLinkTTL)
	if err := l.store.CreateAvatarLink(ctx, hashGrant(token), accountID, expiresAt); err != nil {
		return "", err
	}
	l.cache[accountID] = cachedAvatarLink{token: token, expiresAt: expiresAt}
	return token, nil
}

// Purge apaga os links vencidos do banco e esquece os de memória que já
// não seriam reaproveitados.
func (l *PushAvatarLinks) Purge(ctx context.Context) {
	if n, err := l.store.PurgeExpiredAvatarLinks(ctx); err != nil {
		log.Printf("server-central: push: limpar links de avatar: %v", err)
	} else if n > 0 {
		log.Printf("server-central: push: %d links de avatar vencidos apagados", n)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for id, c := range l.cache {
		if c.expiresAt.Sub(now) <= pushAvatarLinkMinRemaining {
			delete(l.cache, id)
		}
	}
}

// RunPurge chama Purge a cada interval, até ctx acabar.
func (l *PushAvatarLinks) RunPurge(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.Purge(ctx)
		}
	}
}

func newPushAvatarToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// pushAvatarKey é o authorKey: estável enquanto o avatar não muda (a
// versão é o updated_at do perfil) e sem nada secreto. O app guarda o
// avatar baixado com essa chave, para não baixar de novo a cada mensagem.
func pushAvatarKey(avatar store.PushAvatar) string {
	sum := sha256.Sum256([]byte("ffcom-push-avatar\x00" + avatar.AccountID + "\x00" + strconv.FormatInt(avatar.UpdatedAt.UnixMicro(), 10)))
	return hex.EncodeToString(sum[:16])
}

// GET /api/push/avatars/{token}: os bytes do avatar da conta do link, sem
// login. Link vencido e desconhecido respondem o mesmo 404. Ler não
// consome o link.
func handleGetPushAvatar(pushStore *store.PushStore, files *storage.AvatarStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notFound := func() {
			apierr.Write(w, http.StatusNotFound, "push.avatar_not_found", "link de avatar inválido ou vencido")
		}
		token := r.PathValue("token")
		if token == "" || len(token) > maxPushAvatarToken {
			notFound()
			return
		}
		accountID, expiresAt, err := pushStore.AvatarLinkAccount(r.Context(), hashGrant(token))
		if errors.Is(err, store.ErrNotFound) {
			notFound()
			return
		}
		if err != nil {
			log.Printf("server-central: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "push.avatar_read_failed", "erro ao ler o avatar")
			return
		}

		file, err := files.Open(accountID)
		if errors.Is(err, fs.ErrNotExist) {
			// Avatar removido depois de o link sair: igual a link inválido.
			notFound()
			return
		}
		if err != nil {
			log.Printf("server-central: erro ao abrir avatar %s: %v", accountID, err)
			apierr.Write(w, http.StatusInternalServerError, "push.avatar_read_failed", "erro ao ler o avatar")
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "push.avatar_read_failed", "erro ao ler o avatar")
			return
		}

		// O tipo não é gravado à parte (ver handleGetAvatar): sai dos
		// primeiros bytes, e o upload só aceita PNG, JPEG, WebP e GIF.
		head := make([]byte, 512)
		n, _ := io.ReadFull(file, head)
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			apierr.Write(w, http.StatusInternalServerError, "push.avatar_read_failed", "erro ao ler o avatar")
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(head[:n]))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		remaining := int(time.Until(expiresAt).Seconds())
		w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(max(remaining, 0)))
		http.ServeContent(w, r, "", info.ModTime(), file)
	})
}
