package handler_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// Slice 3a against real PostgreSQL. The seed makes Stella (seed.StaffEmail) own its four labs.

// addStaff creates a second staff member who can sign in, and returns their id.
func addStaff(t *testing.T, st *store.Store, email, password string) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(context.Background(), store.User{Email: email, Name: "Omar Haddad", PasswordHash: string(hash), Role: "staff"}); err != nil {
		t.Fatal(err)
	}
	return userIDOf(t, st, email)
}

func TestAC60_DB_AddedLabIsOwnedByTheStaffMember(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	omar := addStaff(t, st, omarEmail, omarPass)
	c := signedIn(t, base, omarEmail, omarPass)
	r := do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", "4"))
	if r.status != http.StatusSeeOther {
		t.Fatalf("add lab = %d\n%s", r.status, r.body)
	}
	var owner int64
	if err := pool.QueryRow(context.Background(), `SELECT owner_id FROM labs WHERE title = 'Optics'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != omar {
		t.Fatalf("owner_id = %d, want Omar (%d)", owner, omar)
	}
	// The new lab's staff page is Omar's, and its Location says so.
	wantStatus(t, do(t, c, http.MethodGet, base+r.header.Get("Location"), nil), http.StatusOK, "Lab added.")
}

func TestAC62_DB_EachStaffMemberSeesOnlyTheirOwnLabs(t *testing.T) {
	base, st, _ := newDBAppWithPool(t)
	addStaff(t, st, omarEmail, omarPass)
	omar := signedIn(t, base, omarEmail, omarPass)
	do(t, omar, http.MethodPost, base+"/staff/labs/new", labForm("Optics", "4"))

	stella := do(t, signedIn(t, base, seed.StaffEmail, seedPasswords.Staff), http.MethodGet, base+"/staff/labs", nil)
	for _, want := range []string{"Chemistry basics", "Circuits workshop", "Open microscopy", "Robotics intro"} {
		wantStatus(t, stella, http.StatusOK, want)
	}
	if strings.Contains(stella.body, "Optics") {
		t.Error("Stella's labs include Omar's")
	}
	mine := do(t, omar, http.MethodGet, base+"/staff/labs", nil)
	wantStatus(t, mine, http.StatusOK, "Optics")
	if strings.Contains(mine.body, "Chemistry basics") {
		t.Error("Omar's labs include Stella's")
	}
}

func TestAC64_DB_AnotherStaffMembersLabIs403(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics")
	bookByForm(t, signedIn(t, base, seed.StudentEmail, seedPasswords.Student), base, bookPath(chemistry.SessionID))
	addStaff(t, st, omarEmail, omarPass)

	r := do(t, signedIn(t, base, omarEmail, omarPass), http.MethodGet, base+staffLabPath(chemistry.LabID), nil)
	wantStatus(t, r, http.StatusForbidden, msgForbidden)
	if strings.Contains(r.body, "Sam Student") {
		t.Error("another staff member sees who booked the lab")
	}
	if n := bookingRows(t, pool, chemistry.SessionID, 0); n != 1 {
		t.Fatalf("bookings = %d, want 1", n)
	}
}
