// Package migrations embeds the numbered SQL files and applies them with golang-migrate.
package migrations

import (
	"embed"
	"errors"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var files embed.FS

// Up applies every migration that has not run yet.
func Up(databaseURL string) error {
	return run(databaseURL, (*migrate.Migrate).Up)
}

// Down reverts every migration.
func Down(databaseURL string) error {
	return run(databaseURL, (*migrate.Migrate).Down)
}

func run(databaseURL string, step func(*migrate.Migrate) error) error {
	src, err := iofs.New(files, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, pgx5URL(databaseURL))
	if err != nil {
		return err
	}
	defer m.Close()
	if err := step(m); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// pgx5URL swaps the postgres:// scheme for the one the pgx/v5 driver registers.
func pgx5URL(u string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(u, prefix) {
			return "pgx5://" + strings.TrimPrefix(u, prefix)
		}
	}
	return u
}
