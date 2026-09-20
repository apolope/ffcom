package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemberStore struct {
	pool *pgxpool.Pool
}

// GetOrCreateByOIDCSubject cria (upsert idempotente) o membro vinculado a um
// "sub" da mesma instância central de Authentik usada por server-central,
// sem privilégio de dono (is_owner fica false). Chamado só por handleJoin
// (POST /api/join) depois que o resgate de um convite já autorizou a
// entrada — não é mais chamado implicitamente por toda requisição
// autenticada (ver docs/architecture.md, "Convites obrigatórios para entrar
// em server-channel"). Para o primeiro membro do servidor, ver CreateFounder.
func (s *MemberStore) GetOrCreateByOIDCSubject(ctx context.Context, oidcSubject string) (Member, error) {
	const query = `
		INSERT INTO members (oidc_subject)
		VALUES ($1)
		ON CONFLICT (oidc_subject) DO UPDATE SET oidc_subject = EXCLUDED.oidc_subject
		RETURNING id, oidc_subject, nickname, joined_at, is_owner
	`
	return s.scanOne(ctx, query, oidcSubject)
}

// CreateFounder cria o primeiro membro do servidor com is_owner = true —
// chamado só pelo bootstrap de POST /api/join, quando members ainda está
// vazia (ver docs/architecture.md, "Sistema de permissões/roles por servidor
// e por canal"). Sem ON CONFLICT: se "sub" já existir isso é a corrida de
// bootstrap documentada em handleJoin, e um erro é o comportamento aceitável
// nesse caso extremamente raro.
func (s *MemberStore) CreateFounder(ctx context.Context, oidcSubject string) (Member, error) {
	const query = `
		INSERT INTO members (oidc_subject, is_owner)
		VALUES ($1, true)
		RETURNING id, oidc_subject, nickname, joined_at, is_owner
	`
	return s.scanOne(ctx, query, oidcSubject)
}

// GetByOIDCSubject busca o membro vinculado a um "sub", sem criá-lo — usado
// por auth.RequireMember para checar associação sem efeito colateral (ver
// docs/architecture.md, "Convites obrigatórios para entrar em
// server-channel"). Devolve ErrNotFound se o "sub" ainda não é membro.
func (s *MemberStore) GetByOIDCSubject(ctx context.Context, oidcSubject string) (Member, error) {
	const query = `SELECT id, oidc_subject, nickname, joined_at, is_owner FROM members WHERE oidc_subject = $1`
	return s.scanOne(ctx, query, oidcSubject)
}

// Count devolve o total de membros do servidor — usado para detectar o
// bootstrap (nenhum membro ainda) em POST /api/join.
func (s *MemberStore) Count(ctx context.Context) (int, error) {
	const query = `SELECT count(*) FROM members`
	var n int
	if err := s.pool.QueryRow(ctx, query).Scan(&n); err != nil {
		return 0, fmt.Errorf("members: count: %w", err)
	}
	return n, nil
}

func (s *MemberStore) GetByID(ctx context.Context, id string) (Member, error) {
	const query = `SELECT id, oidc_subject, nickname, joined_at, is_owner FROM members WHERE id = $1`
	return s.scanOne(ctx, query, id)
}

// SetNickname define ou limpa (nickname == nil) o apelido do membro neste servidor.
func (s *MemberStore) SetNickname(ctx context.Context, id string, nickname *string) (Member, error) {
	const query = `
		UPDATE members SET nickname = $2
		WHERE id = $1
		RETURNING id, oidc_subject, nickname, joined_at, is_owner
	`
	return s.scanOne(ctx, query, id, nickname)
}

// List lista todos os membros do servidor.
func (s *MemberStore) List(ctx context.Context) ([]Member, error) {
	const query = `SELECT id, oidc_subject, nickname, joined_at, is_owner FROM members ORDER BY joined_at ASC`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("members: list: %w", err)
	}
	defer rows.Close()

	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.OIDCSubject, &m.Nickname, &m.JoinedAt, &m.IsOwner); err != nil {
			return nil, fmt.Errorf("members: scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("members: iterar linhas: %w", err)
	}
	return out, nil
}

func (s *MemberStore) scanOne(ctx context.Context, query string, args ...any) (Member, error) {
	var m Member
	err := s.pool.QueryRow(ctx, query, args...).Scan(&m.ID, &m.OIDCSubject, &m.Nickname, &m.JoinedAt, &m.IsOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("members: query: %w", err)
	}
	return m, nil
}
