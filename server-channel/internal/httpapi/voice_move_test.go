package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/auth"
	"a3sitsolutions.com/ffcom/server-channel/internal/livekit"
	"a3sitsolutions.com/ffcom/server-channel/internal/permissions"
	"a3sitsolutions.com/ffcom/server-channel/internal/store"
)

type sentData struct {
	room       string
	identities []string
	topic      string
	data       []byte
}

type fakeDataSender struct{ sent []sentData }

func (f *fakeDataSender) SendData(ctx context.Context, room string, identities []string, topic string, data []byte) error {
	f.sent = append(f.sent, sentData{room, identities, topic, data})
	return nil
}

func TestMoveVoiceParticipant(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()

	newMember := func(subject string) store.Member {
		t.Helper()
		m, err := db.Members.GetOrCreateByOIDCSubject(ctx, subject+"-"+t.Name())
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	plain := newMember("plain")
	mover := newMember("mover")
	target := newMember("target")
	role, err := db.Roles.Create(ctx, "movedor-"+t.Name(), nil, permissions.MoveMembers, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Roles.Delete(context.Background(), role.ID) })
	if err := db.Roles.AssignToMember(ctx, mover.ID, role.ID); err != nil {
		t.Fatal(err)
	}

	newVoice := func(name string) store.Channel {
		t.Helper()
		c, err := db.Channels.Create(ctx, nil, name+"-"+t.Name(), store.ChannelVoice, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Channels.Delete(context.Background(), c.ID) })
		return c
	}
	source := newVoice("origem")
	dest := newVoice("destino")
	closed := newVoice("fechada")
	everyone, err := db.Roles.GetDefault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ChannelOverwrites.Set(ctx, closed.ID, everyone.ID, 0, permissions.Voice); err != nil {
		t.Fatal(err)
	}

	rooms := &fakeRooms{rooms: map[string][]livekit.Participant{}}
	sender := &fakeDataSender{}
	h := handleMoveVoiceParticipant(newVoicePresence(rooms, time.Minute), sender, db.Members, db.Channels, db.Roles, db.ChannelOverwrites, "key", "secret-de-teste-com-tamanho-suficiente", "wss://livekit.test")
	do := func(member store.Member, memberID, channelID string) int {
		t.Helper()
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(moveVoiceRequest{MemberID: memberID, ChannelID: channelID})
		req := httptest.NewRequest("POST", "/", &buf).WithContext(auth.WithMember(ctx, member))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do(plain, target.ID, dest.ID); code != http.StatusForbidden {
		t.Errorf("mover sem MoveMembers: status %d, esperado 403", code)
	}
	if code := do(mover, target.ID, dest.ID); code != http.StatusConflict {
		t.Errorf("mover quem não está em sala: status %d, esperado 409", code)
	}
	// A sala fechada nega Voice à @everyone: quem move também não tem, então
	// não pode puxar ninguém para lá.
	if code := do(mover, target.ID, closed.ID); code != http.StatusForbidden {
		t.Errorf("mover para sala onde quem move não tem Voice: status %d, esperado 403", code)
	}

	rooms.rooms[source.ID] = []livekit.Participant{{Identity: target.ID}}
	if code := do(mover, target.ID, dest.ID); code != http.StatusNoContent {
		t.Fatalf("mover com MoveMembers: status %d, esperado 204", code)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("esperava 1 aviso pelo LiveKit, veio %d", len(sender.sent))
	}
	got := sender.sent[0]
	var msg voiceMoveMessage
	json.Unmarshal(got.data, &msg)
	if got.room != source.ID || len(got.identities) != 1 || got.identities[0] != target.ID || got.topic != VoiceMoveTopic || msg.ChannelID != dest.ID {
		t.Errorf("aviso errado: %+v (%s)", got, got.data)
	}

	// Quem move com Voice liberado na sala fechada puxa o alvo para lá, mesmo
	// o alvo não tendo Voice nela.
	if _, err := db.ChannelOverwrites.Set(ctx, closed.ID, role.ID, permissions.Voice, 0); err != nil {
		t.Fatal(err)
	}
	if code := do(mover, target.ID, closed.ID); code != http.StatusNoContent || len(sender.sent) != 2 {
		t.Fatalf("puxar para sala fechada ao alvo: status %d, %d avisos", code, len(sender.sent))
	}
	json.Unmarshal(sender.sent[1].data, &msg)
	if msg.ChannelID != closed.ID || msg.Token == "" || msg.URL != "wss://livekit.test" {
		t.Errorf("aviso para sala fechada sem token/url: %s", sender.sent[1].data)
	}

	// Já está no destino: nada a fazer.
	if code := do(mover, target.ID, source.ID); code != http.StatusNoContent || len(sender.sent) != 2 {
		t.Errorf("mover para a sala em que já está: status %d, %d avisos", code, len(sender.sent))
	}
}
