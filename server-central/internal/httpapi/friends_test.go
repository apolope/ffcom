package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"a3sitsolutions.com/ffcom/server-central/internal/auth"
	"a3sitsolutions.com/ffcom/server-central/internal/realtime"
	"a3sitsolutions.com/ffcom/server-central/internal/store"
)

func TestDeleteFriend(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()

	mux := http.NewServeMux()
	mux.Handle("DELETE /api/friends/{accountId}", handleDeleteFriend(realtime.NewHub(), db.Friendships))

	newAccount := func(name string) store.Account {
		t.Helper()
		account, err := db.Accounts.GetOrCreateBySubject(ctx, fmt.Sprintf("%s-%s-%d", name, t.Name(), time.Now().UnixNano()), nil)
		if err != nil {
			t.Fatal(err)
		}
		return account
	}
	alice, bob, carol := newAccount("alice"), newAccount("bob"), newAccount("carol")

	remove := func(as store.Account, friendID string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodDelete, "/api/friends/"+friendID, nil)
		req = req.WithContext(auth.WithAccount(req.Context(), as))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	areFriends := func(a, b store.Account) bool {
		t.Helper()
		ok, err := db.Friendships.AreFriends(ctx, a.ID, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	// Quem recebeu o pedido também pode desfazer, não só quem pediu.
	if _, err := db.Friendships.CreateAccepted(ctx, alice.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if code := remove(bob, alice.ID); code != http.StatusNoContent {
		t.Fatalf("desfazer: got %d, want 204", code)
	}
	if areFriends(alice, bob) {
		t.Fatal("continuam amigos depois de desfazer")
	}
	if code := remove(alice, bob.ID); code != http.StatusNotFound {
		t.Fatalf("desfazer de novo: got %d, want 404", code)
	}

	// A linha some, então qualquer um pode pedir amizade de novo.
	if _, err := db.Friendships.Request(ctx, alice.ID, bob.ID); err != nil {
		t.Fatalf("pedir de novo depois de desfazer: %v", err)
	}

	// Pedido pendente não é amizade: esta rota não o apaga.
	if code := remove(alice, bob.ID); code != http.StatusNotFound {
		t.Fatalf("pedido pendente: got %d, want 404", code)
	}
	if _, err := db.Friendships.Between(ctx, alice.ID, bob.ID); err != nil {
		t.Fatalf("pedido pendente sumiu: %v", err)
	}

	// Desfazer com um terceiro não mexe em amizade alheia.
	if _, err := db.Friendships.CreateAccepted(ctx, bob.ID, carol.ID); err != nil {
		t.Fatal(err)
	}
	if code := remove(alice, carol.ID); code != http.StatusNotFound {
		t.Fatalf("sem amizade com carol: got %d, want 404", code)
	}
	if !areFriends(bob, carol) {
		t.Fatal("amizade de bob e carol foi afetada")
	}

	if code := remove(alice, "nao-e-uuid"); code != http.StatusNotFound {
		t.Fatalf("id inválido: got %d, want 404", code)
	}
}
