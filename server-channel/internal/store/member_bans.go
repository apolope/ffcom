package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemberBanStore administra a lista de banimentos por oidc_subject — ver
// docs/architecture.md, "Decisão: kick/ban de membro".
type MemberBanStore struct {
	pool *pgxpool.Pool
}

// Create registra um banimento. Idempotente: banir de novo quem já está
// banido só atualiza reason/banned_by/created_at, sem erro de conflito.
func (s *MemberBanStore) Create(ctx context.Context, oidcSubject, bannedByMemberID string, reason *string) (MemberBan, error) {
	const query = `
		INSERT INTO member_bans (oidc_subject, banned_by_member_id, reason)
		VALUES ($1, $2, $3)
		ON CONFLICT (oidc_subject) DO UPDATE SET
			banned_by_member_id = EXCLUDED.banned_by_member_id,
			reason = EXCLUDED.reason,
			created_at = now()
		RETURNING oidc_subject, banned_by_member_id, reason, created_at
	`
	return s.scanOne(ctx, query, oidcSubject, bannedByMemberID, reason)
}

// IsBanned confere se oidcSubject está na lista de banimentos — checado por
// POST /api/join antes de deixar alguém (re)entrar no servidor.
func (s *MemberBanStore) IsBanned(ctx context.Context, oidcSubject string) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM member_bans WHERE oidc_subject = $1)`
	var banned bool
	if err := s.pool.QueryRow(ctx, query, oidcSubject).Scan(&banned); err != nil {
		return false, fmt.Errorf("member_bans: is banned: %w", err)
	}
	return banned, nil
}

// Delete revoga um banimento (unban). Devolve ErrNotFound se oidcSubject
// não estava banido.
func (s *MemberBanStore) Delete(ctx context.Context, oidcSubject string) error {
	const query = `DELETE FROM member_bans WHERE oidc_subject = $1`
	tag, err := s.pool.Exec(ctx, query, oidcSubject)
	if err != nil {
		return fmt.Errorf("member_bans: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List lista todos os banimentos ativos, para a UI de administração
// revisar/revogar. LEFT JOIN com members traz o nickname que a pessoa
// tinha (se a linha de members ainda existir, ver "Decisão: kick/ban de
// membro") só para dar contexto humano ao oidc_subject na UI.
func (s *MemberBanStore) List(ctx context.Context) ([]MemberBan, error) {
	const query = `
		SELECT b.oidc_subject, b.banned_by_member_id, b.reason, b.created_at, m.nickname
		FROM member_bans b
		LEFT JOIN members m ON m.oidc_subject = b.oidc_subject
		ORDER BY b.created_at DESC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("member_bans: list: %w", err)
	}
	defer rows.Close()

	var out []MemberBan
	for rows.Next() {
		var b MemberBan
		if err := rows.Scan(&b.OIDCSubject, &b.BannedByMemberID, &b.Reason, &b.CreatedAt, &b.LastNickname); err != nil {
			return nil, fmt.Errorf("member_bans: scan: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("member_bans: iterar linhas: %w", err)
	}
	return out, nil
}

func (s *MemberBanStore) scanOne(ctx context.Context, query string, args ...any) (MemberBan, error) {
	var b MemberBan
	err := s.pool.QueryRow(ctx, query, args...).Scan(&b.OIDCSubject, &b.BannedByMemberID, &b.Reason, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return MemberBan{}, ErrNotFound
	}
	if err != nil {
		return MemberBan{}, fmt.Errorf("member_bans: query: %w", err)
	}
	return b, nil
}
