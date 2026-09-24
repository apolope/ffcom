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
	// ProfileName é o nome do perfil do Authentik, gravado pelo client
	// (PUT /api/me/profile-name). Aparece quando não há apelido; ver
	// DisplayName.
	ProfileName *string
	JoinedAt    time.Time
	// IsOwner é quem entrou primeiro neste server-channel (bootstrap do
	// self-host, ver internal/httpapi/join.go). Ignora toda checagem de
	// permissão — não é uma role (ver docs/architecture.md, "Sistema de
	// permissões/roles por servidor e por canal").
	IsOwner bool
	// RemovedAt marca quando o membro foi expulso (kick) deste
	// server-channel — ver docs/architecture.md, "Decisão: kick/ban de
	// membro". A linha não é apagada (preserva a autoria de
	// mensagens/threads/convites já existentes); GetByOIDCSubject só
	// devolve membros com RemovedAt nulo, então perder o acesso é
	// imediato na próxima requisição.
	RemovedAt *time.Time
}

// MemberBan é um banimento indexado por oidc_subject (ver
// docs/architecture.md, "Decisão: kick/ban de membro") — sobrevive
// independente de existir ou não uma linha em members para esse subject, e
// bloqueia POST /api/join enquanto existir.
type MemberBan struct {
	OIDCSubject      string
	BannedByMemberID *string
	Reason           *string
	CreatedAt        time.Time
	// LastNickname é o nickname da linha de members com o mesmo
	// oidc_subject, se ainda existir (ver MemberBanStore.List) — só para
	// exibição na UI de administração, já que o próprio oidc_subject não
	// diz nada a um humano. Nulo se a pessoa nunca chegou a ser membro
	// (banimento preventivo) ou nunca definiu apelido.
	LastNickname *string
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

// Role é um papel do servidor. Os bits de Permissions estão definidos em
// internal/permissions. IsDefault marca a role "@everyone", implícita a todo
// membro sem precisar de linha em member_roles — só existe uma por servidor
// (ver migration 0002_roles_permissions).
type Role struct {
	ID          string
	Name        string
	Color       *string
	Permissions int64
	Position    int
	IsDefault   bool
	CreatedAt   time.Time
}

// ChannelRoleOverwrite sobrescreve, só dentro de Channel, bits específicos
// da permissão base de quem tiver Role atribuída (ver internal/permissions,
// Effective).
type ChannelRoleOverwrite struct {
	ChannelID string
	RoleID    string
	Allow     int64
	Deny      int64
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

// Attachment é um arquivo anexado a uma mensagem de canal de texto (ver
// docs/architecture.md, "Decisão: upload de anexo em mensagem"). StorageKey
// identifica o arquivo em internal/storage.FileStore (disco local do host)
// -- nunca é o Filename original enviado pelo client, para nunca abrir
// caminho de path traversal a partir de um nome de arquivo controlado pelo
// usuário.
type Attachment struct {
	ID          string
	MessageID   string
	Filename    string
	ContentType string
	SizeBytes   int64
	StorageKey  string
	CreatedAt   time.Time
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

// DisplayName é o nome exibido do membro: o apelido escolhido neste
// servidor, senão o nome do perfil do Authentik, senão o começo do id (quem
// ainda não abriu o servidor com um client que grava o nome). Ver
// docs/architecture.md, "Decisão: nome exibido do membro".
func (m Member) DisplayName() string {
	if m.Nickname != nil && *m.Nickname != "" {
		return *m.Nickname
	}
	if m.ProfileName != nil && *m.ProfileName != "" {
		return *m.ProfileName
	}
	if len(m.ID) > 8 {
		return m.ID[:8]
	}
	return m.ID
}
