package storage

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestFileStoreSaveOpenRoundTrip(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	key, err := store.Save(strings.NewReader("conteúdo do anexo"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	file, err := store.Open(key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer file.Close()

	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "conteúdo do anexo" {
		t.Fatalf("conteúdo lido = %q, esperado %q", got, "conteúdo do anexo")
	}
}

func TestFileStoreSaveGeneratesDistinctKeys(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	key1, err := store.Save(strings.NewReader("a"))
	if err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	key2, err := store.Save(strings.NewReader("b"))
	if err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	if key1 == key2 {
		t.Fatalf("Save devolveu a mesma chave duas vezes: %q", key1)
	}
}

func TestFileStoreDeleteIsIdempotent(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	key, err := store.Save(strings.NewReader("x"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete(key); err != nil {
		t.Fatalf("Delete (existente): %v", err)
	}
	if err := store.Delete(key); err != nil {
		t.Fatalf("Delete (já apagado, deveria ser idempotente): %v", err)
	}

	if _, err := store.Open(key); !os.IsNotExist(unwrapPathError(err)) {
		t.Fatalf("Open após Delete deveria falhar com not-exist, veio: %v", err)
	}
}

// unwrapPathError extrai o erro de sistema de dentro do fmt.Errorf("...: %w", err)
// que FileStore.Open devolve, para poder checar os.IsNotExist.
func unwrapPathError(err error) error {
	type unwrapper interface{ Unwrap() error }
	for {
		u, ok := err.(unwrapper)
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
}
