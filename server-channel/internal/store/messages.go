package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MessageStore struct {
	pool *pgxpool.Pool
}

// CreateThread abre uma thread num canal forum.
func (s *MessageStore) CreateThread(ctx context.Context, channelID, title, authorMemberID string) (Thread, error) {
	const query = `
		INSERT INTO threads (channel_id, title, author_member_id)
		VALUES ($1, $2, $3)
		RETURNING id, channel_id, title, author_member_id, created_at
	`
	var t Thread
	err := s.pool.QueryRow(ctx, query, channelID, title, authorMemberID).
		Scan(&t.ID, &t.ChannelID, &t.Title, &t.AuthorMemberID, &t.CreatedAt)
	if err != nil {
		return Thread{}, fmt.Errorf("messages: create thread: %w", err)
	}
	return t, nil
}

// ListThreads lista as threads de um canal forum, mais recentes primeiro.
func (s *MessageStore) ListThreads(ctx context.Context, channelID string) ([]Thread, error) {
	const query = `
		SELECT id, channel_id, title, author_member_id, created_at
		FROM threads
		WHERE channel_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, channelID)
	if err != nil {
		return nil, fmt.Errorf("messages: list threads: %w", err)
	}
	defer rows.Close()

	var out []Thread
	for rows.Next() {
		var t Thread
		if err := rows.Scan(&t.ID, &t.ChannelID, &t.Title, &t.AuthorMemberID, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("messages: scan thread: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("messages: iterar threads: %w", err)
	}
	return out, nil
}

// Create grava uma mensagem. threadID nulo indica mensagem de canal de
// texto; preenchido indica post dentro de uma thread de forum.
func (s *MessageStore) Create(ctx context.Context, channelID string, threadID *string, authorMemberID, content string) (Message, error) {
	const query = `
		INSERT INTO messages (channel_id, thread_id, author_member_id, content)
		VALUES ($1, $2, $3, $4)
		RETURNING id, channel_id, thread_id, author_member_id, content, created_at, edited_at
	`
	var m Message
	err := s.pool.QueryRow(ctx, query, channelID, threadID, authorMemberID, content).
		Scan(&m.ID, &m.ChannelID, &m.ThreadID, &m.AuthorMemberID, &m.Content, &m.CreatedAt, &m.EditedAt)
	if err != nil {
		return Message{}, fmt.Errorf("messages: create: %w", err)
	}
	return m, nil
}

func (s *MessageStore) Edit(ctx context.Context, id, content string) (Message, error) {
	const query = `
		UPDATE messages SET content = $2, edited_at = now()
		WHERE id = $1
		RETURNING id, channel_id, thread_id, author_member_id, content, created_at, edited_at
	`
	var m Message
	err := s.pool.QueryRow(ctx, query, id, content).
		Scan(&m.ID, &m.ChannelID, &m.ThreadID, &m.AuthorMemberID, &m.Content, &m.CreatedAt, &m.EditedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("messages: edit: %w", err)
	}
	return m, nil
}

func (s *MessageStore) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM messages WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("messages: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListForChannel devolve o histórico paginado de um canal de texto ou de uma
// thread de forum (conforme threadID), mais recentes primeiro. before, se
// não nulo, restringe a mensagens anteriores a esse instante (keyset
// pagination); limit é obrigatório para evitar histórico ilimitado numa
// única resposta.
func (s *MessageStore) ListForChannel(ctx context.Context, channelID string, threadID *string, before *time.Time, limit int) ([]Message, error) {
	query := `
		SELECT id, channel_id, thread_id, author_member_id, content, created_at, edited_at
		FROM messages
		WHERE channel_id = $1
		AND thread_id IS NOT DISTINCT FROM $2
		AND ($3::timestamptz IS NULL OR created_at < $3)
		ORDER BY created_at DESC
		LIMIT $4
	`
	rows, err := s.pool.Query(ctx, query, channelID, threadID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("messages: list para channel: %w", err)
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.ThreadID, &m.AuthorMemberID, &m.Content, &m.CreatedAt, &m.EditedAt); err != nil {
			return nil, fmt.Errorf("messages: scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("messages: iterar linhas: %w", err)
	}
	return out, nil
}
