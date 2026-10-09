package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/push"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

// Notificações push do app Android. Ver docs/architecture.md, "Decisão:
// notificações push (fase 6)", e docs/protocol.md, "Notificações push".
//
// Confiança: a pessoa pede ao server-central um grant para um servidor da
// lista dela e o entrega a esse server-channel. O server-channel só
// consegue notificar quem lhe deu um grant, e o grant só vale para o
// endereço em que foi emitido. Tirar o servidor da lista apaga os grants
// (ON DELETE CASCADE em push_grants).

const (
	// pushNotifyPath fica fora do rate limit geral (é chamada por
	// server-channel, sem conta) e tem limites próprios, ver pushLimits.
	pushNotifyPath = "/api/push/notify"

	maxPushTokenLength     = 4096
	maxServerAddressLength = 512
	maxGrantTokenLength    = 128
	maxGrantsPerNotify     = 500
	maxNotifyBodyBytes     = 256 << 10
)

// pushLimits são os rate limits de POST /api/push/notify: por IP de quem
// chama (cada requisição), por servidor (cada notificação entregue, contada
// no endereço do grant) e por grant (cada notificação a uma pessoa). Os de
// servidor e grant só contam grants válidos, então um terceiro mandando
// lixo em nome de um endereço não gasta o orçamento do servidor de verdade.
type pushLimits struct {
	perIP     *rateLimiter
	perServer *rateLimiter
	perGrant  *rateLimiter
}

func newPushLimits() pushLimits {
	return pushLimits{
		perIP:     newRateLimiter(600, 200),
		perServer: newRateLimiter(1200, 400),
		perGrant:  newRateLimiter(60, 20),
	}
}

// PUT /api/push/devices: registra o token FCM do aparelho na conta.
func handleRegisterPushDevice(pushStore *store.PushStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}
		var body pushDeviceRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		body.Token = strings.TrimSpace(body.Token)
		if body.Token == "" || len(body.Token) > maxPushTokenLength {
			apierr.Write(w, http.StatusBadRequest, "push.device_token_invalid", "token do aparelho inválido")
			return
		}
		if body.Platform == "" {
			body.Platform = "android"
		}
		if body.Platform != "android" {
			apierr.Write(w, http.StatusBadRequest, "push.platform_invalid", "plataforma não suportada")
			return
		}
		if err := pushStore.UpsertDevice(r.Context(), account.ID, body.Token, body.Platform); err != nil {
			log.Printf("server-central: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "push.device_save_failed", "erro ao registrar o aparelho")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// DELETE /api/push/devices: tira o token do aparelho da conta (logout).
func handleDeletePushDevice(pushStore *store.PushStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}
		var body pushDeviceRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil || strings.TrimSpace(body.Token) == "" {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		if err := pushStore.DeleteDevice(r.Context(), account.ID, strings.TrimSpace(body.Token)); err != nil {
			log.Printf("server-central: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "push.device_delete_failed", "erro ao remover o aparelho")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// POST /api/push/grants: emite um grant da conta para um servidor da lista
// dela. O token em claro só aparece nesta resposta; o banco guarda o hash.
func handleCreatePushGrant(pushStore *store.PushStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}
		var body pushGrantRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		address := strings.TrimSpace(body.ServerAddress)
		if address == "" || len(address) > maxServerAddressLength {
			apierr.Write(w, http.StatusBadRequest, "push.server_address_required", "serverAddress é obrigatório")
			return
		}

		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			apierr.Write(w, http.StatusInternalServerError, "push.grant_create_failed", "erro ao criar o token de notificação")
			return
		}
		token := base64.RawURLEncoding.EncodeToString(raw)
		err := pushStore.CreateGrant(r.Context(), account.ID, address, hashGrant(token))
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "push.server_unknown", "servidor não está na sua lista")
			return
		}
		if err != nil {
			log.Printf("server-central: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "push.grant_create_failed", "erro ao criar o token de notificação")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(pushGrantResponse{Token: token, ServerAddress: address})
	})
}

// GET /api/push/mutes: servidores e canais silenciados pela conta.
func handleListPushMutes(pushStore *store.PushStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}
		mutes, err := pushStore.ListMutes(r.Context(), account.ID)
		if err != nil {
			log.Printf("server-central: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "push.mutes_fetch_failed", "erro ao buscar os silêncios")
			return
		}
		out := pushMutesResponse{Mutes: make([]pushMuteView, len(mutes))}
		for i, m := range mutes {
			out.Mutes[i] = pushMuteView{ServerAddress: m.ServerAddress}
			if m.ChannelID != "" {
				channelID := m.ChannelID
				out.Mutes[i].ChannelID = &channelID
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
}

// PUT /api/push/mutes: silencia ou volta a notificar um servidor (sem
// channelId) ou um canal dele. Um item por vez, para dois aparelhos
// mexendo ao mesmo tempo não sobrescreverem a lista um do outro.
func handleSetPushMute(pushStore *store.PushStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := auth.AccountFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.account_missing", "conta não encontrada no contexto")
			return
		}
		var body setPushMuteRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.Muted == nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		address := strings.TrimSpace(body.ServerAddress)
		if address == "" || len(address) > maxServerAddressLength {
			apierr.Write(w, http.StatusBadRequest, "push.server_address_required", "serverAddress é obrigatório")
			return
		}
		channelID := ""
		if body.ChannelID != nil {
			channelID = strings.TrimSpace(*body.ChannelID)
			if channelID == "" || len(channelID) > push.MaxIDRunes {
				apierr.Write(w, http.StatusBadRequest, "push.channel_id_invalid", "channelId inválido")
				return
			}
		}
		err := pushStore.SetMute(r.Context(), account.ID, address, channelID, *body.Muted)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "push.server_unknown", "servidor não está na sua lista")
			return
		}
		if err != nil {
			log.Printf("server-central: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "push.mute_save_failed", "erro ao salvar o silêncio")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// POST /api/push/notify: chamada pelo server-channel ao criar mensagem,
// sem conta (o grant é a credencial). Para cada grant que pertence a
// serverAddress e não está silenciado, enfileira a notificação para os
// aparelhos da conta. O texto só passa em memória; nada é gravado.
//
// A resposta é sempre a mesma (202) com grants válidos, desconhecidos, de
// outro servidor, silenciados, acima do limite ou com o push desligado,
// para não revelar quais tokens existem.
func handlePushNotify(pushStore *store.PushStore, dispatcher *push.Dispatcher, limits pushLimits) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limits.perIP.allow("ip:" + clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			apierr.Write(w, http.StatusTooManyRequests, "common.rate_limited", "muitas requisições, tente novamente em instantes")
			return
		}
		var body pushNotifyRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxNotifyBodyBytes)).Decode(&body); err != nil {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}
		address := strings.TrimSpace(body.ServerAddress)
		channelID := strings.TrimSpace(body.ChannelID)
		if address == "" || len(address) > maxServerAddressLength || channelID == "" || utf8.RuneCountInString(channelID) > push.MaxIDRunes || len(body.Grants) > maxGrantsPerNotify {
			apierr.Write(w, http.StatusBadRequest, "common.invalid_body", "corpo da requisição inválido")
			return
		}

		if dispatcher.Enabled() {
			notifyChannelMessage(r.Context(), pushStore, dispatcher, limits, address, channelID, body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"accepted"}`))
	})
}

func notifyChannelMessage(ctx context.Context, pushStore *store.PushStore, dispatcher *push.Dispatcher, limits pushLimits, address, channelID string, body pushNotifyRequest) {
	hashes := make([][]byte, 0, len(body.Grants))
	seen := make(map[string]bool, len(body.Grants))
	for _, g := range body.Grants {
		if g == "" || len(g) > maxGrantTokenLength || seen[g] {
			continue
		}
		seen[g] = true
		hashes = append(hashes, hashGrant(g))
	}
	targets, err := pushStore.ResolveGrants(ctx, address, channelID, hashes)
	if err != nil {
		log.Printf("server-central: push notify: %v", err)
		return
	}

	// Uma linha por chamada, só contagens (sem endereço, conta nem grant),
	// para saber onde um push parou: grant desconhecido ou de outro
	// servidor, silenciado, repetido da mesma conta, limite, ou entregue à
	// fila do FCM.
	muted, duplicate, limited, delivered := 0, 0, 0, 0
	defer func() {
		log.Printf("server-central: push notify: grants=%d desconhecidos=%d silenciados=%d repetidos=%d limitados=%d entregues=%d",
			len(body.Grants), len(body.Grants)-len(targets), muted, duplicate, limited, delivered)
	}()

	// Avatar do autor: resolvido uma vez, só quando há alguém para
	// notificar (chamada com grants inválidos não emite link).
	var authorFields map[string]string
	notified := make(map[string]bool, len(targets))
	for _, t := range targets {
		// Dois grants da mesma conta (dois aparelhos que entregaram o seu)
		// viram uma notificação só: o Dispatcher já manda a todos os
		// aparelhos da conta.
		if t.Muted {
			muted++
			continue
		}
		if notified[t.AccountID] {
			duplicate++
			continue
		}
		if !limits.perGrant.allow("grant:" + string(t.TokenHash)) {
			limited++
			continue
		}
		if !limits.perServer.allow("server:" + address) {
			limited++
			log.Printf("server-central: push notify: limite do servidor %s atingido", address)
			return
		}
		notified[t.AccountID] = true

		serverName := t.ServerName
		if serverName == "" {
			serverName = body.ServerName
		}
		data := push.Base(push.TypeChannelMessage)
		data["serverAddress"] = address
		data["serverName"] = push.Truncate(serverName, push.MaxNameRunes)
		data["channelId"] = channelID
		data["channelName"] = push.Truncate(body.ChannelName, push.MaxNameRunes)
		data["author"] = push.Truncate(body.Author, push.MaxAuthorRunes)
		data["text"] = push.Truncate(body.Text, push.MaxTextRunes)
		optional := map[string]string{
			"messageId":   push.Truncate(body.MessageID, push.MaxIDRunes),
			"threadId":    push.Truncate(body.ThreadID, push.MaxIDRunes),
			"threadTitle": push.Truncate(body.ThreadTitle, push.MaxNameRunes),
			"attachment":  push.Truncate(body.Attachment, push.MaxNameRunes),
		}
		for k, v := range optional {
			if v != "" {
				data[k] = v
			}
		}
		if authorFields == nil {
			authorFields = map[string]string{}
			dispatcher.AddAuthorAvatar(ctx, authorFields, "", strings.TrimSpace(body.AuthorSubject))
		}
		for k, v := range authorFields {
			data[k] = v
		}
		dispatcher.Notify(t.AccountID, data)
		delivered++
	}
}

// pushFromAccount enfileira uma notificação de DM ou de amizade para
// recipientID, com o nome de fromID buscado pelo worker (fora do caminho
// da requisição e do WebSocket). requestID só vale para friend_request.
func pushFromAccount(dispatcher *push.Dispatcher, profiles *store.ProfileStore, kind, recipientID, fromID, requestID string) {
	if !dispatcher.Enabled() {
		return
	}
	dispatcher.NotifyFunc(recipientID, func(ctx context.Context) map[string]string {
		data := push.Base(kind)
		data["accountId"] = fromID
		data["author"] = ""
		if profile, err := profiles.GetByAccountID(ctx, fromID); err == nil {
			data["author"] = push.Truncate(profile.DisplayName, push.MaxAuthorRunes)
		}
		if requestID != "" {
			data["requestId"] = requestID
		}
		dispatcher.AddAuthorAvatar(ctx, data, fromID, "")
		return data
	})
}

func hashGrant(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

type pushDeviceRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

type pushGrantRequest struct {
	ServerAddress string `json:"serverAddress"`
}

type pushGrantResponse struct {
	Token         string `json:"token"`
	ServerAddress string `json:"serverAddress"`
}

type pushMuteView struct {
	ServerAddress string  `json:"serverAddress"`
	ChannelID     *string `json:"channelId,omitempty"`
}

type pushMutesResponse struct {
	Mutes []pushMuteView `json:"mutes"`
}

type setPushMuteRequest struct {
	ServerAddress string  `json:"serverAddress"`
	ChannelID     *string `json:"channelId"`
	Muted         *bool   `json:"muted"`
}

type pushNotifyRequest struct {
	ServerAddress string   `json:"serverAddress"`
	ServerName    string   `json:"serverName"`
	ChannelID     string   `json:"channelId"`
	ChannelName   string   `json:"channelName"`
	Author        string   `json:"author"`
	Text          string   `json:"text"`
	MessageID     string   `json:"messageId"`
	ThreadID      string   `json:"threadId"`
	ThreadTitle   string   `json:"threadTitle"`
	Attachment    string   `json:"attachment"`
	// AuthorSubject é o "sub" do Authentik de quem mandou, que o
	// server-channel guarda por membro; o central o usa só para achar o
	// avatar da conta. Opcional: server-channel antigo não manda, e a
	// notificação sai sem avatar.
	AuthorSubject string   `json:"authorSubject"`
	Grants        []string `json:"grants"`
}
