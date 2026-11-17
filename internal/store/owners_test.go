package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
	"github.com/Jul1eNotFound/webdev-273222011/migrations"
)

// TestAC60_DB_MigrationGivesExistingLabsTheFirstStaffAccount runs migration 000003 over labs that were
// added before labs had owners, as on a database that already ran Slices 0-3.
func TestAC60_DB_MigrationGivesExistingLabsTheFirstStaffAccount(t *testing.T) {
	pool := storetest.Open(t) // holds the lock that keeps other tests off the database
	url := os.Getenv("TEST_DATABASE_URL")
	t.Cleanup(func() {
		// Leave the schema current for the next test, even when this one fails.
		if err := migrations.Up(url); err != nil {
			t.Errorf("migrate back up: %v", err)
		}
	})
	if err := migrations.To(url, 2); err != nil {
		t.Fatalf("migrate down to 2: %v", err)
	}
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	id := func(email string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// A student first, so the lowest user id is not staff.
	for _, u := range []struct{ email, role string }{
		{"sam@labwiz.test", "student"}, {"stella@labwiz.test", "staff"}, {"dana@labwiz.test", "staff"},
	} {
		exec(`INSERT INTO users (email, name, password, role) VALUES ($1, $1, 'unused', $2)`, u.email, u.role)
	}
	now := time.Now()
	exec(`INSERT INTO labs (title, starts_at, ends_at, place_limit) VALUES ('Optics', $1, $2, 4), ('Circuits', $1, $2, 2)`,
		now, now.Add(time.Hour))
	exec(`INSERT INTO bookings (user_id, lab_id) SELECT $1, id FROM labs WHERE title = 'Optics'`, id("sam@labwiz.test"))

	if err := migrations.Up(url); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	stella := id("stella@labwiz.test")
	st := store.New(pool)
	labs, err := st.LabsOwnedBy(ctx, stella, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) != 2 {
		t.Fatalf("Stella owns %d labs, want both", len(labs))
	}
	for _, l := range labs {
		if l.OwnerName != "stella@labwiz.test" {
			t.Errorf("%s: owner = %q", l.Title, l.OwnerName)
		}
		// Migration 000004 then made each lab's period its one session (AC-66).
		sessions, err := st.SessionsOfLab(ctx, l.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions) != 1 {
			t.Fatalf("%s: %d sessions, want 1", l.Title, len(sessions))
		}
		if want := map[string]int{"Optics": 1, "Circuits": 0}[l.Title]; sessions[0].Booked != want {
			t.Errorf("%s: bookings = %d, want %d", l.Title, sessions[0].Booked, want)
		}
	}
	if n := storetest.Count(t, pool, "bookings"); n != 1 {
		t.Fatalf("bookings = %d, want the 1 from before the migration", n)
	}
}

// TestAC60_DB_LabsMustHaveAnExistingOwner shows the database itself refuses a lab without an owner.
func TestAC60_DB_LabsMustHaveAnExistingOwner(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	for name, owner := range map[string]int64{"no owner": 0, "unknown owner": 999999} {
		if _, err := st.CreateLab(context.Background(), store.Lab{Title: name, PlaceLimit: 1, OwnerID: owner}); err == nil {
			t.Errorf("%s: lab was created", name)
		}
	}
	if n := storetest.Count(t, pool, "labs"); n != 0 {
		t.Fatalf("labs = %d, want 0", n)
	}
}
