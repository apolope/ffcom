// Package storage persiste bytes de anexo de mensagem no disco local do
// host (ver docs/architecture.md, "Decisão: upload de anexo em mensagem") --
// sem S3/object storage, mesma filosofia de dependências enxutas já aplicada
// ao resto de server-channel; persistência via volume Docker, mesmo padrão
// já usado pelo Postgres.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileStore grava e lê arquivos sob um único diretório base.
type FileStore struct {
	baseDir string
}

// NewFileStore garante que baseDir existe (cria se preciso) e devolve um
// FileStore pronto para uso.
func NewFileStore(baseDir string) (*FileStore, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: criar diretório %q: %w", baseDir, err)
	}
	return &FileStore{baseDir: baseDir}, nil
}

// Save grava r sob uma chave nova (32 caracteres hex aleatórios, nunca
// derivada do nome de arquivo enviado pelo client) e devolve essa chave.
// Falha ao escrever apaga qualquer arquivo parcial antes de devolver o erro.
func (f *FileStore) Save(r io.Reader) (key string, err error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("storage: gerar chave: %w", err)
	}
	key = hex.EncodeToString(buf)

	path := f.path(key)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("storage: criar arquivo: %w", err)
	}
	if _, err := io.Copy(file, r); err != nil {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("storage: escrever arquivo: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("storage: fechar arquivo: %w", err)
	}
	return key, nil
}

// Open abre o arquivo de key para leitura (ex.: servir download) -- o
// chamador é responsável por fechar.
func (f *FileStore) Open(key string) (*os.File, error) {
	file, err := os.Open(f.path(key))
	if err != nil {
		return nil, fmt.Errorf("storage: abrir arquivo: %w", err)
	}
	return file, nil
}

// Delete remove o arquivo de key. Não é erro se já não existir --
// idempotente, chamado tanto em cleanup de falha de upload quanto ao apagar
// mensagem.
func (f *FileStore) Delete(key string) error {
	if err := os.Remove(f.path(key)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: apagar arquivo: %w", err)
	}
	return nil
}

// path nunca deixa key sair de baseDir: filepath.Base descarta qualquer
// separador de diretório que key viesse a conter (key sempre vem de Save ou
// da coluna storage_key gravada por ela, mas a checagem é defensiva e
// barata).
func (f *FileStore) path(key string) string {
	return filepath.Join(f.baseDir, filepath.Base(key))
}
