package storage

import (
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
)

func TestAvatarStoreSaveOpenRoundTrip(t *testing.T) {
	store, err := NewAvatarStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAvatarStore: %v", err)
	}

	if err := store.Save("conta-1", strings.NewReader("bytes do avatar")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	file, err := store.Open("conta-1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()

	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "bytes do avatar" {
		t.Fatalf("conteúdo lido = %q, esperado %q", got, "bytes do avatar")
	}
}

func TestAvatarStoreSaveOverwritesPrevious(t *testing.T) {
	store, err := NewAvatarStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAvatarStore: %v", err)
	}

	if err := store.Save("conta-1", strings.NewReader("primeiro")); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	if err := store.Save("conta-1", strings.NewReader("segundo")); err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	file, err := store.Open("conta-1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()

	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "segundo" {
		t.Fatalf("conteúdo lido = %q, esperado %q (deveria ter sido substituído)", got, "segundo")
	}
}

func TestAvatarStoreDeleteIsIdempotent(t *testing.T) {
	store, err := NewAvatarStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAvatarStore: %v", err)
	}

	if err := store.Save("conta-1", strings.NewReader("x")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete("conta-1"); err != nil {
		t.Fatalf("Delete (existente): %v", err)
	}
	if err := store.Delete("conta-1"); err != nil {
		t.Fatalf("Delete (já apagado, deveria ser idempotente): %v", err)
	}

	if _, err := store.Open("conta-1"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open após Delete deveria falhar com not-exist, veio: %v", err)
	}
}

func TestAvatarStoreOpenMissingReturnsNotExist(t *testing.T) {
	store, err := NewAvatarStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAvatarStore: %v", err)
	}

	if _, err := store.Open("conta-inexistente"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open de conta sem avatar deveria falhar com not-exist, veio: %v", err)
	}
}
