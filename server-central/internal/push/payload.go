package push

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Formato dos dados das mensagens FCM, lido pelo FirebaseMessagingService
// do app Android. É contrato: mudar um nome de campo ou de tipo quebra o
// app instalado. Campo novo é compatível (o app ignora o que não conhece).
// Documentado em docs/protocol.md, "Notificações push".
const (
	// SchemaVersion vai no campo "v" de toda mensagem.
	SchemaVersion = "1"

	TypeChannelMessage = "channel_message"
	TypeDM             = "dm"
	TypeFriendRequest  = "friend_request"
	TypeFriendAccepted = "friend_accepted"

	// Limites, em caracteres, do que vai na notificação. O FCM aceita até
	// 4 KB de dados; a notificação mostra bem menos que isso.
	MaxTextRunes   = 300
	MaxNameRunes   = 100
	MaxAuthorRunes = 64
	MaxIDRunes     = 64
)

// Base devolve os campos comuns a todo tipo: v, type e sentAt.
func Base(kind string) map[string]string {
	return map[string]string{
		"v":      SchemaVersion,
		"type":   kind,
		"sentAt": time.Now().UTC().Format(time.RFC3339),
	}
}

// Truncate corta s em max caracteres (runas), terminando com "…" quando
// corta, e tira espaços das pontas.
func Truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:max-1])) + "…"
}
