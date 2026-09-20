package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FriendInviteStore struct {
	pool *pgxpool.Pool
}

// Create registra um convite de amizade. expiresAt nulo significa "sem
// expiração" — o código só deixa de valer depois de resgatado.
func (s *FriendInviteStore) Create(ctx context.Context, code, createdByAccountID string) (FriendInvite, error) {
	const query = `
		INSERT INTO friend_invites (code, created_by_account_id)
		VALUES ($1, $2)
		RETURNING id, code, created_by_account_id, expires_at, redeemed_by_account_id, redeemed_at, created_at
	`
	return s.scanOne(ctx, query, code, createdByAccountID)
}

func (s *FriendInviteStore) GetByCode(ctx context.Context, code string) (FriendInvite, error) {
	const query = `
		SELECT id, code, created_by_account_id, expires_at, redeemed_by_account_id, redeemed_at, created_at
		FROM friend_invites WHERE code = $1
	`
	return s.scanOne(ctx, query, code)
}

// Redeem marca o convite id como resgatado por redeemedByAccountID, de
// forma atômica: só afeta uma linha que ainda não tenha sido resgatada e
// não esteja expirada. Devolve ErrConflict se outra requisição já tiver
// resgatado o mesmo convite entre o GetByCode do chamador e esta chamada.
func (s *FriendInviteStore) Redeem(ctx context.Context, id, redeemedByAccountID string) (FriendInvite, error) {
	const query = `
		UPDATE friend_invites
		SET redeemed_by_account_id = $2, redeemed_at = now()
		WHERE id = $1 AND redeemed_at IS NULL AND (expires_at IS NULL OR expires_at > now())
		RETURNING id, code, created_by_account_id, expires_at, redeemed_by_account_id, redeemed_at, created_at
	`
	invite, err := s.scanOne(ctx, query, id, redeemedByAccountID)
	if errors.Is(err, ErrNotFound) {
		return FriendInvite{}, ErrConflict
	}
	return invite, err
}

func (s *FriendInviteStore) scanOne(ctx context.Context, query string, args ...any) (FriendInvite, error) {
	var i FriendInvite
	err := s.pool.QueryRow(ctx, query, args...).
		Scan(&i.ID, &i.Code, &i.CreatedByAccountID, &i.ExpiresAt, &i.RedeemedByAccountID, &i.RedeemedAt, &i.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return FriendInvite{}, ErrNotFound
	}
	if err != nil {
		return FriendInvite{}, fmt.Errorf("friend_invites: query: %w", err)
	}
	return i, nil
}
