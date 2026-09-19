package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ChannelStore struct {
	pool *pgxpool.Pool
}

func (s *ChannelStore) Create(ctx context.Context, categoryID *string, name string, channelType ChannelType, position int) (Channel, error) {
	const query = `
		INSERT INTO channels (category_id, name, type, position)
		VALUES ($1, $2, $3, $4)
		RETURNING id, category_id, name, type, position, created_at
	`
	var c Channel
	err := s.pool.QueryRow(ctx, query, categoryID, name, channelType, position).
		Scan(&c.ID, &c.CategoryID, &c.Name, &c.Type, &c.Position, &c.CreatedAt)
	if err != nil {
		return Channel{}, fmt.Errorf("channels: create: %w", err)
	}
	return c, nil
}

func (s *ChannelStore) GetByID(ctx context.Context, id string) (Channel, error) {
	const query = `SELECT id, category_id, name, type, position, created_at FROM channels WHERE id = $1`
	var c Channel
	err := s.pool.QueryRow(ctx, query, id).Scan(&c.ID, &c.CategoryID, &c.Name, &c.Type, &c.Position, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	if err != nil {
		return Channel{}, fmt.Errorf("channels: get por id: %w", err)
	}
	return c, nil
}

func (s *ChannelStore) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM channels WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("channels: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List lista todos os canais do servidor, ordenados por posição dentro da categoria.
func (s *ChannelStore) List(ctx context.Context) ([]Channel, error) {
	const query = `
		SELECT id, category_id, name, type, position, created_at
		FROM channels
		ORDER BY category_id NULLS FIRST, position ASC
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("channels: list: %w", err)
	}
	defer rows.Close()

	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.CategoryID, &c.Name, &c.Type, &c.Position, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("channels: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("channels: iterar linhas: %w", err)
	}
	return out, nil
}
