package livekit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// RoomClient fala com a RoomService do LiveKit (Twirp, JSON sobre HTTP) sem
// o SDK oficial, pelo mesmo motivo de token.go: só precisamos de duas
// chamadas de leitura. Formato das respostas conferido contra
// livekit-server 1.13.7 (campos em snake_case). Ver docs/architecture.md,
// "Decisão: participantes da sala de voz na barra lateral".
type RoomClient struct {
	baseURL   string
	apiKey    string
	apiSecret string
	http      *http.Client
}

// NewRoomClient recebe a URL HTTP(S) da API do LiveKit (LIVEKIT_API_URL, ou
// derivada de LIVEKIT_PUBLIC_URL, ver APIURLFromPublicURL).
func NewRoomClient(baseURL, apiKey, apiSecret string) *RoomClient {
	return &RoomClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		apiSecret: apiSecret,
		http:      &http.Client{Timeout: 5 * time.Second},
	}
}

// APIURLFromPublicURL troca ws:// por http:// e wss:// por https://. É o
// fallback quando LIVEKIT_API_URL não está definida: funciona em qualquer
// instalação, só dá a volta pelo proxy reverso em vez de ir direto ao
// container.
func APIURLFromPublicURL(publicURL string) string {
	switch {
	case strings.HasPrefix(publicURL, "wss://"):
		return "https://" + strings.TrimPrefix(publicURL, "wss://")
	case strings.HasPrefix(publicURL, "ws://"):
		return "http://" + strings.TrimPrefix(publicURL, "ws://")
	}
	return publicURL
}

// Participant é o subconjunto de ParticipantInfo que o FFCom usa. Identity
// é o member.ID de server-channel (ver NewAccessToken).
type Participant struct {
	Identity string
	Name     string
}

// ListRoomNames devolve o nome das salas abertas agora. Nome da sala é o id
// do canal de voz. O resto de Room é descartado de propósito: inclui
// turn_password, que não pode sair daqui.
func (c *RoomClient) ListRoomNames(ctx context.Context) ([]string, error) {
	var resp struct {
		Rooms []struct {
			Name string `json:"name"`
		} `json:"rooms"`
	}
	if err := c.call(ctx, "ListRooms", adminGrant{RoomList: true}, map[string]any{}, &resp); err != nil {
		return nil, err
	}
	names := make([]string, len(resp.Rooms))
	for i, r := range resp.Rooms {
		names[i] = r.Name
	}
	return names, nil
}

// ListParticipants devolve quem está na sala, sem participantes ocultos
// (grant hidden) nem os que já saíram (state DISCONNECTED).
func (c *RoomClient) ListParticipants(ctx context.Context, room string) ([]Participant, error) {
	var resp struct {
		Participants []struct {
			Identity   string `json:"identity"`
			Name       string `json:"name"`
			State      string `json:"state"`
			Permission *struct {
				Hidden bool `json:"hidden"`
			} `json:"permission"`
		} `json:"participants"`
	}
	if err := c.call(ctx, "ListParticipants", adminGrant{RoomAdmin: true, Room: room}, map[string]string{"room": room}, &resp); err != nil {
		return nil, err
	}
	out := make([]Participant, 0, len(resp.Participants))
	for _, p := range resp.Participants {
		if p.State == "DISCONNECTED" || (p.Permission != nil && p.Permission.Hidden) {
			continue
		}
		out = append(out, Participant{Identity: p.Identity, Name: p.Name})
	}
	return out, nil
}

// adminGrant é o grant de administração da RoomService: roomList para
// ListRooms, roomAdmin + room para operações numa sala específica.
type adminGrant struct {
	RoomList  bool   `json:"roomList,omitempty"`
	RoomAdmin bool   `json:"roomAdmin,omitempty"`
	Room      string `json:"room,omitempty"`
}

type adminClaims struct {
	jwt.RegisteredClaims
	Video adminGrant `json:"video"`
}

func (c *RoomClient) call(ctx context.Context, method string, grant adminGrant, body, out any) error {
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, adminClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.apiKey,
			Subject:   "server-channel",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
		},
		Video: grant,
	}).SignedString([]byte(c.apiSecret))
	if err != nil {
		return fmt.Errorf("livekit: assinar token de admin: %w", err)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/twirp/livekit.RoomService/"+method, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("livekit: %s: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("livekit: %s: status %d: %s", method, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
