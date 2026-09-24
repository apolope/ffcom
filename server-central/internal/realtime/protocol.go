package realtime

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	TypeDMCreate     = "dm.create"
	TypePresenceIdle = "presence.idle"

	typeDMCreated = "dm.created"
	typeError     = "error"
)

// presenceUpdateEnvelope é o frame que o gateway de presença emite:
// "{type: 'presence.update', accountId, status, online}", enviado só para
// contas amigas (accepted) da conta cujo status visível mudou. online
// (= status diferente de offline) continua no frame para clients anteriores
// ao status.
type presenceUpdateEnvelope struct {
	Type      string `json:"type"`
	AccountID string `json:"accountId"`
	Status    string `json:"status"`
	Online    bool   `json:"online"`
}

// EncodePresenceUpdate serializa o envelope de mudança de presença de
// accountID, com o status visível (Status*).
func EncodePresenceUpdate(accountID, status string) ([]byte, error) {
	return json.Marshal(presenceUpdateEnvelope{
		Type:      "presence.update",
		AccountID: accountID,
		Status:    status,
		Online:    status != StatusOffline,
	})
}

// IncomingPresenceIdle é o frame "presence.idle" que o client manda quando
// a pessoa fica ociosa (ou volta) naquela conexão; ver Hub.SetIdle.
type IncomingPresenceIdle struct {
	Idle bool `json:"idle"`
}

// FrameType lê só o campo "type" de um frame vindo do client, para o
// chamador decidir como decodificar o resto.
func FrameType(raw []byte) (string, error) {
	var t typeEnvelope
	if err := json.Unmarshal(raw, &t); err != nil {
		return "", fmt.Errorf("frame não é JSON válido: %w", err)
	}
	return t.Type, nil
}

// DecodeIncomingPresenceIdle decodifica um frame "presence.idle".
func DecodeIncomingPresenceIdle(raw []byte) (IncomingPresenceIdle, error) {
	var m IncomingPresenceIdle
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingPresenceIdle{}, fmt.Errorf("payload de presence.idle inválido: %w", err)
	}
	return m, nil
}

// IncomingDMCreate é o payload decodificado de um frame "dm.create" enviado
// pelo client nesta mesma conexão WebSocket (ver docs/architecture.md,
// "Decisão: DMs entregues no WebSocket de presença"). Ciphertext/Nonce
// chegam como bytes -- encoding/json decodifica []byte a partir de base64
// automaticamente -- e são opacos ao servidor (criptografia ponta-a-ponta,
// ver docs/architecture.md, "Decisão: criptografia ponta-a-ponta em DMs").
type IncomingDMCreate struct {
	RecipientID string `json:"recipientId"`
	Ciphertext  []byte `json:"ciphertext"`
	Nonce       []byte `json:"nonce"`
}

// DirectMessageView é a representação em fio de uma DM, usada no payload de
// "dm.created". Ciphertext/Nonce serializam como base64 (comportamento
// padrão de encoding/json para []byte).
type DirectMessageView struct {
	ID          string     `json:"id"`
	SenderID    string     `json:"senderId"`
	RecipientID string     `json:"recipientId"`
	Ciphertext  []byte     `json:"ciphertext"`
	Nonce       []byte     `json:"nonce"`
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
	if t.Type != TypeDMCreate {
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
