package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FriendshipStore struct {
	pool *pgxpool.Pool
}

// Request cria um pedido de amizade de requesterID para addresseeID.
func (s *FriendshipStore) Request(ctx context.Context, requesterID, addresseeID string) (Friendship, error) {
	const query = `
		INSERT INTO friendships (requester_id, addressee_id, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, requester_id, addressee_id, status, created_at, updated_at
	`
	return s.scanOne(ctx, query, requesterID, addresseeID)
}

// SetStatus atualiza o status de um pedido de amizade existente (ex.: accepted, blocked).
func (s *FriendshipStore) SetStatus(ctx context.Context, id string, status FriendshipStatus) (Friendship, error) {
	const query = `
		UPDATE friendships SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, requester_id, addressee_id, status, created_at, updated_at
	`
	return s.scanOne(ctx, query, id, status)
}

// ListForAccount lista as amizades em que a conta participa, como requester ou addressee.
func (s *FriendshipStore) ListForAccount(ctx context.Context, accountID string) ([]Friendship, error) {
	const query = `
		SELECT id, requester_id, addressee_id, status, created_at, updated_at
		FROM friendships
		WHERE requester_id = $1 OR addressee_id = $1
		ORDER BY updated_at DESC
	`
	rows, err := s.pool.Query(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("friendships: list por account_id: %w", err)
	}
	defer rows.Close()

	var out []Friendship
	for rows.Next() {
		var f Friendship
		if err := rows.Scan(&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("friendships: scan: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("friendships: iterar linhas: %w", err)
	}
	return out, nil
}

// CreateAccepted cria uma amizade já com status "accepted" entre
// requesterID e addresseeID — usado ao resgatar um convite de amizade (ver
// docs/architecture.md, "Decisão: adicionar amigos via convite"):
// resgatar o código já é o consentimento mútuo, não há etapa extra de
// aprovação como em Request/SetStatus. Devolve ErrConflict se já existir
// uma amizade entre as duas contas, em qualquer direção.
func (s *FriendshipStore) CreateAccepted(ctx context.Context, requesterID, addresseeID string) (Friendship, error) {
	const query = `
		INSERT INTO friendships (requester_id, addressee_id, status)
		SELECT $1, $2, 'accepted'
		WHERE NOT EXISTS (
			SELECT 1 FROM friendships
			WHERE (requester_id = $1 AND addressee_id = $2)
			   OR (requester_id = $2 AND addressee_id = $1)
		)
		RETURNING id, requester_id, addressee_id, status, created_at, updated_at
	`
	f, err := s.scanOne(ctx, query, requesterID, addresseeID)
	if errors.Is(err, ErrNotFound) {
		return Friendship{}, ErrConflict
	}
	return f, err
}

// AcceptedFriendIDs lista o account_id do outro lado de cada amizade aceita
// de accountID — usado pelo gateway de presença para saber a quem notificar
// (ou responder, na rota REST) quando o status online de alguém muda.
func (s *FriendshipStore) AcceptedFriendIDs(ctx context.Context, accountID string) ([]string, error) {
	const query = `
		SELECT CASE WHEN requester_id = $1 THEN addressee_id ELSE requester_id END
		FROM friendships
		WHERE status = 'accepted' AND (requester_id = $1 OR addressee_id = $1)
	`
	rows, err := s.pool.Query(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("friendships: accepted friend ids: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var friendID string
		if err := rows.Scan(&friendID); err != nil {
			return nil, fmt.Errorf("friendships: scan friend id: %w", err)
		}
		out = append(out, friendID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("friendships: iterar linhas: %w", err)
	}
	return out, nil
}

func (s *FriendshipStore) scanOne(ctx context.Context, query string, args ...any) (Friendship, error) {
	var f Friendship
	err := s.pool.QueryRow(ctx, query, args...).
		Scan(&f.ID, &f.RequesterID, &f.AddresseeID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Friendship{}, ErrNotFound
	}
	if err != nil {
		return Friendship{}, fmt.Errorf("friendships: query: %w", err)
	}
	return f, nil
}
