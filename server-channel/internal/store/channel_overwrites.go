package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ChannelOverwriteStore guarda os overwrites de permissão por canal e role
// (ver internal/permissions, Effective).
type ChannelOverwriteStore struct {
	pool *pgxpool.Pool
}

// Set cria ou substitui o overwrite de roleID em channelID.
func (s *ChannelOverwriteStore) Set(ctx context.Context, channelID, roleID string, allow, deny int64) (ChannelRoleOverwrite, error) {
	const query = `
		INSERT INTO channel_role_overwrites (channel_id, role_id, allow, deny)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (channel_id, role_id) DO UPDATE SET allow = EXCLUDED.allow, deny = EXCLUDED.deny
		RETURNING channel_id, role_id, allow, deny
	`
	var o ChannelRoleOverwrite
	err := s.pool.QueryRow(ctx, query, channelID, roleID, allow, deny).
		Scan(&o.ChannelID, &o.RoleID, &o.Allow, &o.Deny)
	if err != nil {
		return ChannelRoleOverwrite{}, fmt.Errorf("channel_role_overwrites: set: %w", err)
	}
	return o, nil
}

// Delete remove o overwrite de roleID em channelID, se existir.
func (s *ChannelOverwriteStore) Delete(ctx context.Context, channelID, roleID string) error {
	const query = `DELETE FROM channel_role_overwrites WHERE channel_id = $1 AND role_id = $2`
	tag, err := s.pool.Exec(ctx, query, channelID, roleID)
	if err != nil {
		return fmt.Errorf("channel_role_overwrites: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListForChannel lista os overwrites de um único canal (checagem de
// permissão pontual: mensagens, WebSocket, voz).
func (s *ChannelOverwriteStore) ListForChannel(ctx context.Context, channelID string) ([]ChannelRoleOverwrite, error) {
	const query = `SELECT channel_id, role_id, allow, deny FROM channel_role_overwrites WHERE channel_id = $1`
	return s.query(ctx, query, channelID)
}

// ListForRoleIDs lista todos os overwrites que envolvam qualquer uma das
// roles informadas, em qualquer canal — usado para filtrar GET
// /api/categories e GET /api/channels (visibilidade de vários canais de uma
// vez) sem N+1 queries.
func (s *ChannelOverwriteStore) ListForRoleIDs(ctx context.Context, roleIDs []string) ([]ChannelRoleOverwrite, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	const query = `SELECT channel_id, role_id, allow, deny FROM channel_role_overwrites WHERE role_id = ANY($1)`
	return s.query(ctx, query, roleIDs)
}

func (s *ChannelOverwriteStore) query(ctx context.Context, query string, args ...any) ([]ChannelRoleOverwrite, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("channel_role_overwrites: query: %w", err)
	}
	defer rows.Close()

	var out []ChannelRoleOverwrite
	for rows.Next() {
		var o ChannelRoleOverwrite
		if err := rows.Scan(&o.ChannelID, &o.RoleID, &o.Allow, &o.Deny); err != nil {
			return nil, fmt.Errorf("channel_role_overwrites: scan: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("channel_role_overwrites: iterar linhas: %w", err)
	}
	return out, nil
}
