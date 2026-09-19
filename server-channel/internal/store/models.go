package store

import "time"

// Member é a identidade de um usuário dentro deste server-channel. Não há
// canal direto com server-central (ver docs/architecture.md), então o
// vínculo é direto com o "sub" da mesma instância central de Authentik.
// nickname é o apelido específico deste servidor, distinto do display_name
// global de server-central.
type Member struct {
	ID          string
	OIDCSubject string
	Nickname    *string
	JoinedAt    time.Time
}

// Category agrupa canais. Não existe tabela "servers": cada server-channel
// representa uma única comunidade.
type Category struct {
	ID        string
	Name      string
	Position  int
	CreatedAt time.Time
}

// ChannelType é o tipo de um canal (ver docs/architecture.md, decisão do LiveKit).
type ChannelType string

const (
	ChannelText  ChannelType = "text"
	ChannelVoice ChannelType = "voice"
	ChannelForum ChannelType = "forum"
)

// Channel pode existir fora de qualquer categoria (CategoryID nulo).
type Channel struct {
	ID         string
	CategoryID *string
	Name       string
	Type       ChannelType
	Position   int
	CreatedAt  time.Time
}

// Role é um papel do servidor. Os bits de Permissions ainda não têm
// significado definido — isso é o TODO separado "Sistema de
// permissões/roles por servidor e por canal".
type Role struct {
	ID          string
	Name        string
	Color       *string
	Permissions int64
	Position    int
	CreatedAt   time.Time
}

// Thread é uma discussão dentro de um canal do tipo forum.
type Thread struct {
	ID             string
	ChannelID      string
	Title          string
	AuthorMemberID string
	CreatedAt      time.Time
}

// Message cobre tanto mensagens de canal de texto (ThreadID nulo) quanto
// posts dentro de uma thread de forum (ThreadID preenchido).
type Message struct {
	ID             string
	ChannelID      string
	ThreadID       *string
	AuthorMemberID string
	Content        string
	CreatedAt      time.Time
	EditedAt       *time.Time
}

// Invite é um código de convite gerado por um membro (ver
// docs/architecture.md, "descoberta de server-channel: nenhuma, apenas
// convite ou IP manual"). MaxUses e ExpiresAt nulos significam "sem limite".
type Invite struct {
	ID                string
	Code              string
	CreatedByMemberID string
	MaxUses           *int
	Uses              int
	ExpiresAt         *time.Time
	CreatedAt         time.Time
}
