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

// Redeem soma 1 ao contador de usos de um convite, de forma atômica: só
// afeta uma linha que ainda não esteja expirada nem tenha atingido max_uses.
// Devolve ErrConflict se outra requisição já tiver esgotado/expirado o
// convite entre o GetByCode do chamador e esta chamada.
func (s *InviteStore) Redeem(ctx context.Context, id string) (Invite, error) {
	const query = `
		UPDATE invites SET uses = uses + 1
		WHERE id = $1
			AND (expires_at IS NULL OR expires_at > now())
			AND (max_uses IS NULL OR uses < max_uses)
		RETURNING id, code, created_by_member_id, max_uses, uses, expires_at, created_at
	`
	invite, err := s.scanOne(ctx, query, id)
	if errors.Is(err, ErrNotFound) {
		return Invite{}, ErrConflict
	}
	return invite, err
}

// Delete revoga um convite. Autorização (ManageInvites, ver
// internal/permissions) é checada por quem chama, não aqui — antes de
// existir o sistema de permissões, esta query exigia ser o criador do
// convite; agora qualquer membro com ManageInvites pode revogar qualquer
// convite, mesmo comportamento do MANAGE_GUILD do Discord sobre convites.
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
