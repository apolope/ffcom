package realtime

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	typeDMCreate  = "dm.create"
	typeDMCreated = "dm.created"
	typeError     = "error"
)

// presenceUpdateEnvelope é o frame que o gateway de presença emite:
// "{type: 'presence.update', accountId, online}", enviado só para contas
// amigas (accepted) da conta cujo status mudou.
type presenceUpdateEnvelope struct {
	Type      string `json:"type"`
	AccountID string `json:"accountId"`
	Online    bool   `json:"online"`
}

// EncodePresenceUpdate serializa o envelope de mudança de presença de accountID.
func EncodePresenceUpdate(accountID string, online bool) ([]byte, error) {
	return json.Marshal(presenceUpdateEnvelope{Type: "presence.update", AccountID: accountID, Online: online})
}

// IncomingDMCreate é o payload decodificado de um frame "dm.create" enviado
// pelo client nesta mesma conexão WebSocket (ver docs/architecture.md,
// "Decisão: DMs entregues no WebSocket de presença").
type IncomingDMCreate struct {
	RecipientID string `json:"recipientId"`
	Content     string `json:"content"`
}

// DirectMessageView é a representação em fio de uma DM, usada no payload de
// "dm.created".
type DirectMessageView struct {
	ID          string     `json:"id"`
	SenderID    string     `json:"senderId"`
	RecipientID string     `json:"recipientId"`
	Content     string     `json:"content"`
	CreatedAt   time.Time  `json:"createdAt"`
	EditedAt    *time.Time `json:"editedAt,omitempty"`
}

type dmCreatedEnvelope struct {
	Type    string            `json:"type"`
	Message DirectMessageView `json:"message"`
}

type errorEnvelope struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

type typeEnvelope struct {
	Type string `json:"type"`
}

// DecodeIncomingDM lê o campo "type" de raw e, se for "dm.create",
// decodifica o restante em IncomingDMCreate. Tipos desconhecidos devolvem
// erro para o chamador responder com Client.SendError.
func DecodeIncomingDM(raw []byte) (IncomingDMCreate, error) {
	var t typeEnvelope
	if err := json.Unmarshal(raw, &t); err != nil {
		return IncomingDMCreate{}, fmt.Errorf("frame não é JSON válido: %w", err)
	}
	if t.Type != typeDMCreate {
		return IncomingDMCreate{}, fmt.Errorf("tipo de frame desconhecido: %q", t.Type)
	}

	var m IncomingDMCreate
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingDMCreate{}, fmt.Errorf("payload de dm.create inválido: %w", err)
	}
	return m, nil
}

// EncodeDMCreated serializa o envelope enviado tanto ao remetente (para
// confirmar id/timestamp atribuídos pelo servidor) quanto ao destinatário.
func EncodeDMCreated(m DirectMessageView) ([]byte, error) {
	return json.Marshal(dmCreatedEnvelope{Type: typeDMCreated, Message: m})
}

// EncodeError serializa um envelope de erro, enviado só ao client que causou o erro.
func EncodeError(message string) ([]byte, error) {
	return json.Marshal(errorEnvelope{Type: typeError, Error: message})
}
