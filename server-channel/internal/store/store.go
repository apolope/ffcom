// Package store concentra o acesso a dados do server-channel: conexão com
// Postgres, migrations e repositórios por entidade (members, categories,
// channels, roles, messages/threads, invites).
package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
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
	Attachments       *AttachmentStore
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
		Attachments:       &AttachmentStore{pool: pool},
		Invites:           &InviteStore{pool: pool},
		ChannelOverwrites: &ChannelOverwriteStore{pool: pool},
	}, nil
}

// Close libera o pool de conexões.
func (s *Store) Close() {
	s.pool.Close()
}

// migrateUp aplica as migrations embutidas, tolerando banco à frente: se
// uma versão mais nova do binário já aplicou uma migration que esta não
// conhece (rollback feito pelo ffcom-runtime), loga e segue sem rodar Up.
// Isso só é seguro porque as migrations seguem expand/contract (ver
// migrations/embed.go e docs/architecture.md, "Decisão: container evergreen
// em server-channel").
func migrateUp(databaseURL string) error {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("carregar migrations embutidas: %w", err)
	}

	latest, err := latestSourceVersion(source)
	if err != nil {
		source.Close()
		return fmt.Errorf("ler migrations embutidas: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("preparar migrate: %w", err)
	}
	defer m.Close()

	// ErrNilVersion: banco sem nenhuma migration aplicada, versão 0 (as
	// migrations começam em 1).
	dbVersion, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("ler versão do schema: %w", err)
	}

	runUp, err := planMigration(dbVersion, dirty, latest)
	if err != nil {
		return err
	}
	if !runUp {
		log.Printf("server-channel: AVISO: schema do banco na versão %d, à frente da maior migration deste binário (%d); seguindo sem migrar (provável rollback de versão)", dbVersion, latest)
		return nil
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// planMigration decide se o boot roda m.Up(): dirty é sempre erro (migration
// que falhou no meio pede intervenção manual); banco numa versão acima da
// maior migration embutida (latest) segue sem migrar; o resto roda Up
// normalmente (ErrNoChange se já estiver em dia).
func planMigration(dbVersion uint, dirty bool, latest uint) (bool, error) {
	if dirty {
		return false, fmt.Errorf("schema do banco sujo na versão %d (migration interrompida), precisa de correção manual", dbVersion)
	}
	if dbVersion > latest {
		return false, nil
	}
	return true, nil
}

// latestSourceVersion percorre as migrations do source e devolve a maior
// versão (0 se não houver nenhuma).
func latestSourceVersion(src source.Driver) (uint, error) {
	version, err := src.First()
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	for {
		next, err := src.Next(version)
		if errors.Is(err, fs.ErrNotExist) {
			return version, nil
		}
		if err != nil {
			return 0, err
		}
		version = next
	}
}
