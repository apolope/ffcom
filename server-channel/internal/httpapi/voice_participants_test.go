package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-channel/internal/livekit"
)

type fakeRooms struct {
	rooms       map[string][]livekit.Participant
	failRoom    string
	listCalls   int
	listRoomErr error
}

func (f *fakeRooms) ListRoomNames(ctx context.Context) ([]string, error) {
	f.listCalls++
	if f.listRoomErr != nil {
		return nil, f.listRoomErr
	}
	names := make([]string, 0, len(f.rooms))
	for name := range f.rooms {
		names = append(names, name)
	}
	return names, nil
}

func (f *fakeRooms) ListParticipants(ctx context.Context, room string) ([]livekit.Participant, error) {
	if room == f.failRoom {
		return nil, errors.New("room not found")
	}
	return f.rooms[room], nil
}

func TestVoicePresenceCachesWithinTTL(t *testing.T) {
	fake := &fakeRooms{rooms: map[string][]livekit.Participant{"c1": {{Identity: "m1", Name: "Ana"}}}}
	p := newVoicePresence(fake, time.Minute)

	for i := 0; i < 3; i++ {
		if _, err := p.get(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if fake.listCalls != 1 {
		t.Fatalf("esperava 1 chamada ao LiveKit dentro do ttl, veio %d", fake.listCalls)
	}

	p.fetchedAt = time.Now().Add(-2 * time.Minute)
	if _, err := p.get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.listCalls != 2 {
		t.Fatalf("esperava nova chamada depois do ttl, veio %d", fake.listCalls)
	}
}

func TestVoicePresenceSkipsEmptyAndFailedRooms(t *testing.T) {
	fake := &fakeRooms{
		rooms: map[string][]livekit.Participant{
			"cheia":  {{Identity: "m1"}, {Identity: "m2"}},
			"vazia":  {},
			"fechou": {{Identity: "m3"}},
		},
		failRoom: "fechou",
	}
	snapshot, err := newVoicePresence(fake, time.Minute).get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != 1 || len(snapshot["cheia"]) != 2 {
		t.Fatalf("esperava só a sala cheia com 2 participantes, veio %+v", snapshot)
	}
}

func TestVoicePresenceDoesNotCacheErrors(t *testing.T) {
	fake := &fakeRooms{listRoomErr: errors.New("connection refused")}
	p := newVoicePresence(fake, time.Minute)
	if _, err := p.get(context.Background()); err == nil {
		t.Fatal("esperava erro com LiveKit fora")
	}
	fake.listRoomErr = nil
	fake.rooms = map[string][]livekit.Participant{"c1": {{Identity: "m1"}}}
	snapshot, err := p.get(context.Background())
	if err != nil || len(snapshot) != 1 {
		t.Fatalf("depois do LiveKit voltar deveria buscar de novo, veio %+v, %v", snapshot, err)
	}
}
