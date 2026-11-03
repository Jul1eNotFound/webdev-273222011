// Command seed creates the demo accounts and labs. Passwords come from SEED_STUDENT_PASSWORD and SEED_STAFF_PASSWORD.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/config"
	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return seed.Run(ctx, store.New(pool), time.Now(), time.Local, seed.Passwords{
		Student: cfg.SeedStudentPassword,
		Staff:   cfg.SeedStaffPassword,
	})
}
