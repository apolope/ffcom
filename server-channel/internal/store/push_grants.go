package store

import (
	"context"
	"fmt"
)

// PushCandidate é um membro ativo com grant de push guardado: quem pode
// receber notificação de mensagem nova, antes de conferir a permissão no
// canal (ver internal/httpapi/push.go).
type PushCandidate struct {
	Member        Member
	Token         string
	ServerAddress string
}

// SetPushGrant guarda (ou troca) o grant de push do membro. Ver
// docs/architecture.md, "Decisão: notificações push (fase 6)".
func (s *MemberStore) SetPushGrant(ctx context.Context, memberID, token, serverAddress string) error {
	const query = `
		INSERT INTO member_push_grants (member_id, token, server_address)
		VALUES ($1, $2, $3)
		ON CONFLICT (member_id) DO UPDATE
		SET token = EXCLUDED.token, server_address = EXCLUDED.server_address, updated_at = now()
	`
	if _, err := s.pool.Exec(ctx, query, memberID, token, serverAddress); err != nil {
		return fmt.Errorf("member_push_grants: set: %w", err)
	}
	return nil
}

// DeletePushGrant apaga o grant do membro. Com token, só apaga se for esse
// o guardado: um aparelho saindo não leva junto o grant que outro aparelho
// da mesma pessoa entregou depois.
func (s *MemberStore) DeletePushGrant(ctx context.Context, memberID string, token *string) error {
	var err error
	if token == nil {
		_, err = s.pool.Exec(ctx, `DELETE FROM member_push_grants WHERE member_id = $1`, memberID)
	} else {
		_, err = s.pool.Exec(ctx, `DELETE FROM member_push_grants WHERE member_id = $1 AND token = $2`, memberID, *token)
	}
	if err != nil {
		return fmt.Errorf("member_push_grants: delete: %w", err)
	}
	return nil
}

// PushCandidates lista os membros ativos com grant guardado, menos
// excludeMemberID (o autor da mensagem).
func (s *MemberStore) PushCandidates(ctx context.Context, excludeMemberID string) ([]PushCandidate, error) {
	const query = `
		SELECT m.id, m.oidc_subject, m.nickname, m.profile_name, m.joined_at, m.is_owner, m.removed_at,
		       g.token, g.server_address
		FROM member_push_grants g
		JOIN members m ON m.id = g.member_id
		WHERE m.removed_at IS NULL AND m.id <> $1
	`
	rows, err := s.pool.Query(ctx, query, excludeMemberID)
	if err != nil {
		return nil, fmt.Errorf("member_push_grants: candidates: %w", err)
	}
	defer rows.Close()
	var out []PushCandidate
	for rows.Next() {
		var c PushCandidate
		m := &c.Member
		if err := rows.Scan(&m.ID, &m.OIDCSubject, &m.Nickname, &m.ProfileName, &m.JoinedAt, &m.IsOwner, &m.RemovedAt, &c.Token, &c.ServerAddress); err != nil {
			return nil, fmt.Errorf("member_push_grants: scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("member_push_grants: iterar linhas: %w", err)
	}
	return out, nil
}
