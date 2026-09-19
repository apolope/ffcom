package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InviteStore struct {
	pool *pgxpool.Pool
}

// Create registra um convite. maxUses e expiresAt nulos significam "sem limite".
func (s *InviteStore) Create(ctx context.Context, code, createdByMemberID string, maxUses *int, expiresAt *time.Time) (Invite, error) {
	const query = `
		INSERT INTO invites (code, created_by_member_id, max_uses, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, code, created_by_member_id, max_uses, uses, expires_at, created_at
	`
	return s.scanOne(ctx, query, code, createdByMemberID, maxUses, expiresAt)
}

func (s *InviteStore) GetByCode(ctx context.Context, code string) (Invite, error) {
	const query = `
		SELECT id, code, created_by_member_id, max_uses, uses, expires_at, created_at
		FROM invites WHERE code = $1
	`
	return s.scanOne(ctx, query, code)
}

// IncrementUses soma 1 ao contador de usos de um convite (ex.: ao aceitar o convite).
func (s *InviteStore) IncrementUses(ctx context.Context, id string) (Invite, error) {
	const query = `
		UPDATE invites SET uses = uses + 1
		WHERE id = $1
		RETURNING id, code, created_by_member_id, max_uses, uses, expires_at, created_at
	`
	return s.scanOne(ctx, query, id)
}

// Delete revoga um convite.
func (s *InviteStore) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM invites WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("invites: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List lista todos os convites ativos do servidor.
func (s *InviteStore) List(ctx context.Context) ([]Invite, error) {
	const query = `
		SELECT id, code, created_by_member_id, max_uses, uses, expires_at, created_at
		FROM invites ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("invites: list: %w", err)
	}
	defer rows.Close()

	var out []Invite
	for rows.Next() {
		var i Invite
		if err := rows.Scan(&i.ID, &i.Code, &i.CreatedByMemberID, &i.MaxUses, &i.Uses, &i.ExpiresAt, &i.CreatedAt); err != nil {
			return nil, fmt.Errorf("invites: scan: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("invites: iterar linhas: %w", err)
	}
	return out, nil
}

func (s *InviteStore) scanOne(ctx context.Context, query string, args ...any) (Invite, error) {
	var i Invite
	err := s.pool.QueryRow(ctx, query, args...).
		Scan(&i.ID, &i.Code, &i.CreatedByMemberID, &i.MaxUses, &i.Uses, &i.ExpiresAt, &i.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, fmt.Errorf("invites: query: %w", err)
	}
	return i, nil
}
