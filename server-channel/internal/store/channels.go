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

// Create cria um canal. position nulo coloca o canal depois de todos os
// canais já existentes na mesma categoria (ou entre os sem categoria).
func (s *ChannelStore) Create(ctx context.Context, categoryID *string, name string, channelType ChannelType, position *int) (Channel, error) {
	const query = `
		INSERT INTO channels (category_id, name, type, position)
		VALUES ($1, $2, $3, COALESCE($4, (
			SELECT COALESCE(MAX(position) + 1, 0) FROM channels
			WHERE category_id IS NOT DISTINCT FROM $1::uuid
		)))
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

// Update troca nome, categoria (nula = sem categoria) e posição do canal. O
// tipo não muda depois de criado: o histórico de um canal de texto não faz
// sentido num canal de voz, e vice-versa.
func (s *ChannelStore) Update(ctx context.Context, id, name string, categoryID *string, position int) (Channel, error) {
	const query = `
		UPDATE channels SET name = $2, category_id = $3, position = $4
		WHERE id = $1
		RETURNING id, category_id, name, type, position, created_at
	`
	var c Channel
	err := s.pool.QueryRow(ctx, query, id, name, categoryID, position).
		Scan(&c.ID, &c.CategoryID, &c.Name, &c.Type, &c.Position, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	if err != nil {
		return Channel{}, fmt.Errorf("channels: update: %w", err)
	}
	return c, nil
}

// Delete apaga o canal. Mensagens, threads, anexos (só as linhas) e
// overwrites vão junto por ON DELETE CASCADE; os arquivos de anexo em disco
// não, quem chama precisa buscá-los antes (AttachmentStore.
// StorageKeysForChannel) e apagá-los depois.
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
