package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CategoryStore struct {
	pool *pgxpool.Pool
}

// Create cria uma categoria. position nulo coloca a categoria depois de
// todas as existentes.
func (s *CategoryStore) Create(ctx context.Context, name string, position *int) (Category, error) {
	const query = `
		INSERT INTO categories (name, position)
		VALUES ($1, COALESCE($2, (SELECT COALESCE(MAX(position) + 1, 0) FROM categories)))
		RETURNING id, name, position, created_at
	`
	var c Category
	err := s.pool.QueryRow(ctx, query, name, position).Scan(&c.ID, &c.Name, &c.Position, &c.CreatedAt)
	if err != nil {
		return Category{}, fmt.Errorf("categories: create: %w", err)
	}
	return c, nil
}

func (s *CategoryStore) GetByID(ctx context.Context, id string) (Category, error) {
	const query = `SELECT id, name, position, created_at FROM categories WHERE id = $1`
	var c Category
	err := s.pool.QueryRow(ctx, query, id).Scan(&c.ID, &c.Name, &c.Position, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	if err != nil {
		return Category{}, fmt.Errorf("categories: get por id: %w", err)
	}
	return c, nil
}

func (s *CategoryStore) Update(ctx context.Context, id, name string, position int) (Category, error) {
	const query = `
		UPDATE categories SET name = $2, position = $3
		WHERE id = $1
		RETURNING id, name, position, created_at
	`
	var c Category
	err := s.pool.QueryRow(ctx, query, id, name, position).Scan(&c.ID, &c.Name, &c.Position, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	if err != nil {
		return Category{}, fmt.Errorf("categories: update: %w", err)
	}
	return c, nil
}

// Delete apaga a categoria; os canais dela ficam sem categoria (FK com ON
// DELETE SET NULL, ver migration 0001_init), não são apagados junto.
func (s *CategoryStore) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM categories WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("categories: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List lista todas as categorias do servidor, ordenadas por posição.
func (s *CategoryStore) List(ctx context.Context) ([]Category, error) {
	const query = `SELECT id, name, position, created_at FROM categories ORDER BY position ASC`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("categories: list: %w", err)
	}
	defer rows.Close()

	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Position, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("categories: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("categories: iterar linhas: %w", err)
	}
	return out, nil
}
