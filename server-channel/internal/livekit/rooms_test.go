package livekit

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Roda só com um LiveKit de verdade no ar, ex.:
//
//	docker run --rm -p 17880:7880 -e LIVEKIT_KEYS="devkey: devsecretdevsecretdevsecretdevsecret" \
//	  livekit/livekit-server:latest --dev --bind 0.0.0.0
//	FFCOM_TEST_LIVEKIT_URL=http://127.0.0.1:17880 FFCOM_TEST_LIVEKIT_KEY=devkey \
//	  FFCOM_TEST_LIVEKIT_SECRET=devsecretdevsecretdevsecretdevsecret go test ./internal/livekit/
func TestRoomClientAgainstLiveKit(t *testing.T) {
	baseURL := os.Getenv("FFCOM_TEST_LIVEKIT_URL")
	if baseURL == "" {
		t.Skip("FFCOM_TEST_LIVEKIT_URL não definida")
	}
	key, secret := os.Getenv("FFCOM_TEST_LIVEKIT_KEY"), os.Getenv("FFCOM_TEST_LIVEKIT_SECRET")
	room := "canal-teste-" + time.Now().Format("150405.000")

	// Um participante entra pela sinalização com o mesmo token que o client
	// recebe de POST /api/channels/{id}/voice/token.
	token, err := NewAccessToken(key, secret, "member-1", "Ana", room)
	if err != nil {
		t.Fatal(err)
	}
	wsURL := strings.Replace(baseURL, "http", "ws", 1) + "/rtc?protocol=15&access_token=" + token
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("participante não conectou: %v", err)
	}
	defer conn.Close()
	if _, _, err := conn.ReadMessage(); err != nil { // JoinResponse
		t.Fatal(err)
	}

	c := NewRoomClient(baseURL, key, secret)
	ctx := context.Background()

	names, err := c.ListRoomNames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range names {
		found = found || n == room
	}
	if !found {
		t.Fatalf("sala %s não apareceu em %v", room, names)
	}

	participants, err := c.ListParticipants(ctx, room)
	if err != nil {
		t.Fatal(err)
	}
	if len(participants) != 1 || participants[0].Identity != "member-1" || participants[0].Name != "Ana" {
		t.Fatalf("participantes inesperados: %+v", participants)
	}

	if _, err := NewRoomClient(baseURL, key, "segredo-errado-segredo-errado-xx").ListRoomNames(ctx); err == nil {
		t.Fatal("esperava erro com segredo errado")
	}
}

func TestAPIURLFromPublicURL(t *testing.T) {
	cases := map[string]string{
		"wss://livekit.exemplo.com": "https://livekit.exemplo.com",
		"ws://localhost:7880":       "http://localhost:7880",
		"http://livekit:7880":       "http://livekit:7880",
	}
	for in, want := range cases {
		if got := APIURLFromPublicURL(in); got != want {
			t.Errorf("APIURLFromPublicURL(%q) = %q, esperava %q", in, got, want)
		}
	}
}
