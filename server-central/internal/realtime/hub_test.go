package realtime

import "testing"

func newTestClient() *Client {
	return &Client{send: make(chan []byte, 8)}
}

func TestHubEffectiveStatus(t *testing.T) {
	const id = "conta"
	h := NewHub()

	if got := h.Effective(id); got != StatusOffline {
		t.Fatalf("sem conexão: got %q, want offline", got)
	}

	pc := newTestClient()
	if before, after := h.Register(id, pc, StatusOnline); before != StatusOffline || after != StatusOnline {
		t.Fatalf("primeira conexão: got %q→%q, want offline→online", before, after)
	}

	phone := newTestClient()
	if before, after := h.Register(id, phone, StatusOnline); before != after {
		t.Fatalf("segunda conexão não deveria mudar o status: %q→%q", before, after)
	}

	// Uma conexão ociosa não basta: o outro dispositivo segue em uso.
	if _, after := h.SetIdle(id, phone, true); after != StatusOnline {
		t.Fatalf("uma ociosa de duas: got %q, want online", after)
	}
	if before, after := h.SetIdle(id, pc, true); before != StatusOnline || after != StatusAway {
		t.Fatalf("todas ociosas: got %q→%q, want online→away", before, after)
	}

	// Ocupado não vira ausente por ociosidade.
	if _, after := h.SetChosen(id, StatusBusy); after != StatusBusy {
		t.Fatalf("ocupado ocioso: got %q, want busy", after)
	}
	if _, after := h.SetChosen(id, chosenInvisible); after != StatusOffline {
		t.Fatalf("invisível: got %q, want offline", after)
	}

	h.SetChosen(id, StatusOnline)
	if _, after := h.SetIdle(id, pc, false); after != StatusOnline {
		t.Fatalf("voltou a usar: got %q, want online", after)
	}

	// Sai a conexão em uso: sobra só a ociosa.
	if before, after := h.Unregister(id, pc); before != StatusOnline || after != StatusAway {
		t.Fatalf("sobrou só a ociosa: got %q→%q, want online→away", before, after)
	}
	if _, after := h.Unregister(id, phone); after != StatusOffline {
		t.Fatalf("última conexão: got %q, want offline", after)
	}
}

func TestHubSetChosenOffline(t *testing.T) {
	h := NewHub()
	// Trocar o status sem conexão não cria estado nem muda o visível.
	if before, after := h.SetChosen("conta", StatusBusy); before != StatusOffline || after != StatusOffline {
		t.Fatalf("sem conexão: got %q→%q, want offline→offline", before, after)
	}
}
