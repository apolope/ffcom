package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound é devolvido pelos repositórios quando a linha buscada não existe.
var ErrNotFound = errors.New("store: registro não encontrado")

// ErrConflict é devolvido pelos repositórios quando a operação esbarra num
// estado já existente (ex.: amizade duplicada, convite já resgatado).
var ErrConflict = errors.New("store: conflito, registro já existe")

type AccountStore struct {
	pool *pgxpool.Pool
}

// GetOrCreateBySubject busca a conta vinculada a um "sub" do Authentik,
// criando-a no primeiro login se ainda não existir.
func (s *AccountStore) GetOrCreateBySubject(ctx context.Context, oidcSubject string) (Account, error) {
	const query = `
		INSERT INTO accounts (oidc_subject)
		VALUES ($1)
		ON CONFLICT (oidc_subject) DO UPDATE SET oidc_subject = EXCLUDED.oidc_subject
		RETURNING id, oidc_subject, created_at, e2e_public_key
	`
	var a Account
	err := s.pool.QueryRow(ctx, query, oidcSubject).Scan(&a.ID, &a.OIDCSubject, &a.CreatedAt, &a.E2EPublicKey)
	if err != nil {
		return Account{}, fmt.Errorf("accounts: get or create por subject: %w", err)
	}
	return a, nil
}

func (s *AccountStore) GetByID(ctx context.Context, id string) (Account, error) {
	const query = `SELECT id, oidc_subject, created_at, e2e_public_key FROM accounts WHERE id = $1`
	var a Account
	err := s.pool.QueryRow(ctx, query, id).Scan(&a.ID, &a.OIDCSubject, &a.CreatedAt, &a.E2EPublicKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("accounts: get por id: %w", err)
	}
	return a, nil
}

// GetManyByIDs busca várias contas de uma vez (ex.: juntar a chave pública
// de E2E de cada amigo em GET /api/friends). Ids sem conta correspondente
// simplesmente não aparecem no mapa devolvido -- não é erro (mesmo padrão de
// ProfileStore.GetManyByAccountIDs).
func (s *AccountStore) GetManyByIDs(ctx context.Context, ids []string) (map[string]Account, error) {
	out := make(map[string]Account, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	const query = `SELECT id, oidc_subject, created_at, e2e_public_key FROM accounts WHERE id = ANY($1)`
	rows, err := s.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("accounts: get many por id: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.OIDCSubject, &a.CreatedAt, &a.E2EPublicKey); err != nil {
			return nil, fmt.Errorf("accounts: scan: %w", err)
		}
		out[a.ID] = a
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("accounts: iterar linhas: %w", err)
	}
	return out, nil
}

// SetE2EPublicKey grava a chave pública X25519 (32 bytes) do dispositivo que
// está publicando -- ver docs/architecture.md, "Decisão: criptografia
// ponta-a-ponta em DMs". Sobrescreve qualquer chave anterior da conta (só um
// dispositivo "ativo" por vez nesta v1, sem sincronização multi-dispositivo).
func (s *AccountStore) SetE2EPublicKey(ctx context.Context, accountID string, key []byte) error {
	const query = `UPDATE accounts SET e2e_public_key = $2 WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, accountID, key)
	if err != nil {
		return fmt.Errorf("accounts: set e2e public key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
