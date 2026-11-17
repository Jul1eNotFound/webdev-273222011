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

// bookingRows counts the bookings rows for a lab, or for one user and lab when userID is not 0.
func bookingRows(t *testing.T, pool *pgxpool.Pool, labID, userID int64) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM bookings WHERE lab_id = $1 AND ($2 = 0 OR user_id = $2)`, labID, userID).Scan(&n)
	if err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	return n
}

// labIDByTitle finds any lab by its title, also one that is not available today.
func labIDByTitle(t *testing.T, pool *pgxpool.Pool, title string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `SELECT id FROM labs WHERE title = $1`, title).Scan(&id); err != nil {
		t.Fatalf("lab %q: %v", title, err)
	}
	return id
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

func bookPath(labID int64) string { return "/labs/" + strconv.FormatInt(labID, 10) + "/book" }

func TestAC4_DB_BookingAddsExactlyOneRowAndLowersTheCount(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seededLabID(t, st, "Chemistry basics")
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
	chemistry := seededLabID(t, st, "Chemistry basics")
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantRedirect(t, bookByForm(t, c, base, bookPath(chemistry)), http.StatusSeeOther, "/labs/"+strconv.FormatInt(chemistry, 10)+"?booked=1")
	wantStatus(t, bookByForm(t, c, base, bookPath(chemistry)), http.StatusConflict, msgAlreadyBooked)
	if n := bookingRows(t, pool, chemistry, userIDOf(t, st, seed.StudentEmail)); n != 1 {
		t.Fatalf("bookings for the pair = %d, want 1", n)
	}
}

func TestAC1_DB_FullLabIs409AndCreatesNoRow(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	circuits := seededLabID(t, st, "Circuits workshop") // 2 places
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
	circuits := seededLabID(t, st, "Circuits workshop")
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
	lab, err := st.CreateLab(ctx, store.Lab{
		Title: "Last place", StartsAt: day.Add(14 * time.Hour), EndsAt: day.Add(16 * time.Hour), PlaceLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
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
			_, errs[i] = labs.Book(ctx, service.Caller{ID: id, Role: service.RoleStudent}, lab.ID)
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
	if n := bookingRows(t, pool, lab.ID, 0); n != 1 {
		t.Fatalf("bookings = %d, want 1 (the limit)", n)
	}
}

func TestAC46_DB_StaffBookingIs403AndCreatesNoRow(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seededLabID(t, st, "Chemistry basics")
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	wantStatus(t, bookByForm(t, c, base, bookPath(chemistry)), http.StatusForbidden, msgForbidden)
	if n := storetest.Count(t, pool, "bookings"); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC47_DB_UnknownLabIs404AndCreatesNoRow(t *testing.T) {
	base, _, pool := newDBAppWithPool(t)
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantStatus(t, bookByForm(t, c, base, "/labs/999999/book"), http.StatusNotFound, msgLabNotFound)
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs/999999", nil), http.StatusNotFound, msgLabNotFound)
	if n := storetest.Count(t, pool, "bookings"); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC48_DB_LabNotAvailableTodayIsRefusedAndCreatesNoRow(t *testing.T) {
	base, _, pool := newDBAppWithPool(t)
	robotics := labIDByTitle(t, pool, "Robotics intro") // starts tomorrow
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantStatus(t, bookByForm(t, c, base, bookPath(robotics)), http.StatusConflict, msgNotOpen)
	if n := storetest.Count(t, pool, "bookings"); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC14_DB_StaffSeesAStudentWhoBookedThroughTheApp(t *testing.T) {
	base, st, _ := newDBAppWithPool(t)
	chemistry := seededLabID(t, st, "Chemistry basics")
	bookByForm(t, signedIn(t, base, seed.StudentEmail, seedPasswords.Student), base, bookPath(chemistry))
	staff := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	r := do(t, staff, http.MethodGet, base+"/staff/labs/"+strconv.FormatInt(chemistry, 10), nil)
	wantStatus(t, r, http.StatusOK, "Sam Student")
	wantStatus(t, r, http.StatusOK, "1 of 10 places booked")
}
