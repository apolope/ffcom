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

// Só conta como vendo o canal quem tem pelo menos uma conexão nele marcada
// como ativa. Conexão nova (ou de client antigo, que nunca manda
// "channel.viewing") não conta; anônima também não.
func TestHubViewingMemberIDs(t *testing.T) {
	hub := NewHub()
	phoneTab, desktop, oldClient, otherChannel, anon := newTestClient(), newTestClient(), newTestClient(), newTestClient(), newTestClient()
	phoneTab.MemberID, desktop.MemberID = "m1", "m1"
	oldClient.MemberID = "m2"
	otherChannel.MemberID = "m3"
	hub.Register("c1", phoneTab)
	hub.Register("c1", desktop)
	hub.Register("c1", oldClient)
	hub.Register("c2", otherChannel)
	hub.Register("c1", anon)

	expect := func(step string, want ...string) {
		t.Helper()
		got := hub.ViewingMemberIDs("c1")
		if len(got) != len(want) {
			t.Fatalf("%s: ViewingMemberIDs(c1) = %v, esperado %v", step, got, want)
		}
		for _, id := range want {
			if !got[id] {
				t.Fatalf("%s: ViewingMemberIDs(c1) = %v, esperado %v", step, got, want)
			}
		}
	}

	// Conectado mas sem dizer nada: ninguém está vendo.
	expect("só conectados")

	// Uma sessão de m1 ativa basta, mesmo com a outra escondida.
	hub.SetViewing("c1", desktop, true)
	hub.SetViewing("c1", phoneTab, false)
	expect("desktop ativo", "m1")

	// Ativo em outro canal não conta neste; anônimo ativo também não.
	hub.SetViewing("c2", otherChannel, true)
	hub.SetViewing("c1", anon, true)
	expect("outro canal e anônimo", "m1")

	// SetViewing com o canal errado não mexe na conexão.
	hub.SetViewing("c2", phoneTab, true)
	hub.SetViewing("c1", desktop, false)
	expect("desktop escondido")

	// Duas sessões ativas, uma escondida: continua vendo até as duas saírem.
	hub.SetViewing("c1", desktop, true)
	hub.SetViewing("c1", phoneTab, true)
	hub.SetViewing("c1", desktop, false)
	expect("só a aba ativa", "m1")
	hub.Unregister("c1", phoneTab)
	expect("aba ativa fechada")

	// Reconectar começa sem ver de novo.
	reconnected := newTestClient()
	reconnected.MemberID = "m1"
	hub.Unregister("c1", desktop)
	hub.Register("c1", reconnected)
	expect("reconectou")
	if got := hub.ViewingMemberIDs("c2"); len(got) != 1 || !got["m3"] {
		t.Fatalf("ViewingMemberIDs(c2) = %v, esperado só m3", got)
	}
}
