package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// These tests run against real PostgreSQL and skip when TEST_DATABASE_URL is not set.

var seedPasswords = seed.Passwords{Student: "student-test-pass", Staff: "staff-test-pass"}

func newDBApp(t *testing.T) (string, *store.Store, func(table string) int) {
	t.Helper()
	pool := storetest.Open(t)
	st := store.New(pool)
	if err := seed.Run(context.Background(), st, fixedNow, time.UTC, seedPasswords); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return newServer(t, st, st).URL, st, func(table string) int { return storetest.Count(t, pool, table) }
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
