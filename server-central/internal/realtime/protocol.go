package realtime

import "encoding/json"

// presenceUpdateEnvelope é o único frame que o gateway de presença emite:
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
