package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type KnownServerStore struct {
	pool *pgxpool.Pool
}

// Add adiciona um server-channel ao diretório de uma conta (via convite ou
// endereço informado manualmente — não há descoberta automática). Um
// servidor novo entra no topo do rail (posição menor que todas), como era
// antes de existir ordem manual; readicionar um que já está na lista
// mantém a posição dele.
func (s *KnownServerStore) Add(ctx context.Context, accountID, address, name string, iconURL *string) (KnownServer, error) {
	const query = `
		INSERT INTO known_servers (account_id, address, name, icon_url, position)
		VALUES ($1, $2, $3, $4, (
			SELECT COALESCE(MIN(position) - 1, 0) FROM known_servers WHERE account_id = $1
		))
		ON CONFLICT (account_id, address) DO UPDATE
		SET name = EXCLUDED.name, icon_url = EXCLUDED.icon_url
		RETURNING id, account_id, address, name, icon_url, position, added_at
	`
	var k KnownServer
	err := s.pool.QueryRow(ctx, query, accountID, address, name, iconURL).
		Scan(&k.ID, &k.AccountID, &k.Address, &k.Name, &k.IconURL, &k.Position, &k.AddedAt)
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

// ErrOrderMismatch: a lista passada a Reorder não é exatamente o conjunto
// de servidores da conta (adicionado ou removido em outra aba/dispositivo).
var ErrOrderMismatch = errors.New("known_servers: ids não batem com os servidores da conta")

// Reorder grava a ordem inteira do rail de uma conta (posições 0..n-1 na
// ordem de ids) numa transação, travando as linhas da conta.
func (s *KnownServerStore) Reorder(ctx context.Context, accountID string, ids []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("known_servers: reorder: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `SELECT id FROM known_servers WHERE account_id = $1 FOR UPDATE`, accountID)
	if err != nil {
		return fmt.Errorf("known_servers: reorder: travar: %w", err)
	}
	current := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("known_servers: reorder: scan: %w", err)
		}
		current[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("known_servers: reorder: iterar linhas: %w", err)
	}

	if len(ids) != len(current) {
		return ErrOrderMismatch
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !current[id] || seen[id] {
			return ErrOrderMismatch
		}
		seen[id] = true
	}
	for position, id := range ids {
		if _, err := tx.Exec(ctx, `UPDATE known_servers SET position = $2 WHERE id = $1`, id, position); err != nil {
			return fmt.Errorf("known_servers: reorder: update: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("known_servers: reorder: commit: %w", err)
	}
	return nil
}

// ListForAccount lista o diretório de server-channel conhecidos por uma
// conta, na ordem do rail.
func (s *KnownServerStore) ListForAccount(ctx context.Context, accountID string) ([]KnownServer, error) {
	const query = `
		SELECT id, account_id, address, name, icon_url, position, added_at
		FROM known_servers
		WHERE account_id = $1
		ORDER BY position ASC, added_at DESC
	`
	rows, err := s.pool.Query(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("known_servers: list por account_id: %w", err)
	}
	defer rows.Close()

	var out []KnownServer
	for rows.Next() {
		var k KnownServer
		if err := rows.Scan(&k.ID, &k.AccountID, &k.Address, &k.Name, &k.IconURL, &k.Position, &k.AddedAt); err != nil {
			return nil, fmt.Errorf("known_servers: scan: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("known_servers: iterar linhas: %w", err)
	}
	return out, nil
}
