package store_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
	"github.com/Jul1eNotFound/webdev-273222011/migrations"
)

// hour is a fixed point in the future, so sessions here never depend on the real clock.
var hour = time.Date(2030, 3, 4, 9, 0, 0, 0, time.UTC)

func at(h int) time.Time { return hour.Add(time.Duration(h) * time.Hour) }

func newLab(t *testing.T, st *store.Store, title string, limit int) int64 {
	t.Helper()
	lab, err := st.CreateLab(context.Background(), store.Lab{Title: title, PlaceLimit: limit, OwnerID: staffID(t, st)})
	if err != nil {
		t.Fatal(err)
	}
	return lab.ID
}

func countSessions(t *testing.T, pool *pgxpool.Pool, labID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM lab_sessions WHERE lab_id = $1`, labID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestAC66_DB_MigrationMakesEachLabsPeriodItsOneSession runs migration 000004 over labs and bookings from
// before sessions existed, then back down, and checks nothing is lost either way.
func TestAC66_DB_MigrationMakesEachLabsPeriodItsOneSession(t *testing.T) {
	pool := storetest.Open(t)
	url := os.Getenv("TEST_DATABASE_URL")
	t.Cleanup(func() {
		if err := migrations.Up(url); err != nil {
			t.Errorf("migrate back up: %v", err)
		}
	})
	if err := migrations.To(url, 3); err != nil {
		t.Fatalf("migrate down to 3: %v", err)
	}
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec(`INSERT INTO users (email, name, password, role) VALUES
		('stella@labwiz.test', 'Stella', 'unused', 'staff'),
		('sam@labwiz.test', 'Sam', 'unused', 'student'),
		('ana@labwiz.test', 'Ana', 'unused', 'student')`)
	exec(`INSERT INTO labs (title, starts_at, ends_at, place_limit, owner_id)
		SELECT t.title, $1::timestamptz + t.h * interval '1 hour', $1::timestamptz + (t.h + 2) * interval '1 hour', 4, u.id
		FROM (VALUES ('Optics', 0), ('Circuits', 5)) AS t(title, h), users u WHERE u.email = 'stella@labwiz.test'`, hour)
	exec(`INSERT INTO bookings (user_id, lab_id)
		SELECT u.id, l.id FROM users u, labs l
		WHERE (u.email, l.title) IN (('sam@labwiz.test', 'Optics'), ('ana@labwiz.test', 'Optics'), ('sam@labwiz.test', 'Circuits'))`)

	if err := migrations.Up(url); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	type row struct {
		title      string
		from, to   time.Time
		bookings   int
		perSession int
	}
	var got []row
	rows, err := pool.Query(ctx, `
		SELECT l.title, s.starts_at, s.ends_at,
		       (SELECT count(*) FROM bookings b WHERE b.session_id = s.id),
		       (SELECT count(*) FROM lab_sessions x WHERE x.lab_id = l.id)
		FROM labs l JOIN lab_sessions s ON s.lab_id = l.id ORDER BY l.title`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.title, &r.from, &r.to, &r.bookings, &r.perSession); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if rows.Err() != nil || len(got) != 2 {
		t.Fatalf("sessions = %+v (err %v), want one per lab", got, rows.Err())
	}
	want := map[string]row{
		"Circuits": {from: at(5), to: at(7), bookings: 1},
		"Optics":   {from: at(0), to: at(2), bookings: 2},
	}
	for _, r := range got {
		w := want[r.title]
		if !r.from.Equal(w.from) || !r.to.Equal(w.to) || r.bookings != w.bookings || r.perSession != 1 {
			t.Errorf("%s: %v–%v with %d bookings in %d sessions; want %v–%v with %d in 1",
				r.title, r.from, r.to, r.bookings, r.perSession, w.from, w.to, w.bookings)
		}
	}
	if n := storetest.Count(t, pool, "bookings"); n != 3 {
		t.Fatalf("bookings = %d after migrating up, want 3", n)
	}

	// And back down: each lab gets its session's period back, and each booking its lab.
	if err := migrations.To(url, 3); err != nil {
		t.Fatalf("migrate down to 3: %v", err)
	}
	var optics, opticsBookings int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE starts_at = $1 AND ends_at = $2),
		       (SELECT count(*) FROM bookings b JOIN labs x ON x.id = b.lab_id WHERE x.title = 'Optics')
		FROM labs WHERE title = 'Optics'`, at(0), at(2)).Scan(&optics, &opticsBookings); err != nil {
		t.Fatal(err)
	}
	if optics != 1 || opticsBookings != 2 || storetest.Count(t, pool, "bookings") != 3 {
		t.Errorf("after down: Optics period restored %d, its bookings %d, all bookings %d; want 1, 2, 3",
			optics, opticsBookings, storetest.Count(t, pool, "bookings"))
	}
}

func TestAC66_DB_MigrationDownRefusesALabWithTwoSessions(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	lab := newLab(t, st, "Optics", 4)
	if _, err := st.AddSessions(ctx, lab, []store.Period{{at(0), at(1)}, {at(24), at(25)}}); err != nil {
		t.Fatal(err)
	}
	// Run the real down file, but inside a transaction that is rolled back: through golang-migrate a
	// refused down would mark the shared test database dirty.
	down, err := os.ReadFile("../../migrations/000004_lab_sessions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, string(down))
	if err == nil || !strings.Contains(err.Error(), "cannot migrate down") {
		t.Fatalf("down over a lab with two sessions: err = %v, want the guard's refusal", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if n := countSessions(t, pool, lab); n != 2 {
		t.Fatalf("sessions = %d after the refused down, want 2", n)
	}
}

func TestAC67_DB_AddSessionsAddsRowsForTheLab(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	lab := newLab(t, st, "Optics", 4)
	ids, err := st.AddSessions(context.Background(), lab, []store.Period{{at(0), at(2)}, {at(24), at(26)}})
	if err != nil || len(ids) != 2 {
		t.Fatalf("ids = %v, err = %v; want 2 sessions", ids, err)
	}
	sessions, err := st.SessionsOfLab(context.Background(), lab)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || !sessions[0].StartsAt.Equal(at(0)) || sessions[1].Title != "Optics" || sessions[1].OwnerName != "Owner" {
		t.Fatalf("sessions = %+v", sessions)
	}
	if _, err := st.AddSessions(context.Background(), 999999, []store.Period{{at(0), at(1)}}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing lab: err = %v, want ErrNotFound", err)
	}
}

func TestAC69_DB_OverlappingSessionIsRefused(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	optics, circuits := newLab(t, st, "Optics", 4), newLab(t, st, "Circuits", 2)
	if _, err := st.AddSessions(ctx, optics, []store.Period{{at(0), at(2)}}); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]store.Period{
		"inside": {at(0).Add(30 * time.Minute), at(1)}, "around": {at(-1), at(3)}, "overlapping the end": {at(1), at(3)},
	} {
		if _, err := st.AddSessions(ctx, optics, []store.Period{p}); !errors.Is(err, store.ErrOverlap) {
			t.Errorf("%s: err = %v, want ErrOverlap", name, err)
		}
	}
	// Touching is not overlapping, and another lab may use the same time.
	if _, err := st.AddSessions(ctx, optics, []store.Period{{at(2), at(3)}}); err != nil {
		t.Errorf("right after: %v", err)
	}
	if _, err := st.AddSessions(ctx, circuits, []store.Period{{at(0), at(2)}}); err != nil {
		t.Errorf("another lab at the same time: %v", err)
	}
	// A cancelled session no longer blocks its time (used from Slice 3c).
	if _, err := pool.Exec(ctx, `UPDATE lab_sessions SET cancelled_at = now() WHERE lab_id = $1 AND starts_at = $2`, optics, at(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddSessions(ctx, optics, []store.Period{{at(0), at(2)}}); err != nil {
		t.Errorf("over a cancelled session: %v", err)
	}
}

func TestAC69_DB_TwoOverlappingSessionsAddedAtOnceGiveExactlyOne(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	lab := newLab(t, st, "Optics", 4)
	storetest.Warm(t, pool, 4)
	start := make(chan struct{})
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			// Each one overlaps all the others.
			_, errs[i] = st.AddSessions(context.Background(), lab, []store.Period{{at(0).Add(time.Duration(i) * time.Minute), at(2)}})
		}()
	}
	close(start)
	wg.Wait()
	ok := 0
	for i, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, store.ErrOverlap):
			t.Errorf("%d: unexpected error %v", i, err)
		}
	}
	if ok != 1 || countSessions(t, pool, lab) != 1 {
		t.Fatalf("%d succeeded and %d sessions exist; want 1 and 1", ok, countSessions(t, pool, lab))
	}
}

func TestAC70_DB_AddSessionsIsAllOrNone(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	lab := newLab(t, st, "Optics", 4)
	week := 7 * 24
	if _, err := st.AddSessions(ctx, lab, []store.Period{{at(2 * week), at(2*week + 2)}}); err != nil {
		t.Fatal(err)
	}
	// Four weekly sessions, the third of which clashes: none is added.
	var weekly []store.Period
	for w := range 4 {
		weekly = append(weekly, store.Period{StartsAt: at(w*week + 1), EndsAt: at(w*week + 3)})
	}
	if _, err := st.AddSessions(ctx, lab, weekly); !errors.Is(err, store.ErrOverlap) {
		t.Fatalf("err = %v, want ErrOverlap", err)
	}
	if n := countSessions(t, pool, lab); n != 1 {
		t.Fatalf("sessions = %d, want only the 1 that was there", n)
	}
	// Two in the same call that overlap each other are refused too.
	if _, err := st.AddSessions(ctx, lab, []store.Period{{at(100), at(102)}, {at(101), at(103)}}); !errors.Is(err, store.ErrOverlap) {
		t.Fatalf("overlapping each other: err = %v, want ErrOverlap", err)
	}
}

func TestAC71_DB_EachSessionCountsItsOwnBookings(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	lab := newLab(t, st, "Optics", 4)
	ids, err := st.AddSessions(ctx, lab, []store.Period{{at(0), at(2)}, {at(24), at(26)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(ctx, store.User{Email: "sam@labwiz.test", Name: "Sam", PasswordHash: "unused", Role: "student"}); err != nil {
		t.Fatal(err)
	}
	sam, err := st.UserByEmail(ctx, "sam@labwiz.test")
	if err != nil {
		t.Fatal(err)
	}
	allow := func(store.SessionView, bool) error { return nil }
	first, err := st.BookSession(ctx, sam.ID, ids[0], allow)
	if err != nil || first.Booked != 1 {
		t.Fatalf("first session: %+v, %v", first, err)
	}
	// The same student may book the lab's other session: the unique pair is (user, session).
	second, err := st.BookSession(ctx, sam.ID, ids[1], allow)
	if err != nil || second.Booked != 1 {
		t.Fatalf("second session: %+v, %v", second, err)
	}
	sessions, err := st.SessionsOfLab(ctx, lab)
	if err != nil || len(sessions) != 2 || sessions[0].Booked != 1 || sessions[1].Booked != 1 {
		t.Fatalf("sessions = %+v, err = %v; want one booking each", sessions, err)
	}
	bookers, err := st.SessionBookers(ctx, lab)
	if err != nil || len(bookers) != 2 || bookers[0].SessionID != ids[0] || bookers[1].Name != "Sam" {
		t.Fatalf("bookers = %+v, err = %v", bookers, err)
	}
}

func TestSessionsBetweenAndMyLabsNextSession_AC62(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	lab := newLab(t, st, "Optics", 4)
	empty := newLab(t, st, "Circuits", 2)
	if _, err := st.AddSessions(ctx, lab, []store.Period{{at(-30), at(-28)}, {at(0), at(2)}, {at(24), at(26)}}); err != nil {
		t.Fatal(err)
	}
	day, err := st.SessionsBetween(ctx, at(-1), at(20))
	if err != nil || len(day) != 1 || !day[0].StartsAt.Equal(at(0)) || day[0].LabID != lab {
		t.Fatalf("sessions in the day = %+v, err = %v", day, err)
	}
	owned, err := st.LabsOwnedBy(ctx, staffID(t, st), at(1)) // during the first live session
	if err != nil || len(owned) != 2 {
		t.Fatalf("owned = %+v, err = %v", owned, err)
	}
	if owned[0].ID != lab || owned[0].Sessions != 3 || owned[0].Next == nil || !owned[0].Next.StartsAt.Equal(at(0)) {
		t.Errorf("Optics = %+v, want 3 sessions and the running one next", owned[0])
	}
	if owned[1].ID != empty || owned[1].Next != nil || owned[1].Sessions != 0 {
		t.Errorf("Circuits = %+v, want no sessions and no next", owned[1])
	}
}

func TestEnsureSessionAddsNothingTheSecondTime(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	lab := newLab(t, st, "Optics", 4)
	first, err := st.EnsureSession(ctx, lab, store.Period{StartsAt: at(0), EndsAt: at(2)})
	if err != nil {
		t.Fatal(err)
	}
	again, err := st.EnsureSession(ctx, lab, store.Period{StartsAt: at(0), EndsAt: at(2)})
	if err != nil || again != first || countSessions(t, pool, lab) != 1 {
		t.Fatalf("again = %d (first %d), err = %v, sessions = %d", again, first, err, countSessions(t, pool, lab))
	}
	if _, err := st.EnsureSession(ctx, lab, store.Period{StartsAt: at(1), EndsAt: at(3)}); !errors.Is(err, store.ErrOverlap) {
		t.Fatalf("a different overlapping period: err = %v, want ErrOverlap", err)
	}
	// A cancelled session is found again too: the overlap constraint ignores it, so it must not get a live copy.
	if _, err := st.CancelSession(ctx, first, at(-1), func(store.SessionView) error { return nil }); err != nil {
		t.Fatal(err)
	}
	again, err = st.EnsureSession(ctx, lab, store.Period{StartsAt: at(0), EndsAt: at(2)})
	if err != nil || again != first || countSessions(t, pool, lab) != 1 {
		t.Fatalf("after cancelling: again = %d (first %d), err = %v, sessions = %d", again, first, err, countSessions(t, pool, lab))
	}
}
