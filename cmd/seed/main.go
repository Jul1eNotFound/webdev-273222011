// Command seed creates the demo accounts and labs, and re-dates the labs to today.
//
// The two main accounts take their passwords from SEED_STUDENT_PASSWORD and SEED_STAFF_PASSWORD.
// The extra demo accounts get random passwords, kept between runs in the credentials file, which
// git ignores. Run it from the repository root.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/config"
	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

func main() {
	credentials := flag.String("credentials", "seed-credentials.txt", "where to write the demo logins (git ignores the default)")
	flag.Parse()
	if err := run(*credentials); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(credentials string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	earlier, err := loadPasswords(credentials)
	if err != nil {
		return fmt.Errorf("read %s: %w", credentials, err)
	}
	demo := withPasswords(seed.DemoAccounts(), earlier)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	st := store.New(pool)
	now := time.Now()
	if err := seed.Run(ctx, st, now, time.Local, seed.Passwords{
		Student: cfg.SeedStudentPassword,
		Staff:   cfg.SeedStaffPassword,
	}); err != nil {
		return err
	}
	if err := seed.Demo(ctx, st, now, time.Local, demo); err != nil {
		return err
	}
	if err := writeCredentials(credentials, demo); err != nil {
		return fmt.Errorf("write %s: %w", credentials, err)
	}
	fmt.Printf("Seeded %d accounts. Logins are in %s.\n", 2+len(demo), credentials)
	return nil
}
