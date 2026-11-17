// Package storetest opens the test database for tests that need real PostgreSQL.
package storetest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/migrations"
)

// lockID serialises test packages that share TEST_DATABASE_URL, since go test runs them in parallel.
const lockID = 273222011

// Open migrates and empties the test database. It skips the test when TEST_DATABASE_URL is not set.
func Open(t testing.TB) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	lock, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := lock.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		t.Fatalf("lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lock.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockID)
		lock.Release()
		pool.Close()
	})
	if err := migrations.Up(url); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE bookings, lab_sessions, sessions, labs, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}

// InsertBooking adds a bookings row for one session directly, skipping the booking rules, for tests that
// need a lab with bookings already in it.
func InsertBooking(t testing.TB, pool *pgxpool.Pool, userID, sessionID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO bookings (user_id, session_id) VALUES ($1, $2)`, userID, sessionID); err != nil {
		t.Fatalf("insert booking: %v", err)
	}
}

// InsertSession adds a session to a lab directly and returns its id.
func InsertSession(t testing.TB, pool *pgxpool.Pool, labID int64, from, to time.Time) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO lab_sessions (lab_id, starts_at, ends_at) VALUES ($1, $2, $3) RETURNING id`, labID, from, to).Scan(&id); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return id
}

// Warm opens n connections and returns them to the pool idle, as in a server that has been running a
// while. Concurrency tests need it: opening a connection takes longer than a booking, so on a cold
// pool the first goroutine finishes before the others connect and nothing overlaps.
func Warm(t testing.TB, pool *pgxpool.Pool, n int) {
	t.Helper()
	conns := make([]*pgxpool.Conn, 0, n)
	for range n {
		c, err := pool.Acquire(context.Background())
		if err != nil {
			t.Fatalf("warm pool: %v", err)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		c.Release()
	}
}

// Count returns the number of rows in table. table must be a trusted constant.
func Count(t testing.TB, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}
