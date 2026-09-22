// Package storage persiste os bytes do avatar de conta no disco local do
// host -- mesma filosofia de dependências enxutas (sem S3/object storage)
// já aplicada ao upload de anexo em server-channel (ver
// server-channel/internal/storage/files.go), mas com uma chave fixa por
// conta em vez de uma chave aleatória por upload: um avatar é sempre
// substituído pelo próximo, nunca acumulado, então o accountId já serve
// como chave -- não precisa rastrear "qual é o arquivo atual" em lugar
// nenhum. Ver docs/architecture.md, "Decisão: upload de avatar de conta".
package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// AvatarStore grava e lê avatares sob um único diretório base, um arquivo
// por conta.
type AvatarStore struct {
	baseDir string
}

// NewAvatarStore garante que baseDir existe (cria se preciso) e devolve um
// AvatarStore pronto para uso.
func NewAvatarStore(baseDir string) (*AvatarStore, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: criar diretório %q: %w", baseDir, err)
	}
	return &AvatarStore{baseDir: baseDir}, nil
}

// Save grava r como o avatar de accountID, substituindo qualquer avatar
// anterior dela. Escreve num arquivo temporário à parte e troca via rename
// atômico, para nunca deixar um arquivo parcial servível em caso de falha
// no meio da escrita.
func (s *AvatarStore) Save(accountID string, r io.Reader) (err error) {
	tmp, err := os.CreateTemp(s.baseDir, filepath.Base(accountID)+".tmp-*")
	if err != nil {
		return fmt.Errorf("storage: criar arquivo temporário de avatar: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	if _, err = io.Copy(tmp, r); err != nil {
		tmp.Close()
		return fmt.Errorf("storage: escrever avatar: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("storage: fechar avatar: %w", err)
	}
	if err = os.Rename(tmpPath, s.path(accountID)); err != nil {
		return fmt.Errorf("storage: substituir avatar: %w", err)
	}
	return nil
}

// Open abre o avatar de accountID para leitura (ex.: servir download) -- o
// chamador é responsável por fechar. Erro satisfaz errors.Is(err,
// fs.ErrNotExist) quando a conta não tem avatar.
func (s *AvatarStore) Open(accountID string) (*os.File, error) {
	file, err := os.Open(s.path(accountID))
	if err != nil {
		return nil, fmt.Errorf("storage: abrir avatar: %w", err)
	}
	return file, nil
}

// Delete remove o avatar de accountID. Não é erro se já não existir --
// idempotente, chamado tanto ao remover o avatar quanto antes de um novo
// upload substituir o anterior.
func (s *AvatarStore) Delete(accountID string) error {
	if err := os.Remove(s.path(accountID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("storage: apagar avatar: %w", err)
	}
	return nil
}

// path nunca deixa accountID sair de baseDir: filepath.Base descarta
// qualquer separador de diretório que accountID viesse a conter (accountID
// sempre vem de auth.AccountFromContext, nunca de input livre do client,
// mas a checagem é defensiva e barata).
func (s *AvatarStore) path(accountID string) string {
	return filepath.Join(s.baseDir, filepath.Base(accountID))
}
