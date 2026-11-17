package handler_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// Slice 3e against real PostgreSQL.

func TestAC85_DB_ChangesWithoutTheTokenLeaveTheDatabaseAsItWas(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	student := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	staff := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	chemistry := seeded(t, st, "Chemistry basics")
	robotics := firstSessionOfView(t, st, pool, "Robotics intro")
	count := func() map[string]int {
		return map[string]int{
			"labs": storetest.Count(t, pool, "labs"), "lab_sessions": storetest.Count(t, pool, "lab_sessions"),
			"bookings": storetest.Count(t, pool, "bookings"), "sessions": storetest.Count(t, pool, "sessions"),
		}
	}
	before := count()
	bad := url.Values{"csrf": {"not-the-token"}}
	for _, p := range []struct {
		c    *http.Client
		path string
		form url.Values
	}{
		{student, bookPath(chemistry.SessionID), bad},
		{staff, "/staff/labs/new", cloneWith(url.Values{"title": {"Optics"}, "place_limit": {"3"}}, "csrf", "")},
		{staff, staffLabPath(chemistry.LabID) + "/sessions", cloneWith(sessionForm(), "csrf", "x")},
		{staff, cancelPath(robotics.SessionID), bad},
		{staff, staffLabPath(robotics.LabID) + "/delete", bad},
		{staff, "/logout", bad},
	} {
		wantStatus(t, do(t, p.c, http.MethodPost, base+p.path, p.form), http.StatusForbidden, msgExpired)
	}
	after := count()
	for table, n := range before {
		if after[table] != n {
			t.Errorf("%s went from %d to %d", table, n, after[table])
		}
	}
	if cancelledDB(t, pool, robotics.SessionID) {
		t.Error("the session was cancelled")
	}
}

func TestAC86_DB_AnotherAccountsTokenIsRefused(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	addStaff(t, st, omarEmail, omarPass)
	stella := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	omar := signedIn(t, base, omarEmail, omarPass)
	before := storetest.Count(t, pool, "labs")
	r := do(t, omar, http.MethodPost, base+"/staff/labs/new",
		url.Values{"title": {"Optics"}, "place_limit": {"3"}, "csrf": {tokenFor(stella, base+"/labs")}})
	wantStatus(t, r, http.StatusForbidden, msgExpired)
	if after := storetest.Count(t, pool, "labs"); after != before {
		t.Fatalf("labs went from %d to %d", before, after)
	}
}
