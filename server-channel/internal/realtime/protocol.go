package realtime

import (
	"encoding/json"
	"fmt"
	"time"
)

// Frames trocados no WebSocket de um canal de texto seguem um envelope
// "{type, ...payload}". Cliente -> servidor só emite "message.create"; o
// servidor responde com "message.created" (broadcast, inclusive para o
// autor) ou "error" (só para o client que causou o erro).
const (
	typeMessageCreate  = "message.create"
	typeMessageCreated = "message.created"
	typeError          = "error"
)

// IncomingMessageCreate é o payload decodificado de um frame
// "message.create" enviado pelo client.
type IncomingMessageCreate struct {
	Content string `json:"content"`
}

// MessageView é a representação em fio de uma mensagem, usada no payload de
// "message.created".
type MessageView struct {
	ID             string     `json:"id"`
	ChannelID      string     `json:"channelId"`
	AuthorMemberID string     `json:"authorMemberId"`
	Content        string     `json:"content"`
	CreatedAt      time.Time  `json:"createdAt"`
	EditedAt       *time.Time `json:"editedAt,omitempty"`
}

type messageCreatedEnvelope struct {
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

// DecodeIncoming lê o campo "type" de raw e, se for "message.create",
// decodifica o restante em IncomingMessageCreate. Tipos desconhecidos
// devolvem erro para o chamador responder com SendError.
func DecodeIncoming(raw []byte) (IncomingMessageCreate, error) {
	var t typeEnvelope
	if err := json.Unmarshal(raw, &t); err != nil {
		return IncomingMessageCreate{}, fmt.Errorf("frame não é JSON válido: %w", err)
	}
	if t.Type != typeMessageCreate {
		return IncomingMessageCreate{}, fmt.Errorf("tipo de frame desconhecido: %q", t.Type)
	}

	var m IncomingMessageCreate
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingMessageCreate{}, fmt.Errorf("payload de message.create inválido: %w", err)
	}
	return m, nil
}

// EncodeMessageCreated serializa o envelope broadcast a cada novo message.
func EncodeMessageCreated(m MessageView) ([]byte, error) {
	return json.Marshal(messageCreatedEnvelope{Type: typeMessageCreated, Message: m})
}

func encodeError(message string) ([]byte, error) {
	return json.Marshal(errorEnvelope{Type: typeError, Error: message})
}
