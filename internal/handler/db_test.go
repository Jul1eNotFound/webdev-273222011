package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// These tests run against real PostgreSQL and skip when TEST_DATABASE_URL is not set.

var seedPasswords = seed.Passwords{Student: "student-test-pass", Staff: "staff-test-pass"}

func newDBApp(t *testing.T) (string, *store.Store, func(table string) int) {
	t.Helper()
	base, st, pool := newDBAppWithPool(t)
	return base, st, func(table string) int { return storetest.Count(t, pool, table) }
}

func newDBAppWithPool(t *testing.T) (string, *store.Store, *pgxpool.Pool) {
	t.Helper()
	pool := storetest.Open(t)
	st := store.New(pool)
	if err := seed.Run(context.Background(), st, fixedNow, time.UTC, seedPasswords); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return newServer(t, st, st).URL, st, pool
}

func TestAC3_DB_SeededStudentSignsInAndSeesTodaysLabs(t *testing.T) {
	base, _, _ := newDBApp(t)
	c := newClient(t)
	wantRedirect(t, signIn(t, c, base, seed.StudentEmail, seedPasswords.Student), http.StatusSeeOther, "/labs")
	r := do(t, c, http.MethodGet, base+"/labs", nil)
	for _, title := range []string{"Chemistry basics", "Circuits workshop", "Open microscopy"} {
		if !strings.Contains(r.body, title) {
			t.Errorf("/labs does not show %q", title)
		}
	}
	if strings.Contains(r.body, "Robotics intro") {
		t.Error("/labs shows the lab that starts tomorrow")
	}
}

func TestAC18_DB_SeededPasswordsAreStoredAsBcryptHashes(t *testing.T) {
	_, st, _ := newDBApp(t)
	for email, plain := range map[string]string{seed.StudentEmail: seedPasswords.Student, seed.StaffEmail: seedPasswords.Staff} {
		u, err := st.UserByEmail(context.Background(), email)
		if err != nil {
			t.Fatalf("%s: %v", email, err)
		}
		if u.PasswordHash == plain {
			t.Errorf("%s: password column holds the plain password", email)
		}
		if _, err := bcrypt.Cost([]byte(u.PasswordHash)); err != nil {
			t.Errorf("%s: password column is not a bcrypt hash: %v", email, err)
		}
	}
}

func TestAC19_DB_BookingWithoutSessionIs401AndBookingsUnchanged(t *testing.T) {
	base, _, count := newDBApp(t)
	before := count("bookings")
	r := do(t, newClient(t), http.MethodPost, base+"/labs/1/book", url.Values{})
	if r.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", r.status)
	}
	if after := count("bookings"); after != before {
		t.Fatalf("bookings went from %d to %d", before, after)
	}
}

func TestAC21_DB_SignOutDeletesTheSessionRow(t *testing.T) {
	base, _, count := newDBApp(t)
	c := newClient(t)
	signIn(t, c, base, seed.StudentEmail, seedPasswords.Student)
	if n := count("sessions"); n != 1 {
		t.Fatalf("sessions after sign-in = %d, want 1", n)
	}
	wantRedirect(t, do(t, c, http.MethodPost, base+"/logout", url.Values{}), http.StatusSeeOther, "/login")
	if n := count("sessions"); n != 0 {
		t.Fatalf("sessions after sign-out = %d, want 0", n)
	}
	wantRedirect(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusSeeOther, "/login")
}

// seededLabID finds a seeded lab available on the fixed clock's day by its title.
func seededLabID(t *testing.T, st *store.Store, title string) int64 {
	t.Helper()
	day := time.Date(fixedNow.Year(), fixedNow.Month(), fixedNow.Day(), 0, 0, 0, 0, time.UTC)
	labs, err := st.LabsAvailableBetween(context.Background(), day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range labs {
		if l.Title == title {
			return l.ID
		}
	}
	t.Fatalf("no seeded lab %q", title)
	return 0
}

func TestAC12_DB_StaffAddsLabAndLabsGainsOneRow(t *testing.T) {
	base, _, count := newDBApp(t)
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	before := count("labs")
	r := do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", "1"))
	if r.status != http.StatusSeeOther || !strings.HasPrefix(r.header.Get("Location"), "/staff/labs/") {
		t.Fatalf("got %d Location=%q\n%s", r.status, r.header.Get("Location"), r.body)
	}
	if after := count("labs"); after != before+1 {
		t.Fatalf("labs went from %d to %d, want one more", before, after)
	}
	wantStatus(t, do(t, c, http.MethodGet, base+r.header.Get("Location"), nil), http.StatusOK, "Lab added.")
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "Optics")
}

func TestAC13_DB_BadPlaceLimitCreatesNoRow(t *testing.T) {
	base, _, count := newDBApp(t)
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	before := count("labs")
	for _, limit := range []string{"0", "-1", "abc"} {
		wantStatus(t, do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", limit)), http.StatusBadRequest, msgBadLimit)
	}
	if after := count("labs"); after != before {
		t.Fatalf("labs went from %d to %d", before, after)
	}
}

func TestAC2_DB_DuplicateTitleIs409AndNoRow(t *testing.T) {
	base, _, count := newDBApp(t)
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	before := count("labs")
	wantStatus(t, do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Chemistry basics", "5")), http.StatusConflict, msgChooseTitle)
	if after := count("labs"); after != before {
		t.Fatalf("labs went from %d to %d", before, after)
	}
}

func TestAC15_DB_StudentGetsAndPosts403AndNoRow(t *testing.T) {
	base, _, count := newDBApp(t)
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	before := count("labs")
	wantStatus(t, do(t, c, http.MethodGet, base+"/staff/labs/new", nil), http.StatusForbidden, msgForbidden)
	wantStatus(t, do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", "5")), http.StatusForbidden, msgForbidden)
	if after := count("labs"); after != before {
		t.Fatalf("labs went from %d to %d", before, after)
	}
}

func TestAC14_DB_StaffSeesExactlyTheStudentsWhoBooked(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	ctx := context.Background()
	for _, u := range []store.User{
		{Email: "ana@labwiz.test", Name: "Ana Example", PasswordHash: "unused", Role: "student"},
		{Email: "ben@labwiz.test", Name: "Ben Example", PasswordHash: "unused", Role: "student"},
	} {
		if err := st.UpsertUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	userID := func(email string) int64 {
		u, err := st.UserByEmail(ctx, email)
		if err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	chemistry, circuits := seededLabID(t, st, "Chemistry basics"), seededLabID(t, st, "Circuits workshop")
	storetest.InsertBooking(t, pool, userID(seed.StudentEmail), chemistry)
	storetest.InsertBooking(t, pool, userID("ana@labwiz.test"), chemistry)
	storetest.InsertBooking(t, pool, userID("ben@labwiz.test"), circuits)

	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	r := do(t, c, http.MethodGet, base+"/staff/labs/"+strconv.FormatInt(chemistry, 10), nil)
	for _, want := range []string{"Sam Student", "Ana Example"} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if strings.Contains(r.body, "Ben Example") {
		t.Error("the table lists a student who booked another lab")
	}
	if n := strings.Count(r.body, "<tr><td>"); n != 2 {
		t.Errorf("table rows = %d, want 2", n)
	}
}

func TestAC45_DB_StudentGetsStaffLabPage403(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	ctx := context.Background()
	u, err := st.UserByEmail(ctx, seed.StudentEmail)
	if err != nil {
		t.Fatal(err)
	}
	chemistry := seededLabID(t, st, "Chemistry basics")
	storetest.InsertBooking(t, pool, u.ID, chemistry)

	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	r := do(t, c, http.MethodGet, base+"/staff/labs/"+strconv.FormatInt(chemistry, 10), nil)
	wantStatus(t, r, http.StatusForbidden, msgForbidden)
	if strings.Contains(r.body, labBookersTable) || strings.Contains(r.body, "<table") {
		t.Error("a student is shown the booking list")
	}
}

func TestAC10_DB_LabsShowFreePlacesFromBookingsRows(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	ctx := context.Background()
	if err := st.UpsertUser(ctx, store.User{Email: "ana@labwiz.test", Name: "Ana Example", PasswordHash: "unused", Role: "student"}); err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{seed.StudentEmail, "ana@labwiz.test"} {
		u, err := st.UserByEmail(ctx, email)
		if err != nil {
			t.Fatal(err)
		}
		storetest.InsertBooking(t, pool, u.ID, seededLabID(t, st, "Circuits workshop"))
		if email == seed.StudentEmail {
			storetest.InsertBooking(t, pool, u.ID, seededLabID(t, st, "Chemistry basics"))
		}
	}

	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	r := do(t, c, http.MethodGet, base+"/labs", nil)
	for _, want := range []string{
		"Chemistry basics", "Titration and safe handling of reagents.", "09:00–12:00", "9 of 10 places free",
		"Circuits workshop", "13:00–17:00", "0 of 2 places free",
		"Open microscopy", "08:00–20:00", "5 of 5 places free",
	} {
		wantStatus(t, r, http.StatusOK, want)
	}
}

func TestAC23_DB_NoLabTodayShowsEmptyState(t *testing.T) {
	base, _, pool := newDBAppWithPool(t)
	// Leave only the seeded lab that starts tomorrow.
	if _, err := pool.Exec(context.Background(), `DELETE FROM labs WHERE title <> 'Robotics intro'`); err != nil {
		t.Fatal(err)
	}
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	r := do(t, c, http.MethodGet, base+"/labs", nil)
	wantStatus(t, r, http.StatusOK, "No labs available today")
	if strings.Contains(r.body, "Robotics intro") {
		t.Error("/labs shows the lab that starts tomorrow")
	}
}
