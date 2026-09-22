package realtime

import (
	"encoding/json"
	"fmt"
	"time"
)

// Frames trocados no WebSocket de um canal seguem um envelope
// "{type, ...payload}". Num canal de texto, cliente -> servidor emite
// "message.create", "message.update" (edita mensagem própria) ou
// "message.delete" (apaga mensagem própria, ou de qualquer autor se
// Administrator — ver internal/httpapi/channel_ws.go). Num canal forum
// (mesma rota de WS, ver docs/architecture.md "Canal forum: threads/posts"),
// cliente -> servidor emite "thread.create" (abre uma thread com o post
// inicial) ou "post.create" (responde numa thread existente) — edição/exclusão
// não cobre canal forum ainda, mesmo escopo mínimo do TODO original. O
// servidor responde com o "*.created"/"*.updated"/"*.deleted" correspondente
// (broadcast para todo o canal, inclusive para o autor, para confirmar
// id/timestamp atribuídos pelo servidor) ou "error" (só para o client que
// causou o erro).
const (
	// TypeMessageCreate, TypeMessageUpdate e TypeMessageDelete são
	// exportados porque internal/httpapi precisa comparar com FrameType
	// antes de saber qual decoder chamar (um canal de texto aceita os três
	// tipos na mesma conexão).
	TypeMessageCreate  = "message.create"
	typeMessageCreated = "message.created"
	TypeMessageUpdate  = "message.update"
	typeMessageUpdated = "message.updated"
	TypeMessageDelete  = "message.delete"
	typeMessageDeleted = "message.deleted"

	// TypeThreadCreate e TypePostCreate são exportados pelo mesmo motivo
	// (um canal forum aceita os dois tipos na mesma conexão).
	TypeThreadCreate  = "thread.create"
	typeThreadCreated = "thread.created"
	TypePostCreate    = "post.create"
	typePostCreated   = "post.created"

	typeError = "error"
)

// IncomingMessageCreate é o payload decodificado de um frame
// "message.create" enviado pelo client.
type IncomingMessageCreate struct {
	Content string `json:"content"`
}

// IncomingMessageUpdate é o payload decodificado de um frame
// "message.update" já identificado via FrameType.
type IncomingMessageUpdate struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// IncomingMessageDelete é o payload decodificado de um frame
// "message.delete" já identificado via FrameType.
type IncomingMessageDelete struct {
	ID string `json:"id"`
}

// IncomingThreadCreate é o payload decodificado de um frame "thread.create":
// abre uma thread num canal forum com o título e o post inicial.
type IncomingThreadCreate struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// IncomingPostCreate é o payload decodificado de um frame "post.create":
// responde numa thread existente de um canal forum.
type IncomingPostCreate struct {
	ThreadID string `json:"threadId"`
	Content  string `json:"content"`
}

// MessageView é a representação em fio de uma mensagem, usada tanto no
// payload de "message.created" (canal de texto) quanto no de
// "thread.created"/"post.created" (canal forum). ThreadID vem nulo para
// mensagens de canal de texto. Attachments só é preenchido em mensagens de
// canal de texto (canal forum fora do escopo, ver docs/architecture.md,
// "Decisão: upload de anexo em mensagem").
type MessageView struct {
	ID             string           `json:"id"`
	ChannelID      string           `json:"channelId"`
	ThreadID       *string          `json:"threadId,omitempty"`
	AuthorMemberID string           `json:"authorMemberId"`
	Content        string           `json:"content"`
	CreatedAt      time.Time        `json:"createdAt"`
	EditedAt       *time.Time       `json:"editedAt,omitempty"`
	Attachments    []AttachmentView `json:"attachments,omitempty"`
}

// AttachmentView é a representação em fio de um anexo de mensagem. URL é
// relativo (o client resolve contra o baseUrl do server-channel, mesmo
// padrão do resto da API) e exige o mesmo Bearer token de qualquer outra
// rota -- não dá para usar direto num <img src>, o client busca via
// fetch()+Blob (ver client/src/lib/serverChannelApi.ts).
type AttachmentView struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
	URL         string `json:"url"`
}

// ThreadView é a representação em fio de uma thread de forum, usada no
// payload de "thread.created" e na listagem REST de threads de um canal.
type ThreadView struct {
	ID             string    `json:"id"`
	ChannelID      string    `json:"channelId"`
	Title          string    `json:"title"`
	AuthorMemberID string    `json:"authorMemberId"`
	CreatedAt      time.Time `json:"createdAt"`
}

type messageCreatedEnvelope struct {
	Type    string      `json:"type"`
	Message MessageView `json:"message"`
}

type messageUpdatedEnvelope struct {
	Type    string      `json:"type"`
	Message MessageView `json:"message"`
}

// messageDeletedEnvelope não carrega o MessageView inteiro — o client só
// precisa do id para remover a mensagem da lista local (ver
// hooks/useChannelChat.ts).
type messageDeletedEnvelope struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	ChannelID string `json:"channelId"`
}

type threadCreatedEnvelope struct {
	Type    string      `json:"type"`
	Thread  ThreadView  `json:"thread"`
	Message MessageView `json:"message"`
}

type postCreatedEnvelope struct {
	Type    string      `json:"type"`
	Message MessageView `json:"message"`
}

type errorEnvelope struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

type typeEnvelope struct {
	Type string `json:"type"`
}

// FrameType lê só o campo "type" de raw, sem decodificar o resto do payload
// — usado pelo canal forum para decidir se o frame é "thread.create" ou
// "post.create" antes de chamar o decoder específico.
func FrameType(raw []byte) (string, error) {
	var t typeEnvelope
	if err := json.Unmarshal(raw, &t); err != nil {
		return "", fmt.Errorf("frame não é JSON válido: %w", err)
	}
	return t.Type, nil
}

// DecodeIncoming lê o campo "type" de raw e, se for "message.create",
// decodifica o restante em IncomingMessageCreate. Tipos desconhecidos
// devolvem erro para o chamador responder com SendError.
func DecodeIncoming(raw []byte) (IncomingMessageCreate, error) {
	t, err := FrameType(raw)
	if err != nil {
		return IncomingMessageCreate{}, err
	}
	if t != TypeMessageCreate {
		return IncomingMessageCreate{}, fmt.Errorf("tipo de frame desconhecido: %q", t)
	}

	var m IncomingMessageCreate
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingMessageCreate{}, fmt.Errorf("payload de message.create inválido: %w", err)
	}
	return m, nil
}

// DecodeMessageUpdate decodifica o payload de um frame "message.update" já
// identificado via FrameType.
func DecodeMessageUpdate(raw []byte) (IncomingMessageUpdate, error) {
	var m IncomingMessageUpdate
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingMessageUpdate{}, fmt.Errorf("payload de message.update inválido: %w", err)
	}
	return m, nil
}

// DecodeMessageDelete decodifica o payload de um frame "message.delete" já
// identificado via FrameType.
func DecodeMessageDelete(raw []byte) (IncomingMessageDelete, error) {
	var m IncomingMessageDelete
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingMessageDelete{}, fmt.Errorf("payload de message.delete inválido: %w", err)
	}
	return m, nil
}

// DecodeThreadCreate decodifica o payload de um frame "thread.create" já
// identificado via FrameType.
func DecodeThreadCreate(raw []byte) (IncomingThreadCreate, error) {
	var m IncomingThreadCreate
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingThreadCreate{}, fmt.Errorf("payload de thread.create inválido: %w", err)
	}
	return m, nil
}

// DecodePostCreate decodifica o payload de um frame "post.create" já
// identificado via FrameType.
func DecodePostCreate(raw []byte) (IncomingPostCreate, error) {
	var m IncomingPostCreate
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingPostCreate{}, fmt.Errorf("payload de post.create inválido: %w", err)
	}
	return m, nil
}

// EncodeMessageCreated serializa o envelope broadcast a cada novo message.
func EncodeMessageCreated(m MessageView) ([]byte, error) {
	return json.Marshal(messageCreatedEnvelope{Type: typeMessageCreated, Message: m})
}

// EncodeMessageUpdated serializa o envelope broadcast quando uma mensagem é
// editada.
func EncodeMessageUpdated(m MessageView) ([]byte, error) {
	return json.Marshal(messageUpdatedEnvelope{Type: typeMessageUpdated, Message: m})
}

// EncodeMessageDeleted serializa o envelope broadcast quando uma mensagem é
// apagada.
func EncodeMessageDeleted(id, channelID string) ([]byte, error) {
	return json.Marshal(messageDeletedEnvelope{Type: typeMessageDeleted, ID: id, ChannelID: channelID})
}

// EncodeThreadCreated serializa o envelope broadcast quando uma thread de
// forum é aberta, incluindo o post inicial.
func EncodeThreadCreated(t ThreadView, m MessageView) ([]byte, error) {
	return json.Marshal(threadCreatedEnvelope{Type: typeThreadCreated, Thread: t, Message: m})
}

// EncodePostCreated serializa o envelope broadcast a cada nova resposta numa
// thread de forum.
func EncodePostCreated(m MessageView) ([]byte, error) {
	return json.Marshal(postCreatedEnvelope{Type: typePostCreated, Message: m})
}

func encodeError(message string) ([]byte, error) {
	return json.Marshal(errorEnvelope{Type: typeError, Error: message})
}
