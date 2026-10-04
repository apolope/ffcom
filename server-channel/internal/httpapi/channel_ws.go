package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/storage"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

const (
	maxMessageContentLength = 4000
	maxThreadTitleLength    = 200
)

// newUpgrader monta o upgrader do WebSocket de canal de texto. Subprotocols
// precisa incluir "access_token" (ver internal/auth.wsAuthSubprotocol) para
// o gorilla aceitar e ecoar de volta o subprotocolo que o client usa para
// carregar o token no handshake, já que a API WebSocket do navegador não
// permite setar o header Authorization diretamente.
//
// CheckOrigin reproduz o fallback padrão do gorilla (sem header Origin, ou
// Origin igual ao Host, sempre passa) e adicionalmente aceita qualquer
// origem em allowedOrigins (mesma lista usada pelo CORS REST em cors.go) —
// ver docs/architecture.md, "Decisão: CORS em server-channel".
func newUpgrader(allowedOrigins map[string]bool) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"access_token"},
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			if allowedOrigins[origin] {
				return true
			}
			u, err := url.Parse(origin)
			return err == nil && u.Host == r.Host
		},
	}
}

// GET /api/channels/{id}/ws — WebSocket de um canal de texto ou forum. Num
// canal de texto o client autenticado envia frames "message.create"; num
// canal forum, "thread.create" (abre thread com o post inicial) ou
// "post.create" (responde numa thread existente) — mesma rota e mesmo
// realtime.Hub particionado por canal, ver docs/architecture.md "Canal
// forum: threads/posts" (reaproveita a decisão já registrada para canal de
// texto em vez de introduzir um hub por thread). O servidor persiste via
// MessageStore e distribui o "*.created" correspondente via realtime.Hub
// para todos os clients conectados a este canal (broadcast, inclusive para
// o autor, para confirmar id/timestamp atribuídos pelo servidor).
//
// wsLimiter throttla frames por membro+dispositivo dentro desta conexão já aberta (ver
// docs/architecture.md, "Decisão: rate limiting em server-channel") — o
// limiter por IP em withRateLimit só cobre o handshake HTTP inicial, não
// protege contra flood de mensagens depois do upgrade.
func handleChannelWS(hub *realtime.Hub, channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore, attachments *store.AttachmentStore, files *storage.FileStore, upgrader websocket.Upgrader, wsLimiter *rateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID := r.PathValue("id")

		channel, err := channels.GetByID(r.Context(), channelID)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "channels.not_found", "canal não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "channels.fetch_failed", "erro ao buscar canal")
			return
		}
		if channel.Type != store.ChannelText && channel.Type != store.ChannelForum {
			apierr.Write(w, http.StatusBadRequest, "channels.realtime_unsupported", "canal não suporta conexão em tempo real")
			return
		}

		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
			return
		}
		effective, err := channelPermission(r.Context(), roles, overwrites, member, channelID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
			return
		}
		if !permissions.Has(effective, permissions.ViewChannels) {
			apierr.Write(w, http.StatusForbidden, "channels.view_denied", "sem permissão para ver este canal")
			return
		}
		// SendMessages é checado uma vez aqui, não a cada frame: mudanças de
		// role só valem a partir da próxima conexão (mesmo tipo de corte já
		// aceito no resto do sistema de tempo real, ver docs/architecture.md).
		// Num canal forum, o mesmo bit controla abrir thread e postar
		// resposta — não ganhou bit próprio (ver docs/architecture.md).
		canSend := permissions.Has(effective, permissions.SendMessages)
		// Um balde por membro+dispositivo ("sid" do token do handshake), como
		// no limiter de REST: o celular mandando mensagem não esgota o
		// orçamento do PC. Sem "sid", todas as conexões do membro dividem um.
		frameKey := member.ID
		if session := auth.SessionFromContext(r.Context()); session != "" {
			frameKey += "|sid:" + session
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// upgrader já escreveu a resposta HTTP de erro.
			return
		}

		client := realtime.NewClient(conn)
		hub.Register(channelID, client)
		defer hub.Unregister(channelID, client)

		go client.WritePump()
		if channel.Type == store.ChannelForum {
			client.ReadPump(func(raw []byte) {
				if !wsLimiter.allow(frameKey) {
					client.SendError(apierr.New("realtime.rate_limited", "muitas mensagens, aguarde um instante"))
					return
				}
				if !canSend {
					client.SendError(apierr.New("forum.post_denied", "sem permissão para postar neste canal"))
					return
				}
				handleIncomingForumFrame(r.Context(), hub, messages, channelID, member.ID, client, raw)
			})
			return
		}
		client.ReadPump(func(raw []byte) {
			if !wsLimiter.allow(frameKey) {
				client.SendError(apierr.New("realtime.rate_limited", "muitas mensagens, aguarde um instante"))
				return
			}
			handleIncomingTextFrame(r.Context(), hub, messages, attachments, files, channelID, member.ID, effective, canSend, client, raw)
		})
	})
}

// handleIncomingTextFrame despacha um frame recebido num canal de texto:
// "message.create" (exige SendMessages, checado uma vez por conexão — ver
// canSend acima), "message.update" ou "message.delete" (autoria/Administrator
// checados por mensagem dentro de cada handler, não pelo bit SendMessages —
// ver docs/architecture.md, "Decisão: editar/apagar mensagem de texto").
// Qualquer outro tipo é rejeitado com error.
func handleIncomingTextFrame(ctx context.Context, hub *realtime.Hub, messages *store.MessageStore, attachments *store.AttachmentStore, files *storage.FileStore, channelID, memberID string, effective int64, canSend bool, client *realtime.Client, raw []byte) {
	frameType, prob := realtime.FrameType(raw)
	if prob != nil {
		client.SendError(prob)
		return
	}

	switch frameType {
	case realtime.TypeMessageCreate:
		if !canSend {
			client.SendError(apierr.New("messages.send_denied", "sem permissão para enviar mensagens neste canal"))
			return
		}
		handleIncomingMessage(ctx, hub, messages, channelID, memberID, client, raw)
	case realtime.TypeMessageUpdate:
		handleIncomingMessageUpdate(ctx, hub, messages, attachments, channelID, memberID, client, raw)
	case realtime.TypeMessageDelete:
		handleIncomingMessageDelete(ctx, hub, messages, attachments, files, channelID, memberID, effective, client, raw)
	default:
		client.SendError(realtime.UnknownFrameType(frameType))
	}
}

func handleIncomingMessage(ctx context.Context, hub *realtime.Hub, messages *store.MessageStore, channelID, authorMemberID string, client *realtime.Client, raw []byte) {
	incoming, prob := realtime.DecodeIncoming(raw)
	if prob != nil {
		client.SendError(prob)
		return
	}

	content := strings.TrimSpace(incoming.Content)
	if content == "" {
		client.SendError(apierr.New("messages.content_empty", "conteúdo da mensagem não pode ser vazio"))
		return
	}
	if len(content) > maxMessageContentLength {
		client.SendError(apierr.NewParams("messages.content_too_long", fmt.Sprintf("conteúdo excede o limite de %d caracteres", maxMessageContentLength),
			apierr.Params{"max": maxMessageContentLength}))
		return
	}

	m, err := messages.Create(ctx, channelID, nil, authorMemberID, content)
	if err != nil {
		log.Printf("server-channel: erro ao criar mensagem: %v", err)
		client.SendError(apierr.New("messages.send_failed", "erro ao enviar mensagem"))
		return
	}

	payload, err := realtime.EncodeMessageCreated(toMessageView(m))
	if err != nil {
		log.Printf("server-channel: erro ao codificar mensagem criada: %v", err)
		return
	}
	hub.Broadcast(channelID, payload)
}

// handleIncomingMessageUpdate edita uma mensagem existente. Só o autor pode
// editar a própria mensagem — edição por moderação não está no escopo desta
// v1 (Administrator só pode apagar, ver handleIncomingMessageDelete).
// Edição não mexe em anexo (imutável nesta v1, ver docs/architecture.md,
// "Decisão: upload de anexo em mensagem") — mas o broadcast precisa
// reanexar o que já existia, ou o client perderia o anexo da mensagem ao
// aplicar "message.updated" (que substitui a mensagem inteira no estado
// local, ver hooks/useChannelChat.ts).
func handleIncomingMessageUpdate(ctx context.Context, hub *realtime.Hub, messages *store.MessageStore, attachments *store.AttachmentStore, channelID, memberID string, client *realtime.Client, raw []byte) {
	incoming, prob := realtime.DecodeMessageUpdate(raw)
	if prob != nil {
		client.SendError(prob)
		return
	}

	content := strings.TrimSpace(incoming.Content)
	if content == "" {
		client.SendError(apierr.New("messages.content_empty", "conteúdo da mensagem não pode ser vazio"))
		return
	}
	if len(content) > maxMessageContentLength {
		client.SendError(apierr.NewParams("messages.content_too_long", fmt.Sprintf("conteúdo excede o limite de %d caracteres", maxMessageContentLength),
			apierr.Params{"max": maxMessageContentLength}))
		return
	}

	existing, err := messages.GetByID(ctx, incoming.ID)
	if errors.Is(err, store.ErrNotFound) {
		client.SendError(apierr.New("messages.not_found", "mensagem não encontrada"))
		return
	}
	if err != nil {
		log.Printf("server-channel: erro ao buscar mensagem para editar: %v", err)
		client.SendError(apierr.New("messages.edit_failed", "erro ao editar mensagem"))
		return
	}
	if existing.ChannelID != channelID {
		client.SendError(apierr.New("messages.wrong_channel", "mensagem não pertence a este canal"))
		return
	}
	if existing.AuthorMemberID != memberID {
		client.SendError(apierr.New("messages.edit_author_only", "só o autor pode editar a mensagem"))
		return
	}

	m, err := messages.Edit(ctx, incoming.ID, content)
	if err != nil {
		log.Printf("server-channel: erro ao editar mensagem: %v", err)
		client.SendError(apierr.New("messages.edit_failed", "erro ao editar mensagem"))
		return
	}

	view := toMessageView(m)
	if atts, err := attachments.ListForMessage(ctx, m.ID); err != nil {
		log.Printf("server-channel: erro ao buscar anexos da mensagem editada %s: %v", m.ID, err)
	} else {
		view.Attachments = toAttachmentViews(atts)
	}

	payload, err := realtime.EncodeMessageUpdated(view)
	if err != nil {
		log.Printf("server-channel: erro ao codificar mensagem editada: %v", err)
		return
	}
	hub.Broadcast(channelID, payload)
}

// handleIncomingMessageDelete apaga uma mensagem existente. O autor sempre
// pode apagar a própria mensagem; Administrator pode apagar qualquer
// mensagem do canal (moderação — não há bit de permissão dedicado a
// mensagens, ver docs/architecture.md). O DELETE em cascata (FK
// attachments.message_id) já limpa a linha em Postgres, mas não o arquivo
// em disco -- por isso busca os anexos antes de apagar a mensagem e limpa
// os arquivos depois, best-effort (erro de limpeza de arquivo não desfaz a
// exclusão da mensagem, só fica logado).
func handleIncomingMessageDelete(ctx context.Context, hub *realtime.Hub, messages *store.MessageStore, attachments *store.AttachmentStore, files *storage.FileStore, channelID, memberID string, effective int64, client *realtime.Client, raw []byte) {
	incoming, prob := realtime.DecodeMessageDelete(raw)
	if prob != nil {
		client.SendError(prob)
		return
	}

	existing, err := messages.GetByID(ctx, incoming.ID)
	if errors.Is(err, store.ErrNotFound) {
		client.SendError(apierr.New("messages.not_found", "mensagem não encontrada"))
		return
	}
	if err != nil {
		log.Printf("server-channel: erro ao buscar mensagem para apagar: %v", err)
		client.SendError(apierr.New("messages.delete_failed", "erro ao apagar mensagem"))
		return
	}
	if existing.ChannelID != channelID {
		client.SendError(apierr.New("messages.wrong_channel", "mensagem não pertence a este canal"))
		return
	}
	if existing.AuthorMemberID != memberID && !permissions.Has(effective, permissions.Administrator) {
		client.SendError(apierr.New("messages.delete_denied", "sem permissão para apagar esta mensagem"))
		return
	}

	atts, err := attachments.ListForMessage(ctx, incoming.ID)
	if err != nil {
		log.Printf("server-channel: erro ao buscar anexos antes de apagar mensagem %s: %v", incoming.ID, err)
	}

	if err := messages.Delete(ctx, incoming.ID); err != nil {
		log.Printf("server-channel: erro ao apagar mensagem: %v", err)
		client.SendError(apierr.New("messages.delete_failed", "erro ao apagar mensagem"))
		return
	}
	for _, a := range atts {
		if err := files.Delete(a.StorageKey); err != nil {
			log.Printf("server-channel: erro ao apagar arquivo do anexo %s: %v", a.ID, err)
		}
	}

	payload, err := realtime.EncodeMessageDeleted(incoming.ID, channelID)
	if err != nil {
		log.Printf("server-channel: erro ao codificar mensagem apagada: %v", err)
		return
	}
	hub.Broadcast(channelID, payload)
}

// handleIncomingForumFrame despacha um frame recebido num canal forum:
// "thread.create" ou "post.create" (ver realtime.FrameType). Qualquer outro
// tipo é rejeitado com error.
func handleIncomingForumFrame(ctx context.Context, hub *realtime.Hub, messages *store.MessageStore, channelID, authorMemberID string, client *realtime.Client, raw []byte) {
	frameType, prob := realtime.FrameType(raw)
	if prob != nil {
		client.SendError(prob)
		return
	}

	switch frameType {
	case realtime.TypeThreadCreate:
		handleIncomingThreadCreate(ctx, hub, messages, channelID, authorMemberID, client, raw)
	case realtime.TypePostCreate:
		handleIncomingPostCreate(ctx, hub, messages, channelID, authorMemberID, client, raw)
	default:
		client.SendError(realtime.UnknownFrameType(frameType))
	}
}

// handleIncomingThreadCreate abre uma thread num canal forum: cria a linha
// em threads e o post inicial em messages (ThreadID preenchido) numa única
// operação lógica, depois faz broadcast de "thread.created" com os dois.
func handleIncomingThreadCreate(ctx context.Context, hub *realtime.Hub, messages *store.MessageStore, channelID, authorMemberID string, client *realtime.Client, raw []byte) {
	incoming, prob := realtime.DecodeThreadCreate(raw)
	if prob != nil {
		client.SendError(prob)
		return
	}

	title := strings.TrimSpace(incoming.Title)
	if title == "" {
		client.SendError(apierr.New("forum.thread_title_empty", "título da thread não pode ser vazio"))
		return
	}
	if len(title) > maxThreadTitleLength {
		client.SendError(apierr.NewParams("forum.thread_title_too_long", fmt.Sprintf("título excede o limite de %d caracteres", maxThreadTitleLength),
			apierr.Params{"max": maxThreadTitleLength}))
		return
	}
	content := strings.TrimSpace(incoming.Content)
	if content == "" {
		client.SendError(apierr.New("forum.first_post_empty", "conteúdo do post inicial não pode ser vazio"))
		return
	}
	if len(content) > maxMessageContentLength {
		client.SendError(apierr.NewParams("messages.content_too_long", fmt.Sprintf("conteúdo excede o limite de %d caracteres", maxMessageContentLength),
			apierr.Params{"max": maxMessageContentLength}))
		return
	}

	thread, err := messages.CreateThread(ctx, channelID, title, authorMemberID)
	if err != nil {
		log.Printf("server-channel: erro ao criar thread: %v", err)
		client.SendError(apierr.New("forum.thread_create_failed", "erro ao criar thread"))
		return
	}
	m, err := messages.Create(ctx, channelID, &thread.ID, authorMemberID, content)
	if err != nil {
		log.Printf("server-channel: erro ao criar post inicial da thread: %v", err)
		client.SendError(apierr.New("forum.thread_create_failed", "erro ao criar thread"))
		return
	}

	payload, err := realtime.EncodeThreadCreated(toThreadView(thread), toMessageView(m))
	if err != nil {
		log.Printf("server-channel: erro ao codificar thread criada: %v", err)
		return
	}
	hub.Broadcast(channelID, payload)
}

// handleIncomingPostCreate responde numa thread existente. Confere que a
// thread pertence ao canal desta conexão antes de gravar, para não permitir
// postar via o id de uma thread de outro canal forum.
func handleIncomingPostCreate(ctx context.Context, hub *realtime.Hub, messages *store.MessageStore, channelID, authorMemberID string, client *realtime.Client, raw []byte) {
	incoming, prob := realtime.DecodePostCreate(raw)
	if prob != nil {
		client.SendError(prob)
		return
	}

	content := strings.TrimSpace(incoming.Content)
	if content == "" {
		client.SendError(apierr.New("messages.content_empty", "conteúdo da mensagem não pode ser vazio"))
		return
	}
	if len(content) > maxMessageContentLength {
		client.SendError(apierr.NewParams("messages.content_too_long", fmt.Sprintf("conteúdo excede o limite de %d caracteres", maxMessageContentLength),
			apierr.Params{"max": maxMessageContentLength}))
		return
	}

	thread, err := messages.GetThread(ctx, incoming.ThreadID)
	if errors.Is(err, store.ErrNotFound) {
		client.SendError(apierr.New("forum.thread_not_found", "thread não encontrada"))
		return
	}
	if err != nil {
		log.Printf("server-channel: erro ao buscar thread: %v", err)
		client.SendError(apierr.New("forum.post_failed", "erro ao enviar post"))
		return
	}
	if thread.ChannelID != channelID {
		client.SendError(apierr.New("forum.thread_wrong_channel", "thread não pertence a este canal"))
		return
	}

	m, err := messages.Create(ctx, channelID, &thread.ID, authorMemberID, content)
	if err != nil {
		log.Printf("server-channel: erro ao criar post: %v", err)
		client.SendError(apierr.New("forum.post_failed", "erro ao enviar post"))
		return
	}

	payload, err := realtime.EncodePostCreated(toMessageView(m))
	if err != nil {
		log.Printf("server-channel: erro ao codificar post criado: %v", err)
		return
	}
	hub.Broadcast(channelID, payload)
}
