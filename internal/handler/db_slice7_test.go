package handler_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// Smart search against real PostgreSQL, with a fake model. The clock says 08:30; seed.Run gives
// Chemistry basics and Circuits workshop today and Robotics intro tomorrow.

func newSearchDBApp(t *testing.T, m *searchModel) (string, *pgxpool.Pool) {
	t.Helper()
	pool := storetest.Open(t)
	st := store.New(pool)
	if err := seed.Run(context.Background(), st, fixedNow, time.UTC, seedPasswords); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return newServer(t, st, st, withSearch(m, 50*time.Millisecond)).URL, pool
}

// snapshot is every row of every table, so any insert, update or delete shows.
func snapshot(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var b strings.Builder
	for _, table := range []string{"users", "sessions", "labs", "lab_sessions", "bookings"} {
		var s string
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) || ':' || coalesce(md5(string_agg(t::text, '|' ORDER BY t::text)), '') FROM `+table+` t`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		b.WriteString(table + "=" + s + "\n")
	}
	return b.String()
}

func TestAC28_DB_SmartSearchWritesNothing(t *testing.T) {
	m := &searchModel{out: `{"date_from":"2026-10-23","date_to":"2026-10-23"}`}
	base, pool := newSearchDBApp(t, m)
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	before := snapshot(t, pool)

	wantRedirect(t, searchFor(t, c, base, "labs tomorrow"), http.StatusSeeOther, tomorrowOnly) // success
	do(t, c, http.MethodGet, base+tomorrowOnly+"&time=morning&min=2&kw=robot", nil)            // chips
	searchFor(t, c, base, strings.Repeat("x", 201))                                            // too long
	m.err = errors.New("timeout")
	searchFor(t, c, base, "labs") // the model fails
	m.err, m.out = nil, `{"lab_id": 1}`
	searchFor(t, c, base, "labs") // bad output
	for range 10 {
		searchFor(t, c, base, "labs") // up to and past the limit
	}
	if after := snapshot(t, pool); after != before {
		t.Fatalf("a search changed the database:\nbefore %s\nafter  %s", before, after)
	}
}

func TestAC31_DB_LabsTomorrowListsTomorrowsSessionOnly(t *testing.T) {
	base, _ := newSearchDBApp(t, &searchModel{out: `{"date_from":"2026-10-23","date_to":"2026-10-23"}`})
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantRedirect(t, searchFor(t, c, base, "labs tomorrow"), http.StatusSeeOther, tomorrowOnly)
	r := do(t, c, http.MethodGet, base+tomorrowOnly, nil)
	wantStatus(t, r, http.StatusOK, "Robotics intro")
	for _, today := range []string{"Chemistry basics", "Circuits workshop", "Open microscopy"} {
		if strings.Contains(r.body, today) {
			t.Errorf("%s (today) is shown", today)
		}
	}
	wantStatus(t, do(t, c, http.MethodGet, base+tomorrowOnly+"&kw=chemistry", nil), http.StatusOK, msgNoMatch)
}
