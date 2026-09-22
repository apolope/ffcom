// Package store concentra o acesso a dados do server-channel: conexão com
// Postgres, migrations e repositórios por entidade (members, categories,
// channels, roles, messages/threads, invites).
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"a3sitsolutions.com/ffcom/server-channel/migrations"
)

// ErrNotFound é devolvido pelos repositórios quando a linha buscada não existe.
var ErrNotFound = errors.New("store: registro não encontrado")

// ErrConflict é devolvido pelos repositórios quando a operação esbarra numa
// condição atômica não satisfeita (ex.: convite já esgotado/expirado no
// momento do resgate).
var ErrConflict = errors.New("store: conflito, condição não satisfeita")

// Store agrupa o pool de conexões e os repositórios de cada entidade.
type Store struct {
	pool *pgxpool.Pool

	Members           *MemberStore
	MemberBans        *MemberBanStore
	Categories        *CategoryStore
	Channels          *ChannelStore
	Roles             *RoleStore
	Messages          *MessageStore
	Invites           *InviteStore
	ChannelOverwrites *ChannelOverwriteStore
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
		pool:              pool,
		Members:           &MemberStore{pool: pool},
		MemberBans:        &MemberBanStore{pool: pool},
		Categories:        &CategoryStore{pool: pool},
		Channels:          &ChannelStore{pool: pool},
		Roles:             &RoleStore{pool: pool},
		Messages:          &MessageStore{pool: pool},
		Invites:           &InviteStore{pool: pool},
		ChannelOverwrites: &ChannelOverwriteStore{pool: pool},
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
