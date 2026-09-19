package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type KnownServerStore struct {
	pool *pgxpool.Pool
}

// Add adiciona um server-channel ao diretório de uma conta (via convite ou
// endereço informado manualmente — não há descoberta automática).
func (s *KnownServerStore) Add(ctx context.Context, accountID, address, name string, iconURL *string) (KnownServer, error) {
	const query = `
		INSERT INTO known_servers (account_id, address, name, icon_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id, address) DO UPDATE
		SET name = EXCLUDED.name, icon_url = EXCLUDED.icon_url
		RETURNING id, account_id, address, name, icon_url, added_at
	`
	var k KnownServer
	err := s.pool.QueryRow(ctx, query, accountID, address, name, iconURL).
		Scan(&k.ID, &k.AccountID, &k.Address, &k.Name, &k.IconURL, &k.AddedAt)
	if err != nil {
		return KnownServer{}, fmt.Errorf("known_servers: add: %w", err)
	}
	return k, nil
}

// Remove tira um server-channel do diretório de uma conta.
func (s *KnownServerStore) Remove(ctx context.Context, accountID, id string) error {
	const query = `DELETE FROM known_servers WHERE id = $1 AND account_id = $2`
	tag, err := s.pool.Exec(ctx, query, id, accountID)
	if err != nil {
		return fmt.Errorf("known_servers: remove: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListForAccount lista o diretório de server-channel conhecidos por uma conta.
func (s *KnownServerStore) ListForAccount(ctx context.Context, accountID string) ([]KnownServer, error) {
	const query = `
		SELECT id, account_id, address, name, icon_url, added_at
		FROM known_servers
		WHERE account_id = $1
		ORDER BY added_at DESC
	`
	rows, err := s.pool.Query(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("known_servers: list por account_id: %w", err)
	}
	defer rows.Close()

	var out []KnownServer
	for rows.Next() {
		var k KnownServer
		if err := rows.Scan(&k.ID, &k.AccountID, &k.Address, &k.Name, &k.IconURL, &k.AddedAt); err != nil {
			return nil, fmt.Errorf("known_servers: scan: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("known_servers: iterar linhas: %w", err)
	}
	return out, nil
}
