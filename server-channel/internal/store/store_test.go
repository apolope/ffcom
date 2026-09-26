package store

import (
	"testing"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func TestPlanMigration(t *testing.T) {
	cases := []struct {
		name      string
		dbVersion uint
		dirty     bool
		latest    uint
		wantUp    bool
		wantErr   bool
	}{
		{name: "banco vazio", dbVersion: 0, latest: 5, wantUp: true},
		{name: "banco atrás", dbVersion: 3, latest: 5, wantUp: true},
		{name: "banco em dia", dbVersion: 5, latest: 5, wantUp: true},
		{name: "banco à frente (rollback)", dbVersion: 6, latest: 5, wantUp: false},
		{name: "sujo atrás", dbVersion: 3, dirty: true, latest: 5, wantErr: true},
		{name: "sujo à frente", dbVersion: 6, dirty: true, latest: 5, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up, err := planMigration(tc.dbVersion, tc.dirty, tc.latest)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("esperava erro, veio up=%v", up)
				}
				return
			}
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if up != tc.wantUp {
				t.Fatalf("up = %v, esperava %v", up, tc.wantUp)
			}
		})
	}
}

func TestLatestSourceVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"0001_init.up.sql":    {Data: []byte("select 1;")},
		"0002_b.up.sql":       {Data: []byte("select 1;")},
		"0002_b.down.sql":     {Data: []byte("select 1;")},
		"0010_c.up.sql":       {Data: []byte("select 1;")},
		"0010_c.down.sql":     {Data: []byte("select 1;")},
		"nao_e_migration.txt": {Data: []byte("ignorado")},
	}
	src, err := iofs.New(fsys, ".")
	if err != nil {
		t.Fatalf("iofs.New: %v", err)
	}
	defer src.Close()

	got, err := latestSourceVersion(src)
	if err != nil {
		t.Fatalf("latestSourceVersion: %v", err)
	}
	if got != 10 {
		t.Fatalf("latest = %d, esperava 10", got)
	}
}

func TestLatestSourceVersionSemMigrations(t *testing.T) {
	src, err := iofs.New(fstest.MapFS{"leia.txt": {Data: []byte("x")}}, ".")
	if err != nil {
		t.Fatalf("iofs.New: %v", err)
	}
	defer src.Close()

	got, err := latestSourceVersion(src)
	if err != nil {
		t.Fatalf("latestSourceVersion: %v", err)
	}
	if got != 0 {
		t.Fatalf("latest = %d, esperava 0", got)
	}
}
