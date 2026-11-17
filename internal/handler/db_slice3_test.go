package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// Slice 3 against real PostgreSQL. Like db_test.go, these skip when TEST_DATABASE_URL is not set.

// bookingRows counts the bookings rows for a session, or for one user and session when userID is not 0.
func bookingRows(t *testing.T, pool *pgxpool.Pool, sessionID, userID int64) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM bookings WHERE session_id = $1 AND ($2 = 0 OR user_id = $2)`, sessionID, userID).Scan(&n)
	if err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	return n
}

// firstSessionOf finds the first session of a lab by the lab's title, also one that is not today.
func firstSessionOf(t *testing.T, pool *pgxpool.Pool, title string) (labID, sessionID int64) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT l.id, s.id FROM labs l JOIN lab_sessions s ON s.lab_id = l.id
		WHERE l.title = $1 ORDER BY s.starts_at LIMIT 1`, title).Scan(&labID, &sessionID); err != nil {
		t.Fatalf("session of %q: %v", title, err)
	}
	return labID, sessionID
}

// addStudents creates n students who never sign in and returns their ids.
func addStudents(t *testing.T, st *store.Store, n int) []int64 {
	t.Helper()
	ctx := context.Background()
	ids := make([]int64, n)
	for i := range n {
		email := fmt.Sprintf("student%d@labwiz.test", i)
		if err := st.UpsertUser(ctx, store.User{Email: email, Name: fmt.Sprintf("Student %d", i), PasswordHash: "unused", Role: "student"}); err != nil {
			t.Fatal(err)
		}
		u, err := st.UserByEmail(ctx, email)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = u.ID
	}
	return ids
}

func userIDOf(t *testing.T, st *store.Store, email string) int64 {
	t.Helper()
	u, err := st.UserByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func bookPath(sessionID int64) string {
	return "/sessions/" + strconv.FormatInt(sessionID, 10) + "/book"
}

func TestAC4_DB_BookingAddsExactlyOneRowAndLowersTheCount(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics").SessionID
	student := userIDOf(t, st, seed.StudentEmail)
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)

	reply := bookByScript(t, c, base, bookPath(chemistry))
	if reply.status != http.StatusOK || reply.FreePlaces == nil || *reply.FreePlaces != 9 {
		t.Fatalf("got %d %q free_places=%v, want 200 with 9", reply.status, reply.Message, reply.FreePlaces)
	}
	if n := bookingRows(t, pool, chemistry, student); n != 1 {
		t.Fatalf("bookings for the student and lab = %d, want 1", n)
	}
	if n := bookingRows(t, pool, chemistry, 0); n != 1 {
		t.Fatalf("bookings for the lab = %d, want 1", n)
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "9 of 10 places free")
}

func TestAC5_DB_SecondBookingIs409AndLeavesOneRow(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics")
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantRedirect(t, bookByForm(t, c, base, bookPath(chemistry.SessionID)), http.StatusSeeOther, "/labs/"+strconv.FormatInt(chemistry.LabID, 10)+"?booked=1")
	wantStatus(t, bookByForm(t, c, base, bookPath(chemistry.SessionID)), http.StatusConflict, msgAlreadyBooked)
	if n := bookingRows(t, pool, chemistry.SessionID, userIDOf(t, st, seed.StudentEmail)); n != 1 {
		t.Fatalf("bookings for the pair = %d, want 1", n)
	}
}

func TestAC1_DB_FullLabIs409AndCreatesNoRow(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	circuits := seeded(t, st, "Circuits workshop").SessionID // 2 places
	for _, id := range addStudents(t, st, 2) {
		storetest.InsertBooking(t, pool, id, circuits)
	}
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	r := bookByForm(t, c, base, bookPath(circuits))
	wantStatus(t, r, http.StatusConflict, msgFull)
	wantStatus(t, r, http.StatusConflict, `href="/"`)
	if n := bookingRows(t, pool, circuits, 0); n != 2 {
		t.Fatalf("bookings = %d, want 2", n)
	}
}

func TestAC8_DB_FullLabShowsFullAndRefusesAPost(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	circuits := seeded(t, st, "Circuits workshop").SessionID
	for _, id := range addStudents(t, st, 2) {
		storetest.InsertBooking(t, pool, id, circuits)
	}
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "disabled>Full</button>")
	if reply := bookByScript(t, c, base, bookPath(circuits)); reply.status != http.StatusConflict {
		t.Fatalf("got %d %q, want 409", reply.status, reply.Message)
	}
	if n := bookingRows(t, pool, circuits, 0); n != 2 {
		t.Fatalf("bookings = %d, want 2", n)
	}
}

func TestAC6_DB_ConcurrentBookingsOfTheLastPlaceLetExactlyOneThrough(t *testing.T) {
	_, st, pool := newDBAppWithPool(t)
	ctx := context.Background()
	day := time.Date(fixedNow.Year(), fixedNow.Month(), fixedNow.Day(), 0, 0, 0, 0, time.UTC)
	lab, err := st.CreateLab(ctx, store.Lab{Title: "Last place", PlaceLimit: 1, OwnerID: userIDOf(t, st, seed.StaffEmail)})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.AddSessions(ctx, lab.ID, []store.Period{{StartsAt: day.Add(14 * time.Hour), EndsAt: day.Add(16 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	session := ids[0]
	students := addStudents(t, st, 10)
	labs := service.NewLabs(st, clock, time.UTC)
	storetest.Warm(t, pool, len(students))

	// All goroutines wait at start, so their transactions really overlap.
	start := make(chan struct{})
	errs := make([]error, len(students))
	var wg sync.WaitGroup
	for i, id := range students {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = labs.Book(ctx, service.Caller{ID: id, Role: service.RoleStudent}, session)
		}()
	}
	close(start)
	wg.Wait()

	succeeded, full := 0, 0
	for i, err := range errs {
		switch err {
		case nil:
			succeeded++
		case service.ErrLabFull:
			full++
		default:
			t.Errorf("student %d: unexpected error %v", i, err)
		}
	}
	if succeeded != 1 || full != len(students)-1 {
		t.Fatalf("succeeded = %d, full = %d; want 1 and %d", succeeded, full, len(students)-1)
	}
	if n := bookingRows(t, pool, session, 0); n != 1 {
		t.Fatalf("bookings = %d, want 1 (the limit)", n)
	}
}

func TestAC46_DB_StaffBookingIs403AndCreatesNoRow(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics").SessionID
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	wantStatus(t, bookByForm(t, c, base, bookPath(chemistry)), http.StatusForbidden, msgForbidden)
	if n := storetest.Count(t, pool, "bookings"); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC47_DB_UnknownLabIs404AndCreatesNoRow(t *testing.T) {
	base, _, pool := newDBAppWithPool(t)
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantStatus(t, bookByForm(t, c, base, "/sessions/999999/book"), http.StatusNotFound, msgSessionNotFound)
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs/999999", nil), http.StatusNotFound, msgLabNotFound)
	if n := storetest.Count(t, pool, "bookings"); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC84_DB_SessionTomorrowCanBeBooked(t *testing.T) {
	base, _, pool := newDBAppWithPool(t)
	_, robotics := firstSessionOf(t, pool, "Robotics intro") // tomorrow
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	if r := bookByForm(t, c, base, bookPath(robotics)); r.status != http.StatusSeeOther {
		t.Fatalf("booking tomorrow's session = %d\n%s", r.status, r.body)
	}
	if n := bookingRows(t, pool, robotics, 0); n != 1 {
		t.Fatalf("bookings = %d, want 1", n)
	}
}

func TestAC83_DB_StartedSessionIsRefusedAndCreatesNoRow(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	microscopy := seeded(t, st, "Open microscopy").SessionID // 08:00–20:00; the clock says 08:30
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantStatus(t, bookByForm(t, c, base, bookPath(microscopy)), http.StatusConflict, msgClosed)
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "disabled>Closed</button>")
	if n := storetest.Count(t, pool, "bookings"); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC14_DB_StaffSeesAStudentWhoBookedThroughTheApp(t *testing.T) {
	base, st, _ := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics")
	bookByForm(t, signedIn(t, base, seed.StudentEmail, seedPasswords.Student), base, bookPath(chemistry.SessionID))
	staff := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	r := do(t, staff, http.MethodGet, base+staffLabPath(chemistry.LabID), nil)
	wantStatus(t, r, http.StatusOK, "Sam Student")
	wantStatus(t, r, http.StatusOK, "1 of 10 places booked")
}
