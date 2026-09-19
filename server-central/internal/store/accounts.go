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
		RETURNING id, oidc_subject, created_at
	`
	var a Account
	err := s.pool.QueryRow(ctx, query, oidcSubject).Scan(&a.ID, &a.OIDCSubject, &a.CreatedAt)
	if err != nil {
		return Account{}, fmt.Errorf("accounts: get or create por subject: %w", err)
	}
	return a, nil
}

func (s *AccountStore) GetByID(ctx context.Context, id string) (Account, error) {
	const query = `SELECT id, oidc_subject, created_at FROM accounts WHERE id = $1`
	var a Account
	err := s.pool.QueryRow(ctx, query, id).Scan(&a.ID, &a.OIDCSubject, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("accounts: get por id: %w", err)
	}
	return a, nil
}
