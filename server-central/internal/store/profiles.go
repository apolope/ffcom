package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProfileStore struct {
	pool *pgxpool.Pool
}

// displayNameSQL é o nome exibido de uma conta, para consultas que juntam
// accounts (a) com profiles (p) em LEFT JOIN: o nome escolhido no perfil do
// FFCom; sem ele, o nome do perfil do Authentik (accounts.profile_name).
// Perfis criados só para guardar o avatar, antes de existir
// accounts.profile_name, gravaram o "sub" como nome (o NULLIF descarta).
// Pode dar NULL: conta sem perfil e sem nome do Authentik.
const displayNameSQL = `COALESCE(NULLIF(p.display_name, a.oidc_subject), a.profile_name, p.display_name)`

// profileSelectSQL devolve o perfil de cada conta com o nome resolvido por
// displayNameSQL. Conta sem linha em profiles aparece quando já tem nome do
// Authentik, com avatar nulo.
const profileSelectSQL = `
		SELECT a.id, ` + displayNameSQL + `, p.avatar_url, COALESCE(p.updated_at, a.created_at)
		FROM accounts a
		LEFT JOIN profiles p ON p.account_id = a.id
`

// GetByAccountID devolve o perfil da conta, com o nome resolvido por
// displayNameSQL. ErrNotFound quando não há perfil nem nome do Authentik.
func (s *ProfileStore) GetByAccountID(ctx context.Context, accountID string) (Profile, error) {
	const query = profileSelectSQL + `
		WHERE a.id = $1 AND (p.account_id IS NOT NULL OR a.profile_name IS NOT NULL)
	`
	var p Profile
	err := s.pool.QueryRow(ctx, query, accountID).Scan(&p.AccountID, &p.DisplayName, &p.AvatarURL, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("profiles: get por account_id: %w", err)
	}
	return p, nil
}

// StoredDisplayName devolve o display_name gravado na linha de profiles, sem
// a resolução de displayNameSQL. É o que as rotas de avatar regravam ao
// trocar só o avatar: regravar o nome resolvido congelaria o nome do
// Authentik como se fosse escolhido no FFCom. ErrNotFound sem linha.
func (s *ProfileStore) StoredDisplayName(ctx context.Context, accountID string) (string, error) {
	var name string
	err := s.pool.QueryRow(ctx, `SELECT display_name FROM profiles WHERE account_id = $1`, accountID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("profiles: get display_name gravado: %w", err)
	}
	return name, nil
}

// GetManyByAccountIDs busca o perfil de várias contas de uma vez (ex.: lista
// de amigos), com o nome resolvido por displayNameSQL. Contas sem perfil e
// sem nome do Authentik simplesmente não aparecem no mapa devolvido — não é
// erro.
func (s *ProfileStore) GetManyByAccountIDs(ctx context.Context, accountIDs []string) (map[string]Profile, error) {
	out := make(map[string]Profile, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}

	const query = profileSelectSQL + `
		WHERE a.id = ANY($1) AND (p.account_id IS NOT NULL OR a.profile_name IS NOT NULL)
	`
	rows, err := s.pool.Query(ctx, query, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("profiles: get many por account_id: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.AccountID, &p.DisplayName, &p.AvatarURL, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("profiles: scan: %w", err)
		}
		out[p.AccountID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("profiles: iterar linhas: %w", err)
	}
	return out, nil
}

// Upsert cria ou atualiza o perfil de uma conta.
func (s *ProfileStore) Upsert(ctx context.Context, accountID, displayName string, avatarURL *string) (Profile, error) {
	const query = `
		INSERT INTO profiles (account_id, display_name, avatar_url, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (account_id) DO UPDATE
		SET display_name = EXCLUDED.display_name,
		    avatar_url = EXCLUDED.avatar_url,
		    updated_at = now()
		RETURNING account_id, display_name, avatar_url, updated_at
	`
	var p Profile
	err := s.pool.QueryRow(ctx, query, accountID, displayName, avatarURL).
		Scan(&p.AccountID, &p.DisplayName, &p.AvatarURL, &p.UpdatedAt)
	if err != nil {
		return Profile{}, fmt.Errorf("profiles: upsert: %w", err)
	}
	return p, nil
}
