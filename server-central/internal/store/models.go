package store

import "time"

// Account é a identidade autenticada via Authentik (OIDC). server-central
// não guarda senha nenhuma, só o vínculo com o "sub" emitido pelo IdP.
type Account struct {
	ID           string
	OIDCSubject  string
	CreatedAt    time.Time
	E2EPublicKey []byte
	// Status escolhido pela pessoa (PresenceStatus*), não o que os amigos
	// veem: esse depende também das conexões abertas (ver realtime.Hub).
	PresenceStatus string
}

// Status de presença escolhidos pela pessoa (coluna accounts.presence_status).
const (
	PresenceOnline    = "online"
	PresenceBusy      = "busy"
	PresenceAway      = "away"
	PresenceInvisible = "invisible"
)

// ValidPresenceStatus diz se status é um dos que a pessoa pode escolher.
func ValidPresenceStatus(status string) bool {
	switch status {
	case PresenceOnline, PresenceBusy, PresenceAway, PresenceInvisible:
		return true
	}
	return false
}

// AccountSummary é o que qualquer conta autenticada pode descobrir de outra a
// partir do oidc_subject (ver AccountStore.GetManyBySubjects): o vínculo com
// a conta e o avatar, sem status de presença, que continua só para amigos.
type AccountSummary struct {
	AccountID   string
	OIDCSubject string
	DisplayName *string
	AvatarURL   *string
}

// Profile são os dados editáveis pelo usuário, associados 1:1 a uma Account.
type Profile struct {
	AccountID   string
	DisplayName string
	AvatarURL   *string
	UpdatedAt   time.Time
}

// FriendshipStatus é o estado de uma relação de amizade entre duas contas.
type FriendshipStatus string

const (
	FriendshipPending  FriendshipStatus = "pending"
	FriendshipAccepted FriendshipStatus = "accepted"
	FriendshipBlocked  FriendshipStatus = "blocked"
)

// Friendship é uma relação direcionada requester -> addressee.
type Friendship struct {
	ID          string
	RequesterID string
	AddresseeID string
	Status      FriendshipStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// KnownServer é uma entrada no diretório de server-channel conhecidos por
// uma conta. Não há descoberta automática (ver docs/architecture.md); a
// entrada vem de convite ou endereço adicionado manualmente.
type KnownServer struct {
	ID        string
	AccountID string
	Address   string
	Name      string
	IconURL   *string
	Position  int
	AddedAt   time.Time
}

// DirectMessage é uma mensagem trocada diretamente entre duas contas, sem
// passar por nenhum server-channel (ver docs/architecture.md, "Decisão:
// modelo de DMs"). Ciphertext/Nonce são opacos ao server-central --
// criptografia ponta-a-ponta (NaCl box), ver docs/architecture.md, "Decisão:
// criptografia ponta-a-ponta em DMs".
type DirectMessage struct {
	ID          string
	SenderID    string
	RecipientID string
	Ciphertext  []byte
	Nonce       []byte
	CreatedAt   time.Time
	EditedAt    *time.Time
}

// FriendInvite é um código de uso único gerado por uma conta para outra
// resgatar e virar amiga (ver docs/architecture.md, "Decisão: adicionar
// amigos via convite"). RedeemedByAccountID/RedeemedAt ficam nulos até o
// código ser resgatado; depois disso o código não pode ser usado de novo.
type FriendInvite struct {
	ID                  string
	Code                string
	CreatedByAccountID  string
	ExpiresAt           *time.Time
	RedeemedByAccountID *string
	RedeemedAt          *time.Time
	CreatedAt           time.Time
}
