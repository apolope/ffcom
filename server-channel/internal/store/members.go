package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemberStore struct {
	pool *pgxpool.Pool
}

// GetOrCreateByOIDCSubject busca o membro vinculado a um "sub" da mesma
// instância central de Authentik usada por server-central, criando-o na
// primeira vez que esse "sub" aparece neste server-channel.
func (s *MemberStore) GetOrCreateByOIDCSubject(ctx context.Context, oidcSubject string) (Member, error) {
	const query = `
		INSERT INTO members (oidc_subject)
		VALUES ($1)
		ON CONFLICT (oidc_subject) DO UPDATE SET oidc_subject = EXCLUDED.oidc_subject
		RETURNING id, oidc_subject, nickname, joined_at
	`
	var m Member
	err := s.pool.QueryRow(ctx, query, oidcSubject).Scan(&m.ID, &m.OIDCSubject, &m.Nickname, &m.JoinedAt)
	if err != nil {
		return Member{}, fmt.Errorf("members: get or create por subject: %w", err)
	}
	return m, nil
}

func (s *MemberStore) GetByID(ctx context.Context, id string) (Member, error) {
	const query = `SELECT id, oidc_subject, nickname, joined_at FROM members WHERE id = $1`
	var m Member
	err := s.pool.QueryRow(ctx, query, id).Scan(&m.ID, &m.OIDCSubject, &m.Nickname, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("members: get por id: %w", err)
	}
	return m, nil
}

// SetNickname define ou limpa (nickname == nil) o apelido do membro neste servidor.
func (s *MemberStore) SetNickname(ctx context.Context, id string, nickname *string) (Member, error) {
	const query = `
		UPDATE members SET nickname = $2
		WHERE id = $1
		RETURNING id, oidc_subject, nickname, joined_at
	`
	var m Member
	err := s.pool.QueryRow(ctx, query, id, nickname).Scan(&m.ID, &m.OIDCSubject, &m.Nickname, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("members: set nickname: %w", err)
	}
	return m, nil
}

// List lista todos os membros do servidor.
func (s *MemberStore) List(ctx context.Context) ([]Member, error) {
	const query = `SELECT id, oidc_subject, nickname, joined_at FROM members ORDER BY joined_at ASC`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("members: list: %w", err)
	}
	defer rows.Close()

	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.OIDCSubject, &m.Nickname, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("members: scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("members: iterar linhas: %w", err)
	}
	return out, nil
}
