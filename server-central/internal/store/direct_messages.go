package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DirectMessageStore struct {
	pool *pgxpool.Pool
}

// Create grava uma DM de senderID para recipientID. ciphertext/nonce são
// opacos ao server-central (criptografia ponta-a-ponta, ver
// docs/architecture.md).
func (s *DirectMessageStore) Create(ctx context.Context, senderID, recipientID string, ciphertext, nonce []byte) (DirectMessage, error) {
	const query = `
		INSERT INTO direct_messages (sender_id, recipient_id, ciphertext, nonce)
		VALUES ($1, $2, $3, $4)
		RETURNING id, sender_id, recipient_id, ciphertext, nonce, created_at, edited_at
	`
	var m DirectMessage
	err := s.pool.QueryRow(ctx, query, senderID, recipientID, ciphertext, nonce).
		Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Ciphertext, &m.Nonce, &m.CreatedAt, &m.EditedAt)
	if err != nil {
		return DirectMessage{}, fmt.Errorf("direct_messages: create: %w", err)
	}
	return m, nil
}

// ListConversation devolve o histórico paginado entre accountA e accountB,
// nos dois sentidos, mais recentes primeiro (keyset pagination por
// created_at — mesmo padrão de MessageStore.ListForChannel em
// server-channel).
func (s *DirectMessageStore) ListConversation(ctx context.Context, accountA, accountB string, before *time.Time, limit int) ([]DirectMessage, error) {
	const query = `
		SELECT id, sender_id, recipient_id, ciphertext, nonce, created_at, edited_at
		FROM direct_messages
		WHERE ((sender_id = $1 AND recipient_id = $2) OR (sender_id = $2 AND recipient_id = $1))
		AND ($3::timestamptz IS NULL OR created_at < $3)
		ORDER BY created_at DESC
		LIMIT $4
	`
	rows, err := s.pool.Query(ctx, query, accountA, accountB, before, limit)
	if err != nil {
		return nil, fmt.Errorf("direct_messages: list conversation: %w", err)
	}
	defer rows.Close()

	var out []DirectMessage
	for rows.Next() {
		var m DirectMessage
		if err := rows.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.Ciphertext, &m.Nonce, &m.CreatedAt, &m.EditedAt); err != nil {
			return nil, fmt.Errorf("direct_messages: scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("direct_messages: iterar linhas: %w", err)
	}
	return out, nil
}
