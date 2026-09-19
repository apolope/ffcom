package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProfileStore struct {
	pool *pgxpool.Pool
}

func (s *ProfileStore) GetByAccountID(ctx context.Context, accountID string) (Profile, error) {
	const query = `
		SELECT account_id, display_name, avatar_url, updated_at
		FROM profiles WHERE account_id = $1
	`
	var p Profile
	err := s.pool.QueryRow(ctx, query, accountID).Scan(&p.AccountID, &p.DisplayName, &p.AvatarURL, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("profiles: get por account_id: %w", err)
	}
	return p, nil
}

// Upsert cria ou atualiza o perfil de uma conta.
func (s *ProfileStore) Upsert(ctx context.Context, accountID, displayName string, avatarURL *string) (Profile, error) {
	const query = `
		INSERT INTO profiles (account_id, display_name, avatar_url, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (account_id) DO UPDATE
		SET display_name = EXCLUDED.display_name,
		    avatar_url = EXCLUDED.avatar_url,
		    updated_at = now()
		RETURNING account_id, display_name, avatar_url, updated_at
	`
	var p Profile
	err := s.pool.QueryRow(ctx, query, accountID, displayName, avatarURL).
		Scan(&p.AccountID, &p.DisplayName, &p.AvatarURL, &p.UpdatedAt)
	if err != nil {
		return Profile{}, fmt.Errorf("profiles: upsert: %w", err)
	}
	return p, nil
}
