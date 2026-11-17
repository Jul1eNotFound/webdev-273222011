package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
)

// Slice 3b against real PostgreSQL.

func sessionsOf(t *testing.T, pool *pgxpool.Pool, labID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM lab_sessions WHERE lab_id = $1`, labID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAC67_DB_OwnerAddsASessionAndAStudentBooksIt(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics").LabID
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	form := url.Values{"starts": {"2026-10-23T14:00"}, "ends": {"2026-10-23T16:00"}, "repeat_until": {"2026-11-06"}}
	wantRedirect(t, do(t, c, http.MethodPost, base+staffLabPath(chemistry)+"/sessions", form), http.StatusSeeOther, staffLabPath(chemistry)+"?scheduled=3")
	if n := sessionsOf(t, pool, chemistry); n != 4 {
		t.Fatalf("sessions = %d, want today's and 3 weekly ones", n)
	}
	// The first new one, tomorrow, can be booked (AC-84) and counts its own places (AC-71).
	var tomorrow int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM lab_sessions WHERE lab_id = $1 ORDER BY starts_at OFFSET 1 LIMIT 1`, chemistry).Scan(&tomorrow); err != nil {
		t.Fatal(err)
	}
	student := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	reply := bookByScript(t, student, base, bookPath(tomorrow))
	if reply.status != http.StatusOK || reply.FreePlaces == nil || *reply.FreePlaces != 9 {
		t.Fatalf("booking = %d %q %v", reply.status, reply.Message, reply.FreePlaces)
	}
	// Today's session of the same lab still has all 10 places, and the student may book it too.
	today := seeded(t, st, "Chemistry basics")
	if today.Booked != 0 {
		t.Errorf("today's session has %d bookings, want 0", today.Booked)
	}
	if r := bookByForm(t, student, base, bookPath(today.SessionID)); r.status != http.StatusSeeOther {
		t.Errorf("booking today's session as well = %d", r.status)
	}
}

func TestAC69_DB_OverlappingSessionIs409AndAddsNothing(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics").LabID // today 09:00–12:00
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	r := do(t, c, http.MethodPost, base+staffLabPath(chemistry)+"/sessions", url.Values{"starts": {"2026-10-22T11:00"}, "ends": {"2026-10-22T13:00"}})
	wantStatus(t, r, http.StatusConflict, msgOverlap)
	if n := sessionsOf(t, pool, chemistry); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
}

func TestAC72_DB_AnotherStaffMemberCannotAddSessions(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics").LabID
	addStaff(t, st, omarEmail, omarPass)
	form := url.Values{"starts": {"2026-10-23T14:00"}, "ends": {"2026-10-23T16:00"}}
	wantStatus(t, do(t, signedIn(t, base, omarEmail, omarPass), http.MethodPost, base+staffLabPath(chemistry)+"/sessions", form), http.StatusForbidden, msgForbidden)
	wantStatus(t, do(t, signedIn(t, base, seed.StudentEmail, seedPasswords.Student), http.MethodPost, base+staffLabPath(chemistry)+"/sessions", form), http.StatusForbidden, msgForbidden)
	if n := sessionsOf(t, pool, chemistry); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
}
