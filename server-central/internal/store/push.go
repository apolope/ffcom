package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PushStore guarda o que as notificações push precisam: aparelhos (tokens
// FCM), grants por servidor e silêncios. Ver docs/architecture.md,
// "Decisão: notificações push (fase 6)". Nada do conteúdo notificado passa
// por aqui: o texto de uma mensagem só existe em memória, no caminho até o
// FCM.
type PushStore struct {
	pool *pgxpool.Pool
}

const (
	// maxPushDevicesPerAccount: aparelhos guardados por conta. Um token novo
	// além disso tira o usado há mais tempo (aparelho trocado que nunca
	// avisou o DELETE).
	maxPushDevicesPerAccount = 20
	// maxPushGrantsPerServer: grants guardados por conta e servidor. Cada
	// aparelho pede o seu, e o server-channel guarda só o último entregue;
	// manter alguns evita que dois aparelhos pedindo ao mesmo tempo
	// invalidem o grant um do outro.
	maxPushGrantsPerServer = 10
)

// UpsertDevice registra (ou renova) o token FCM de um aparelho da conta.
// O token já registrado por outra conta passa para esta.
func (s *PushStore) UpsertDevice(ctx context.Context, accountID, token, platform string) error {
	const query = `
		INSERT INTO push_devices (token, account_id, platform)
		VALUES ($1, $2, $3)
		ON CONFLICT (token) DO UPDATE
		SET account_id = EXCLUDED.account_id, platform = EXCLUDED.platform, last_seen_at = now()
	`
	if _, err := s.pool.Exec(ctx, query, token, accountID, platform); err != nil {
		return fmt.Errorf("push_devices: upsert: %w", err)
	}
	const trim = `
		DELETE FROM push_devices WHERE account_id = $1 AND token NOT IN (
			SELECT token FROM push_devices WHERE account_id = $1
			ORDER BY last_seen_at DESC LIMIT $2
		)
	`
	if _, err := s.pool.Exec(ctx, trim, accountID, maxPushDevicesPerAccount); err != nil {
		return fmt.Errorf("push_devices: limitar por conta: %w", err)
	}
	return nil
}

// DeleteDevice tira o token da conta (logout no aparelho). Token que não é
// da conta não é tocado, sem erro.
func (s *PushStore) DeleteDevice(ctx context.Context, accountID, token string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM push_devices WHERE account_id = $1 AND token = $2`, accountID, token); err != nil {
		return fmt.Errorf("push_devices: delete: %w", err)
	}
	return nil
}

// DeleteDeviceToken apaga um token que o FCM disse não existir mais
// (UNREGISTERED), de qualquer conta.
func (s *PushStore) DeleteDeviceToken(ctx context.Context, token string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM push_devices WHERE token = $1`, token); err != nil {
		return fmt.Errorf("push_devices: delete token: %w", err)
	}
	return nil
}

// DeviceTokens lista os tokens FCM da conta.
func (s *PushStore) DeviceTokens(ctx context.Context, accountID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT token FROM push_devices WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("push_devices: list: %w", err)
	}
	tokens, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("push_devices: scan: %w", err)
	}
	return tokens, nil
}

// CreateGrant guarda o hash de um grant novo da conta para o servidor de
// endereço address. Devolve ErrNotFound se o servidor não está na lista da
// conta (known_servers).
func (s *PushStore) CreateGrant(ctx context.Context, accountID, address string, tokenHash []byte) error {
	const query = `
		INSERT INTO push_grants (token_hash, account_id, known_server_id)
		SELECT $3, $1, id FROM known_servers WHERE account_id = $1 AND address = $2
		RETURNING known_server_id
	`
	var knownServerID string
	err := s.pool.QueryRow(ctx, query, accountID, address, tokenHash).Scan(&knownServerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("push_grants: create: %w", err)
	}
	const trim = `
		DELETE FROM push_grants WHERE known_server_id = $1 AND token_hash NOT IN (
			SELECT token_hash FROM push_grants WHERE known_server_id = $1
			ORDER BY created_at DESC LIMIT $2
		)
	`
	if _, err := s.pool.Exec(ctx, trim, knownServerID, maxPushGrantsPerServer); err != nil {
		return fmt.Errorf("push_grants: limitar por servidor: %w", err)
	}
	return nil
}

// PushTarget é um grant válido resolvido para uma notificação de canal: de
// quem é, o nome que a pessoa deu ao servidor e se ela silenciou o servidor
// ou o canal.
type PushTarget struct {
	TokenHash  []byte
	AccountID  string
	ServerName string
	Muted      bool
}

// ResolveGrants devolve os grants entre tokenHashes que pertencem ao
// servidor de endereço address (os de outro servidor e os desconhecidos
// ficam de fora, sem erro), já com o silêncio do servidor ou do canal
// channelID.
func (s *PushStore) ResolveGrants(ctx context.Context, address, channelID string, tokenHashes [][]byte) ([]PushTarget, error) {
	if len(tokenHashes) == 0 {
		return nil, nil
	}
	const query = `
		SELECT g.token_hash, g.account_id, k.name,
		       EXISTS (SELECT 1 FROM push_mutes m
		               WHERE m.known_server_id = k.id AND (m.channel_id = '' OR m.channel_id = $3))
		FROM push_grants g
		JOIN known_servers k ON k.id = g.known_server_id
		WHERE g.token_hash = ANY($1) AND k.address = $2
	`
	rows, err := s.pool.Query(ctx, query, tokenHashes, address, channelID)
	if err != nil {
		return nil, fmt.Errorf("push_grants: resolve: %w", err)
	}
	defer rows.Close()
	var out []PushTarget
	for rows.Next() {
		var t PushTarget
		if err := rows.Scan(&t.TokenHash, &t.AccountID, &t.ServerName, &t.Muted); err != nil {
			return nil, fmt.Errorf("push_grants: scan: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("push_grants: iterar linhas: %w", err)
	}
	return out, nil
}

// PushMute é um silêncio da conta: o servidor inteiro (ChannelID vazio) ou
// um canal dele.
type PushMute struct {
	ServerAddress string
	ChannelID     string
}

// ListMutes lista os silêncios da conta.
func (s *PushStore) ListMutes(ctx context.Context, accountID string) ([]PushMute, error) {
	const query = `
		SELECT k.address, m.channel_id
		FROM push_mutes m JOIN known_servers k ON k.id = m.known_server_id
		WHERE k.account_id = $1
		ORDER BY k.address, m.channel_id
	`
	rows, err := s.pool.Query(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("push_mutes: list: %w", err)
	}
	defer rows.Close()
	var out []PushMute
	for rows.Next() {
		var m PushMute
		if err := rows.Scan(&m.ServerAddress, &m.ChannelID); err != nil {
			return nil, fmt.Errorf("push_mutes: scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("push_mutes: iterar linhas: %w", err)
	}
	return out, nil
}

// SetMute silencia (muted) ou volta a notificar o servidor de endereço
// address (channelID vazio) ou um canal dele. Devolve ErrNotFound se o
// servidor não está na lista da conta.
func (s *PushStore) SetMute(ctx context.Context, accountID, address, channelID string, muted bool) error {
	var knownServerID string
	err := s.pool.QueryRow(ctx, `SELECT id FROM known_servers WHERE account_id = $1 AND address = $2`, accountID, address).Scan(&knownServerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("push_mutes: buscar servidor: %w", err)
	}
	if muted {
		_, err = s.pool.Exec(ctx, `INSERT INTO push_mutes (known_server_id, channel_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, knownServerID, channelID)
	} else {
		_, err = s.pool.Exec(ctx, `DELETE FROM push_mutes WHERE known_server_id = $1 AND channel_id = $2`, knownServerID, channelID)
	}
	if err != nil {
		return fmt.Errorf("push_mutes: set: %w", err)
	}
	return nil
}
