// Package store concentra o acesso a dados do server-central: conexão com
// Postgres, migrations e repositórios por entidade (accounts, profiles,
// friendships, known_servers).
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"a3sitsolutions.com/ffcom/server-central/migrations"
)

// Store agrupa o pool de conexões e os repositórios de cada entidade.
type Store struct {
	pool *pgxpool.Pool

	Accounts       *AccountStore
	Profiles       *ProfileStore
	Friendships    *FriendshipStore
	KnownServers   *KnownServerStore
	FriendInvites  *FriendInviteStore
	DirectMessages *DirectMessageStore
}

// Open conecta ao Postgres em databaseURL, aplica as migrations pendentes e
// devolve um Store pronto para uso.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: conectar ao postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping no postgres: %w", err)
	}

	if err := migrateUp(databaseURL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: aplicar migrations: %w", err)
	}

	return &Store{
		pool:           pool,
		Accounts:       &AccountStore{pool: pool},
		Profiles:       &ProfileStore{pool: pool},
		Friendships:    &FriendshipStore{pool: pool},
		KnownServers:   &KnownServerStore{pool: pool},
		FriendInvites:  &FriendInviteStore{pool: pool},
		DirectMessages: &DirectMessageStore{pool: pool},
	}, nil
}

// Close libera o pool de conexões.
func (s *Store) Close() {
	s.pool.Close()
}

func migrateUp(databaseURL string) error {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("carregar migrations embutidas: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("preparar migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
