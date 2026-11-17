package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
)

// Slice 3d against real PostgreSQL. The seed puts Robotics intro tomorrow (23 October 2026).

func TestAC80_DB_TomorrowsListShowsTomorrowsSessionOnly(t *testing.T) {
	base, _, _ := newDBAppWithPool(t)
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	tomorrow := do(t, c, http.MethodGet, base+"/labs?date=2026-10-23", nil)
	wantStatus(t, tomorrow, http.StatusOK, "Labs on Friday 23 October 2026")
	wantStatus(t, tomorrow, http.StatusOK, "Robotics intro")
	if strings.Contains(tomorrow.body, "Chemistry basics") {
		t.Error("tomorrow's list shows today's Chemistry basics")
	}
	if strings.Contains(do(t, c, http.MethodGet, base+"/labs", nil).body, "Robotics intro") {
		t.Error("today's list shows tomorrow's Robotics intro")
	}
}

func TestAC82_DB_AnEmptyDayFarAheadSaysSo(t *testing.T) {
	base, _, _ := newDBAppWithPool(t)
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs?date=2027-03-01", nil), http.StatusOK, "No labs on this day")
}
