package store

import "time"

// Account é a identidade autenticada via Authentik (OIDC). server-central
// não guarda senha nenhuma, só o vínculo com o "sub" emitido pelo IdP.
type Account struct {
	ID          string
	OIDCSubject string
	CreatedAt   time.Time
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
	AddedAt   time.Time
}
