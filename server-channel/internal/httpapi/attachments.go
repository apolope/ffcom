package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
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
func handleCreateMessageWithAttachment(hub *realtime.Hub, channels *store.ChannelStore, roles *store.RoleStore, overwrites *store.ChannelOverwriteStore, messages *store.MessageStore, attachments *store.AttachmentStore, files *storage.FileStore, attachmentMaxBytes int64) http.Handler {
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
		if channel.Type != store.ChannelText {
			apierr.Write(w, http.StatusBadRequest, "channels.not_text", "canal não é de texto")
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
		if !permissions.Has(effective, permissions.SendMessages) {
			apierr.Write(w, http.StatusForbidden, "messages.send_denied", "sem permissão para enviar mensagens neste canal")
			return
		}

		// +1MiB de folga sobre attachmentMaxBytes para o resto do multipart
		// form (boundary, headers de parte, campo content) -- o anexo em si
		// continua limitado a attachmentMaxBytes pelo tamanho do arquivo.
		r.Body = http.MaxBytesReader(w, r.Body, attachmentMaxBytes+1<<20)
		if err := r.ParseMultipartForm(multipartMemoryThreshold); err != nil {
			apierr.Write(w, http.StatusRequestEntityTooLarge, "attachments.invalid_or_too_large", "corpo inválido ou anexo maior que o limite permitido")
			return
		}
		defer r.MultipartForm.RemoveAll()

		content := strings.TrimSpace(r.FormValue("content"))
		if len(content) > maxMessageContentLength {
			apierr.WriteParams(w, http.StatusBadRequest, "messages.content_too_long", fmt.Sprintf("conteúdo excede o limite de %d caracteres", maxMessageContentLength),
				apierr.Params{"max": maxMessageContentLength})
			return
		}

		file, header, ferr := r.FormFile("file")
		hasFile := ferr == nil
		if hasFile {
			defer file.Close()
		} else if !errors.Is(ferr, http.ErrMissingFile) {
			apierr.Write(w, http.StatusBadRequest, "attachments.invalid", "anexo inválido")
			return
		}

		if content == "" && !hasFile {
			apierr.Write(w, http.StatusBadRequest, "messages.content_or_attachment_required", "mensagem precisa de conteúdo ou anexo")
			return
		}

		var storageKey, filename, contentType string
		var sizeBytes int64
		if hasFile {
			if header.Size > attachmentMaxBytes {
				apierr.Write(w, http.StatusRequestEntityTooLarge, "attachments.too_large", "anexo maior que o limite permitido")
				return
			}
			filename = sanitizeAttachmentFilename(header.Filename)
			contentType = header.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			key, err := files.Save(file)
			if err != nil {
				log.Printf("server-channel: erro ao salvar anexo: %v", err)
				apierr.Write(w, http.StatusInternalServerError, "attachments.save_failed", "erro ao salvar anexo")
				return
			}
			storageKey = key
			sizeBytes = header.Size
		}

		m, err := messages.Create(r.Context(), channelID, nil, member.ID, content)
		if err != nil {
			if hasFile {
				_ = files.Delete(storageKey)
			}
			log.Printf("server-channel: erro ao criar mensagem: %v", err)
			apierr.Write(w, http.StatusInternalServerError, "messages.send_failed", "erro ao enviar mensagem")
			return
		}

		view := toMessageView(m)
		if hasFile {
			a, err := attachments.Create(r.Context(), m.ID, filename, contentType, sizeBytes, storageKey)
			if err != nil {
				_ = files.Delete(storageKey)
				if delErr := messages.Delete(r.Context(), m.ID); delErr != nil {
					log.Printf("server-channel: erro ao limpar mensagem órfã %s: %v", m.ID, delErr)
				}
				log.Printf("server-channel: erro ao gravar anexo: %v", err)
				apierr.Write(w, http.StatusInternalServerError, "messages.send_failed", "erro ao enviar mensagem")
				return
			}
			view.Attachments = toAttachmentViews([]store.Attachment{a})
		}

		if payload, err := realtime.EncodeMessageCreated(view); err != nil {
			log.Printf("server-channel: erro ao codificar mensagem criada: %v", err)
		} else {
			hub.Broadcast(channelID, payload)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(view)
	})
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
