package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RoleStore struct {
	pool *pgxpool.Pool
}

// Create cria uma role comum (is_default sempre false — a role "@everyone"
// é seeded pela migration 0002_roles_permissions e não pode ser recriada).
func (s *RoleStore) Create(ctx context.Context, name string, color *string, permissions int64, position int) (Role, error) {
	const query = `
		INSERT INTO roles (name, color, permissions, position)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, color, permissions, position, is_default, created_at
	`
	return s.scanOne(ctx, query, name, color, permissions, position)
}

// Update edita name/color/permissions/position de uma role existente,
// inclusive a role default (só não dá pra mudar is_default por aqui).
func (s *RoleStore) Update(ctx context.Context, id, name string, color *string, permissions int64, position int) (Role, error) {
	const query = `
		UPDATE roles SET name = $2, color = $3, permissions = $4, position = $5
		WHERE id = $1
		RETURNING id, name, color, permissions, position, is_default, created_at
	`
	return s.scanOne(ctx, query, id, name, color, permissions, position)
}

// Delete remove uma role. A role default (is_default) não pode ser
// removida — checado por quem chama (internal/httpapi/roles.go) antes,
// já que a checagem aqui exigiria mais uma ida ao banco sem necessidade.
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
	const query = `SELECT id, name, color, permissions, position, is_default, created_at FROM roles ORDER BY position ASC`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("roles: list: %w", err)
	}
	defer rows.Close()
	return scanRoles(rows)
}

// GetDefault busca a role "@everyone", seeded pela migration
// 0002_roles_permissions — sempre deve existir; ErrNotFound aqui indica
// banco em estado inconsistente (migration não aplicada).
func (s *RoleStore) GetDefault(ctx context.Context) (Role, error) {
	const query = `SELECT id, name, color, permissions, position, is_default, created_at FROM roles WHERE is_default`
	return s.scanOne(ctx, query)
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

// ListForMember lista os roles atribuídos a um membro (não inclui a role
// default — ver internal/httpapi/permissions.go, que sempre soma
// GetDefault a este resultado).
func (s *RoleStore) ListForMember(ctx context.Context, memberID string) ([]Role, error) {
	const query = `
		SELECT r.id, r.name, r.color, r.permissions, r.position, r.is_default, r.created_at
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
	return scanRoles(rows)
}

// AssignmentsForMembers devolve, para cada um dos memberIDs informados, a
// lista de IDs de role atribuídas (sem a default — mesma convenção de
// ListForMember). Usado por GET /api/members para montar a lista de membros
// com suas roles numa única query em vez de N+1.
func (s *RoleStore) AssignmentsForMembers(ctx context.Context, memberIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(memberIDs))
	if len(memberIDs) == 0 {
		return out, nil
	}

	const query = `SELECT member_id, role_id FROM member_roles WHERE member_id = ANY($1)`
	rows, err := s.pool.Query(ctx, query, memberIDs)
	if err != nil {
		return nil, fmt.Errorf("roles: assignments para members: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var memberID, roleID string
		if err := rows.Scan(&memberID, &roleID); err != nil {
			return nil, fmt.Errorf("roles: scan assignment: %w", err)
		}
		out[memberID] = append(out[memberID], roleID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("roles: iterar assignments: %w", err)
	}
	return out, nil
}

func scanRoles(rows pgx.Rows) ([]Role, error) {
	var out []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Color, &r.Permissions, &r.Position, &r.IsDefault, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("roles: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("roles: iterar linhas: %w", err)
	}
	return out, nil
}

func (s *RoleStore) scanOne(ctx context.Context, query string, args ...any) (Role, error) {
	var r Role
	err := s.pool.QueryRow(ctx, query, args...).
		Scan(&r.ID, &r.Name, &r.Color, &r.Permissions, &r.Position, &r.IsDefault, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Role{}, ErrNotFound
	}
	if err != nil {
		return Role{}, fmt.Errorf("roles: query: %w", err)
	}
	return r, nil
}
