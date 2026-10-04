package realtime

import (
	"encoding/json"
	"fmt"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/apierr"
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
func FrameType(raw []byte) (string, *apierr.Problem) {
	var t typeEnvelope
	if err := json.Unmarshal(raw, &t); err != nil {
		return "", apierr.New("realtime.frame_invalid", fmt.Sprintf("frame não é JSON válido: %v", err))
	}
	return t.Type, nil
}

// DecodeIncomingPresenceIdle decodifica um frame "presence.idle".
func DecodeIncomingPresenceIdle(raw []byte) (IncomingPresenceIdle, *apierr.Problem) {
	var m IncomingPresenceIdle
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingPresenceIdle{}, invalidPayload(TypePresenceIdle, err)
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

// errorEnvelope é o frame "error", no mesmo formato dos erros da API HTTP
// (code, message, params; ver docs/protocol.md, "Erros da API HTTP"). Error
// repete Message para os clients anteriores ao code, que só liam esse campo.
type errorEnvelope struct {
	Type    string        `json:"type"`
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Params  apierr.Params `json:"params,omitempty"`
	Error   string        `json:"error"`
}

type typeEnvelope struct {
	Type string `json:"type"`
}

// DecodeIncomingDM lê o campo "type" de raw e, se for "dm.create",
// decodifica o restante em IncomingDMCreate. Tipos desconhecidos devolvem
// erro para o chamador responder com Client.SendError.
func DecodeIncomingDM(raw []byte) (IncomingDMCreate, *apierr.Problem) {
	var t typeEnvelope
	if err := json.Unmarshal(raw, &t); err != nil {
		return IncomingDMCreate{}, apierr.New("realtime.frame_invalid", fmt.Sprintf("frame não é JSON válido: %v", err))
	}
	if t.Type != TypeDMCreate {
		return IncomingDMCreate{}, UnknownFrameType(t.Type)
	}

	var m IncomingDMCreate
	if err := json.Unmarshal(raw, &m); err != nil {
		return IncomingDMCreate{}, invalidPayload(TypeDMCreate, err)
	}
	return m, nil
}

// EncodeDMCreated serializa o envelope enviado tanto ao remetente (para
// confirmar id/timestamp atribuídos pelo servidor) quanto ao destinatário.
func EncodeDMCreated(m DirectMessageView) ([]byte, error) {
	return json.Marshal(dmCreatedEnvelope{Type: typeDMCreated, Message: m})
}

// EncodeError serializa um frame "error", enviado só ao client que causou o erro.
func EncodeError(p *apierr.Problem) ([]byte, error) {
	return json.Marshal(errorEnvelope{Type: typeError, Code: p.Code, Message: p.Message, Params: p.Params, Error: p.Message})
}

// UnknownFrameType é o erro de um frame com "type" que a conexão não aceita.
func UnknownFrameType(frameType string) *apierr.Problem {
	return apierr.NewParams("realtime.frame_type_unknown", fmt.Sprintf("tipo de frame desconhecido: %q", frameType),
		apierr.Params{"type": frameType})
}

// invalidPayload é o erro de um frame de tipo conhecido cujo payload não
// decodifica.
func invalidPayload(frameType string, err error) *apierr.Problem {
	return apierr.NewParams("realtime.payload_invalid", fmt.Sprintf("payload de %s inválido: %v", frameType, err),
		apierr.Params{"type": frameType})
}

// FriendRequestView é um pedido de amizade pendente do ponto de vista de
// quem o recebe no frame: AccountID/DisplayName/AvatarURL são sempre do
// outro lado (quem mandou, num pedido recebido). Mesmo formato dos itens de
// GET /api/friends/requests.
type FriendRequestView struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"accountId"`
	DisplayName *string   `json:"displayName,omitempty"`
	AvatarURL   *string   `json:"avatarUrl,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type friendRequestEnvelope struct {
	Type    string            `json:"type"`
	Request FriendRequestView `json:"request"`
}

type friendRequestRemovedEnvelope struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type friendAcceptedEnvelope struct {
	Type      string  `json:"type"`
	RequestID string  `json:"requestId,omitempty"`
	AccountID string  `json:"accountId"`
	Name      *string `json:"displayName,omitempty"`
}

type friendRemovedEnvelope struct {
	Type      string `json:"type"`
	AccountID string `json:"accountId"`
}

// EncodeFriendRemoved serializa "friend.removed", enviado aos dois lados
// quando uma amizade aceita é desfeita; accountID é o ex-amigo do ponto de
// vista de quem recebe o frame.
func EncodeFriendRemoved(accountID string) ([]byte, error) {
	return json.Marshal(friendRemovedEnvelope{Type: "friend.removed", AccountID: accountID})
}

// EncodeFriendRequest serializa "friend.request", enviado a quem recebeu
// um pedido de amizade novo (ver docs/architecture.md, "Decisão: pedido de
// amizade pela lista de membros").
func EncodeFriendRequest(req FriendRequestView) ([]byte, error) {
	return json.Marshal(friendRequestEnvelope{Type: "friend.request", Request: req})
}

// EncodeFriendRequestRemoved serializa "friend.request.removed", enviado aos
// dois lados quando um pedido pendente é recusado ou cancelado.
func EncodeFriendRequestRemoved(id string) ([]byte, error) {
	return json.Marshal(friendRequestRemovedEnvelope{Type: "friend.request.removed", ID: id})
}

// EncodeFriendAccepted serializa "friend.accepted": accountID (e o nome,
// quando houver perfil) é o novo amigo do ponto de vista de quem recebe o
// frame. requestID é o pedido que deixou de estar pendente, vazio quando a
// amizade veio de um convite.
func EncodeFriendAccepted(requestID, accountID string, displayName *string) ([]byte, error) {
	return json.Marshal(friendAcceptedEnvelope{Type: "friend.accepted", RequestID: requestID, AccountID: accountID, Name: displayName})
}
