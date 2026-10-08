package realtime

import (
	"testing"

	"a3sitsolutions.com/ffcom/server-channel/internal/apierr"
)

// newTestClient monta um Client sem conexão: Hub e a fila de saída não
// tocam no conn.
func newTestClient() *Client {
	return &Client{send: make(chan []byte, 16)}
}

func TestHubCloseFechaFilasEIgnoraRegistroTardio(t *testing.T) {
	hub := NewHub()
	a, b := newTestClient(), newTestClient()
	hub.Register("c1", a)
	hub.Register("c2", b)

	hub.Close()

	for name, c := range map[string]*Client{"a": a, "b": b} {
		if _, ok := <-c.send; ok {
			t.Fatalf("fila de %s deveria estar fechada", name)
		}
	}

	// SendError e Broadcast depois do Close não podem entrar em pânico
	// (send num canal fechado).
	a.SendError(apierr.New("realtime.rate_limited", "tarde demais"))
	hub.Broadcast("c1", []byte("x"))

	late := newTestClient()
	hub.Register("c1", late)
	if _, ok := <-late.send; ok {
		t.Fatal("client registrado depois do Close deveria ter a fila fechada")
	}
	hub.Unregister("c1", late)
}

func TestHubMemberIDs(t *testing.T) {
	hub := NewHub()
	a, b, anon := newTestClient(), newTestClient(), newTestClient()
	a.MemberID, b.MemberID = "m1", "m2"
	hub.Register("c1", a)
	hub.Register("c2", b)
	hub.Register("c1", anon)

	got := hub.MemberIDs("c1")
	if len(got) != 1 || !got["m1"] {
		t.Fatalf("MemberIDs(c1) = %v, esperado só m1", got)
	}
	hub.Unregister("c1", a)
	if got := hub.MemberIDs("c1"); len(got) != 0 {
		t.Fatalf("depois do Unregister, MemberIDs(c1) = %v", got)
	}
}
