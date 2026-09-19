package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleStore struct {
	pool *pgxpool.Pool
}

func (s *RoleStore) Create(ctx context.Context, name string, color *string, permissions int64, position int) (Role, error) {
	const query = `
		INSERT INTO roles (name, color, permissions, position)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, color, permissions, position, created_at
	`
	var r Role
	err := s.pool.QueryRow(ctx, query, name, color, permissions, position).
		Scan(&r.ID, &r.Name, &r.Color, &r.Permissions, &r.Position, &r.CreatedAt)
	if err != nil {
		return Role{}, fmt.Errorf("roles: create: %w", err)
	}
	return r, nil
}

func (s *RoleStore) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM roles WHERE id = $1`
	tag, err := s.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("roles: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List lista todos os roles do servidor, ordenados por posição.
func (s *RoleStore) List(ctx context.Context) ([]Role, error) {
	const query = `SELECT id, name, color, permissions, position, created_at FROM roles ORDER BY position ASC`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("roles: list: %w", err)
	}
	defer rows.Close()

	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Color, &r.Permissions, &r.Position, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("roles: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("roles: iterar linhas: %w", err)
	}
	return out, nil
}

// AssignToMember atribui um role a um membro. Idempotente: atribuir de novo não gera erro.
func (s *RoleStore) AssignToMember(ctx context.Context, memberID, roleID string) error {
	const query = `
		INSERT INTO member_roles (member_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	if _, err := s.pool.Exec(ctx, query, memberID, roleID); err != nil {
		return fmt.Errorf("roles: assign to member: %w", err)
	}
	return nil
}

func (s *RoleStore) RemoveFromMember(ctx context.Context, memberID, roleID string) error {
	const query = `DELETE FROM member_roles WHERE member_id = $1 AND role_id = $2`
	tag, err := s.pool.Exec(ctx, query, memberID, roleID)
	if err != nil {
		return fmt.Errorf("roles: remove from member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListForMember lista os roles atribuídos a um membro.
func (s *RoleStore) ListForMember(ctx context.Context, memberID string) ([]Role, error) {
	const query = `
		SELECT r.id, r.name, r.color, r.permissions, r.position, r.created_at
		FROM roles r
		JOIN member_roles mr ON mr.role_id = r.id
		WHERE mr.member_id = $1
		ORDER BY r.position ASC
	`
	rows, err := s.pool.Query(ctx, query, memberID)
	if err != nil {
		return nil, fmt.Errorf("roles: list para member: %w", err)
	}
	defer rows.Close()

	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Color, &r.Permissions, &r.Position, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("roles: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("roles: iterar linhas: %w", err)
	}
	return out, nil
}
