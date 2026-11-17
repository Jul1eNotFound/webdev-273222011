package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

var now = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)

func demoAccounts() []seed.Account {
	accounts := seed.DemoAccounts()
	for i := range accounts {
		accounts[i].Password = "demo-pass-" + accounts[i].Email
	}
	return accounts
}

// seedAll runs what go run ./cmd/seed runs.
func seedAll(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	if err := seed.Run(ctx, st, now, time.UTC, seed.Passwords{Student: "s", Staff: "t"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := seed.Demo(ctx, st, now, time.UTC, demoAccounts()); err != nil {
		t.Fatalf("demo: %v", err)
	}
}

func TestDemoSeedsAccountsLabsAndBookingsOnce(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	seedAll(t, st)
	users, labs, bookings := storetest.Count(t, pool, "users"), storetest.Count(t, pool, "labs"), storetest.Count(t, pool, "bookings")
	if want := 2 + len(seed.DemoAccounts()); users != want {
		t.Errorf("users = %d, want %d", users, want)
	}
	if labs < 10 || bookings < 10 {
		t.Errorf("labs = %d, bookings = %d; want plenty of both", labs, bookings)
	}

	// A second run re-dates the labs but adds nothing (AC-5: no duplicate bookings).
	seedAll(t, st)
	sessions := storetest.Count(t, pool, "lab_sessions")
	for table, before := range map[string]int{"users": users, "labs": labs, "bookings": bookings, "lab_sessions": sessions} {
		if after := storetest.Count(t, pool, table); after != before {
			t.Errorf("%s went from %d to %d on the second run", table, before, after)
		}
	}

	var overfull int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM lab_sessions s JOIN labs l ON l.id = s.lab_id
		WHERE (SELECT count(*) FROM bookings b WHERE b.session_id = s.id) > l.place_limit`).Scan(&overfull); err != nil {
		t.Fatal(err)
	}
	if overfull != 0 {
		t.Errorf("%d sessions have more bookings than places", overfull)
	}
}

func TestDemoLabsAreOwnedByStaffFromThreeAccounts_AC60(t *testing.T) {
	pool := storetest.Open(t)
	seedAll(t, store.New(pool))
	var owners, notStaff int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(DISTINCT l.owner_id), count(*) FILTER (WHERE u.role <> 'staff')
		FROM labs l JOIN users u ON u.id = l.owner_id`).Scan(&owners, &notStaff); err != nil {
		t.Fatal(err)
	}
	if owners < 3 || notStaff != 0 {
		t.Errorf("labs have %d owners (want at least 3), %d of them owned by non-staff (want 0)", owners, notStaff)
	}
}

func TestDemoGivesAFullLabAndALabPastMidnightToday(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	seedAll(t, st)
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	today, err := st.SessionsBetween(context.Background(), day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	var full, pastMidnight bool
	for _, l := range today {
		full = full || l.Booked == l.PlaceLimit
		pastMidnight = pastMidnight || l.EndsAt.After(day.AddDate(0, 0, 1))
	}
	if !full || !pastMidnight {
		t.Errorf("today has a full lab: %v, a lab past midnight: %v; want both", full, pastMidnight)
	}
}

func TestDemoAccountsSignInWithTheirPasswordAndRole(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	seedAll(t, st)
	roles := map[string]int{}
	for _, a := range demoAccounts() {
		u, err := st.UserByEmail(context.Background(), a.Email)
		if err != nil {
			t.Fatalf("%s: %v", a.Email, err)
		}
		if u.PasswordHash == a.Password || !(auth.Bcrypt{}).Compare(u.PasswordHash, a.Password) {
			t.Errorf("%s: password is not stored as a bcrypt hash of the given password", a.Email)
		}
		if u.Role != a.Role {
			t.Errorf("%s: role = %q, want %q", a.Email, u.Role, a.Role)
		}
		roles[u.Role]++
	}
	if roles["staff"] < 2 || roles["student"] < 4 {
		t.Errorf("roles = %v, want several of each", roles)
	}
}

func TestDemoRefusesAnAccountWithoutPassword(t *testing.T) {
	st := store.New(storetest.Open(t))
	if err := seed.Demo(context.Background(), st, now, time.UTC, seed.DemoAccounts()); err == nil {
		t.Fatal("Demo accepted accounts without passwords")
	}
}

func TestDemoHasALabThatRunsWeekly_AC67(t *testing.T) {
	pool := storetest.Open(t)
	seedAll(t, store.New(pool))
	var weekly int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM lab_sessions s JOIN labs l ON l.id = s.lab_id WHERE l.title = 'Pendulum timing'`).Scan(&weekly); err != nil {
		t.Fatal(err)
	}
	if weekly != 3 {
		t.Errorf("Pendulum timing has %d sessions, want 3 weekly ones", weekly)
	}
}
