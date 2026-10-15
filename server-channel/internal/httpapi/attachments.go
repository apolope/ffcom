package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/push"
	"a3sitsolutions.com/ffcom/server-channel/internal/realtime"
	"a3sitsolutions.com/ffcom/server-channel/internal/storage"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

// maxAttachmentFilenameLength trunca nomes de arquivo absurdamente longos
// antes de gravar -- não é um limite de segurança (storage_key, não
// filename, é o que localiza o arquivo em disco), só evita lixo na UI/DB.
const maxAttachmentFilenameLength = 255

// multipartMemoryThreshold é o maxMemory passado a ParseMultipartForm: só
// controla quantos bytes de partes não-arquivo ficam em memória antes de
// spillar pra um temp file, não é um limite de tamanho (isso é
// http.MaxBytesReader, aplicado antes, com o valor de attachmentMaxBytes).
const multipartMemoryThreshold = 10 << 20

// POST /api/channels/{id}/messages — envia mensagem com anexo
// (multipart/form-data: campo "content" opcional, "file" opcional; pelo
// menos um dos dois é obrigatório). Mensagem só-texto continua indo por
// WebSocket ("message.create", ver internal/httpapi/channel_ws.go); esta
// rota REST existe só porque o handshake de WebSocket não tem como carregar
// um arquivo multipart -- ver docs/architecture.md, "Decisão: upload de
// anexo em mensagem". O broadcast ("message.created") usa o mesmo
// hub.Broadcast do fluxo via WebSocket, então quem está com o canal aberto
// recebe a mensagem em tempo real por qualquer um dos dois caminhos,
// inclusive quem enviou (mesmo critério já usado no resto do sistema).
func handleCreateMessageWithAttachment(hub *realtime.Hub, channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore, attachments *store.AttachmentStore, files *storage.FileStore, attachmentMaxBytes int64, notifier *push.Notifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID := r.PathValue("id")

		channel, ok := loadChannel(w, r, channels, channelID)
		if !ok {
			return
		}
		if channel.Type != store.ChannelText {
			apierr.Write(w, http.StatusBadRequest, "channels.not_text", "canal não é de texto")
			return
		}
		member, canSend, ok := sendPermission(w, r, roles, overwrites, channelID)
		if !ok {
			return
		}
		if !canSend {
			apierr.Write(w, http.StatusForbidden, "messages.send_denied", "sem permissão para enviar mensagens neste canal")
			return
		}

		upload, ok := parseMessageUpload(w, r, attachmentMaxBytes)
		if !ok {
			return
		}
		defer upload.close(r)

		m, view, ok := createUploadedMessage(w, r, messages, attachments, files, channelID, nil, member.ID, upload)
		if !ok {
			return
		}

		if payload, err := realtime.EncodeMessageCreated(view); err != nil {
			log.Printf("server-channel: erro ao codificar mensagem criada: %v", err)
		} else {
			hub.Broadcast(channelID, payload)
		}
		messagePusher(notifier, hub, member)(m, "", upload.filename())

		writeCreatedJSON(w, view)
	})
}

// POST /api/channels/{id}/threads — abre uma thread num canal forum com
// anexo no post inicial (multipart/form-data: "title" obrigatório,
// "content" e "file" opcionais, pelo menos um dos dois). Thread sem anexo
// continua indo por WebSocket ("thread.create"); esta rota é o equivalente
// de POST /api/channels/{id}/messages para o fórum, pelo mesmo motivo (o
// WebSocket não carrega arquivo) -- ver docs/architecture.md, "Decisão:
// anexo em canal forum". Broadcast de "thread.created" pelo mesmo Hub.
func handleCreateThreadWithAttachment(hub *realtime.Hub, channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore, attachments *store.AttachmentStore, files *storage.FileStore, attachmentMaxBytes int64, notifier *push.Notifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelID := r.PathValue("id")

		channel, ok := loadChannel(w, r, channels, channelID)
		if !ok {
			return
		}
		if channel.Type != store.ChannelForum {
			apierr.Write(w, http.StatusBadRequest, "channels.not_forum", "canal não é forum")
			return
		}
		member, canSend, ok := sendPermission(w, r, roles, overwrites, channelID)
		if !ok {
			return
		}
		if !canSend {
			apierr.Write(w, http.StatusForbidden, "forum.post_denied", "sem permissão para postar neste canal")
			return
		}

		upload, ok := parseMessageUpload(w, r, attachmentMaxBytes)
		if !ok {
			return
		}
		defer upload.close(r)

		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			apierr.Write(w, http.StatusBadRequest, "forum.thread_title_empty", "título da thread não pode ser vazio")
			return
		}
		if len(title) > maxThreadTitleLength {
			apierr.WriteParams(w, http.StatusBadRequest, "forum.thread_title_too_long", fmt.Sprintf("título excede o limite de %d caracteres", maxThreadTitleLength),
				apierr.Params{"max": maxThreadTitleLength})
			return
		}

		thread, err := messages.CreateThread(r.Context(), channelID, title, member.ID)
		if err != nil {
			log.Printf("server-channel: erro ao criar thread: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "forum.thread_create_failed", "erro ao criar thread")
			return
		}
		m, view, ok := createUploadedMessage(w, r, messages, attachments, files, channelID, &thread.ID, member.ID, upload)
		if !ok {
			// Sem o post inicial, a thread ficaria vazia na lista.
			if err := messages.DeleteThread(r.Context(), thread.ID); err != nil {
				log.Printf("server-channel: erro ao limpar thread órfã %s: %v", thread.ID, err)
			}
			return
		}

		threadView := toThreadView(thread)
		if payload, err := realtime.EncodeThreadCreated(threadView, view); err != nil {
			log.Printf("server-channel: erro ao codificar thread criada: %v", err)
		} else {
			hub.Broadcast(channelID, payload)
		}
		messagePusher(notifier, hub, member)(m, thread.Title, upload.filename())

		writeCreatedJSON(w, createThreadResponse{Thread: threadView, Message: view})
	})
}

// POST /api/threads/{id}/messages — responde numa thread de forum com
// anexo (multipart/form-data, mesmos campos de POST
// /api/channels/{id}/messages). Resposta sem anexo continua indo por
// WebSocket ("post.create"). Broadcast de "post.created" no canal da thread.
func handleCreatePostWithAttachment(hub *realtime.Hub, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore, attachments *store.AttachmentStore, files *storage.FileStore, attachmentMaxBytes int64, notifier *push.Notifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		threadID := r.PathValue("id")

		thread, err := messages.GetThread(r.Context(), threadID)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "forum.thread_not_found", "thread não encontrada")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "forum.thread_fetch_failed", "erro ao buscar thread")
			return
		}
		member, canSend, ok := sendPermission(w, r, roles, overwrites, thread.ChannelID)
		if !ok {
			return
		}
		if !canSend {
			apierr.Write(w, http.StatusForbidden, "forum.post_denied", "sem permissão para postar neste canal")
			return
		}

		upload, ok := parseMessageUpload(w, r, attachmentMaxBytes)
		if !ok {
			return
		}
		defer upload.close(r)

		m, view, ok := createUploadedMessage(w, r, messages, attachments, files, thread.ChannelID, &thread.ID, member.ID, upload)
		if !ok {
			return
		}

		if payload, err := realtime.EncodePostCreated(view); err != nil {
			log.Printf("server-channel: erro ao codificar post criado: %v", err)
		} else {
			hub.Broadcast(thread.ChannelID, payload)
		}
		messagePusher(notifier, hub, member)(m, thread.Title, upload.filename())

		writeCreatedJSON(w, view)
	})
}

type createThreadResponse struct {
	Thread  realtime.ThreadView  `json:"thread"`
	Message realtime.MessageView `json:"message"`
}

func loadChannel(w http.ResponseWriter, r *http.Request, channels *store.ChannelStore, channelID string) (store.Channel, bool) {
	channel, err := channels.GetByID(r.Context(), channelID)
	if errors.Is(err, store.ErrNotFound) {
		apierr.Write(w, http.StatusNotFound, "channels.not_found", "canal não encontrado")
		return store.Channel{}, false
	}
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "channels.fetch_failed", "erro ao buscar canal")
		return store.Channel{}, false
	}
	return channel, true
}

// sendPermission devolve o membro da requisição e se ele tem SendMessages
// no canal. ok=false só em erro interno (resposta já escrita); o 403 fica
// com quem chama, porque canal de texto e fórum usam códigos diferentes.
func sendPermission(w http.ResponseWriter, r *http.Request, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, channelID string) (member store.Member, canSend, ok bool) {
	member, ok = auth.MemberFromContext(r.Context())
	if !ok {
		apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
		return store.Member{}, false, false
	}
	effective, err := channelPermission(r.Context(), roles, overwrites, member, channelID)
	if err != nil {
		apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
		return store.Member{}, false, false
	}
	return member, permissions.Has(effective, permissions.SendMessages), true
}

// messageUpload é o corpo multipart já validado de uma mensagem com anexo:
// conteúdo (pode ser vazio) e, opcionalmente, o arquivo.
type messageUpload struct {
	content string
	file    multipart.File
	header  *multipart.FileHeader
}

func (u messageUpload) hasFile() bool { return u.file != nil }

// filename é o nome do arquivo já sanitizado, ou "" sem anexo (o formato
// que messagePusher espera).
func (u messageUpload) filename() string {
	if !u.hasFile() {
		return ""
	}
	return sanitizeAttachmentFilename(u.header.Filename)
}

func (u messageUpload) close(r *http.Request) {
	if u.file != nil {
		u.file.Close()
	}
	r.MultipartForm.RemoveAll()
}

// parseMessageUpload lê o multipart (limitado a attachmentMaxBytes) e
// valida "content"/"file": pelo menos um dos dois, conteúdo dentro do
// limite, arquivo dentro do tamanho. Em erro, já escreveu a resposta; com
// ok, quem chama precisa de defer upload.close(r).
func parseMessageUpload(w http.ResponseWriter, r *http.Request, attachmentMaxBytes int64) (messageUpload, bool) {
	// +1MiB de folga sobre attachmentMaxBytes para o resto do multipart
	// form (boundary, headers de parte, campos de texto) -- o anexo em si
	// continua limitado a attachmentMaxBytes pelo tamanho do arquivo.
	r.Body = http.MaxBytesReader(w, r.Body, attachmentMaxBytes+1<<20)
	if err := r.ParseMultipartForm(multipartMemoryThreshold); err != nil {
		apierr.Write(w, http.StatusRequestEntityTooLarge, "attachments.invalid_or_too_large", "corpo inválido ou anexo maior que o limite permitido")
		return messageUpload{}, false
	}

	upload := messageUpload{content: strings.TrimSpace(r.FormValue("content"))}
	fail := func() (messageUpload, bool) {
		upload.close(r)
		return messageUpload{}, false
	}

	file, header, ferr := r.FormFile("file")
	if ferr == nil {
		upload.file, upload.header = file, header
	} else if !errors.Is(ferr, http.ErrMissingFile) {
		apierr.Write(w, http.StatusBadRequest, "attachments.invalid", "anexo inválido")
		return fail()
	}

	if len(upload.content) > maxMessageContentLength {
		apierr.WriteParams(w, http.StatusBadRequest, "messages.content_too_long", fmt.Sprintf("conteúdo excede o limite de %d caracteres", maxMessageContentLength),
			apierr.Params{"max": maxMessageContentLength})
		return fail()
	}
	if upload.content == "" && !upload.hasFile() {
		apierr.Write(w, http.StatusBadRequest, "messages.content_or_attachment_required", "mensagem precisa de conteúdo ou anexo")
		return fail()
	}
	if upload.hasFile() && upload.header.Size > attachmentMaxBytes {
		apierr.Write(w, http.StatusRequestEntityTooLarge, "attachments.too_large", "anexo maior que o limite permitido")
		return fail()
	}
	return upload, true
}

// createUploadedMessage grava o arquivo em disco, a mensagem (threadID nil
// em canal de texto, preenchido em post de fórum) e a linha do anexo,
// desfazendo o que já tinha sido gravado se um passo falha. Em erro, já
// escreveu a resposta.
func createUploadedMessage(w http.ResponseWriter, r *http.Request, messages *store.MessageStore, attachments *store.AttachmentStore, files *storage.FileStore, channelID string, threadID *string, authorMemberID string, upload messageUpload) (store.Message, realtime.MessageView, bool) {
	var storageKey string
	if upload.hasFile() {
		key, err := files.Save(upload.file)
		if err != nil {
			log.Printf("server-channel: erro ao salvar anexo: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "attachments.save_failed", "erro ao salvar anexo")
			return store.Message{}, realtime.MessageView{}, false
		}
		storageKey = key
	}

	m, err := messages.Create(r.Context(), channelID, threadID, authorMemberID, upload.content)
	if err != nil {
		if upload.hasFile() {
			_ = files.Delete(storageKey)
		}
		log.Printf("server-channel: erro ao criar mensagem: %v", err)
		apierr.Write(w, http.StatusInternalServerError, "messages.send_failed", "erro ao enviar mensagem")
		return store.Message{}, realtime.MessageView{}, false
	}

	view := toMessageView(m)
	if upload.hasFile() {
		contentType := upload.header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		a, err := attachments.Create(r.Context(), m.ID, upload.filename(), contentType, upload.header.Size, storageKey)
		if err != nil {
			_ = files.Delete(storageKey)
			if delErr := messages.Delete(r.Context(), m.ID); delErr != nil {
				log.Printf("server-channel: erro ao limpar mensagem órfã %s: %v", m.ID, delErr)
			}
			log.Printf("server-channel: erro ao gravar anexo: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "messages.send_failed", "erro ao enviar mensagem")
			return store.Message{}, realtime.MessageView{}, false
		}
		view.Attachments = toAttachmentViews([]store.Attachment{a})
	}
	return m, view, true
}

func writeCreatedJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(body)
}

// GET /api/attachments/{id} — baixa o arquivo de um anexo. Exige a mesma
// permissão ViewChannels do canal a que a mensagem do anexo pertence.
// Precisa de Bearer token como qualquer outra rota deste servidor, por isso
// um <img src="..."> ou clique direto num link não funciona -- o client
// sempre busca via fetch()+Blob (ver client/src/lib/serverChannelApi.ts).
func handleGetAttachment(attachments *store.AttachmentStore, messages *store.MessageStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, files *storage.FileStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		attachment, err := attachments.GetByID(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "attachments.not_found", "anexo não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "attachments.fetch_failed", "erro ao buscar anexo")
			return
		}

		msg, err := messages.GetByID(r.Context(), attachment.MessageID)
		if errors.Is(err, store.ErrNotFound) {
			apierr.Write(w, http.StatusNotFound, "attachments.not_found", "anexo não encontrado")
			return
		}
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "attachments.message_fetch_failed", "erro ao buscar mensagem do anexo")
			return
		}

		member, ok := auth.MemberFromContext(r.Context())
		if !ok {
			apierr.Write(w, http.StatusInternalServerError, "common.member_missing", "membro não encontrado no contexto")
			return
		}
		effective, err := channelPermission(r.Context(), roles, overwrites, member, msg.ChannelID)
		if err != nil {
			apierr.Write(w, http.StatusInternalServerError, "permissions.resolve_failed", "erro ao resolver permissões")
			return
		}
		if !permissions.Has(effective, permissions.ViewChannels) {
			apierr.Write(w, http.StatusForbidden, "channels.view_denied", "sem permissão para ver este canal")
			return
		}

		f, err := files.Open(attachment.StorageKey)
		if err != nil {
			log.Printf("server-channel: erro ao abrir anexo %s: %v", attachment.ID, err)
			apierr.Write(w, http.StatusInternalServerError, "attachments.read_failed", "erro ao ler anexo")
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", attachment.ContentType)
		w.Header().Set("Content-Length", strconv.FormatInt(attachment.SizeBytes, 10))
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", attachment.Filename))
		if _, err := io.Copy(w, f); err != nil {
			log.Printf("server-channel: erro ao servir anexo %s: %v", attachment.ID, err)
		}
	})
}

// sanitizeAttachmentFilename descarta qualquer componente de diretório do
// nome enviado pelo client (o browser manda só o basename, mas nada garante
// isso de todo client HTTP) e trunca para maxAttachmentFilenameLength. Um
// nome vazio depois disso vira "arquivo" -- só afeta o filename exibido/de
// download, nunca o caminho em disco (esse é storage_key, gerado pelo
// servidor, ver internal/storage.FileStore).
func sanitizeAttachmentFilename(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return "arquivo"
	}
	if len(name) > maxAttachmentFilenameLength {
		name = name[:maxAttachmentFilenameLength]
	}
	return name
}

func toAttachmentView(a store.Attachment) realtime.AttachmentView {
	return realtime.AttachmentView{
		ID:          a.ID,
		Filename:    a.Filename,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		URL:         "/api/attachments/" + a.ID,
	}
}

func toAttachmentViews(atts []store.Attachment) []realtime.AttachmentView {
	if len(atts) == 0 {
		return nil
	}
	out := make([]realtime.AttachmentView, len(atts))
	for i, a := range atts {
		out[i] = toAttachmentView(a)
	}
	return out
}
