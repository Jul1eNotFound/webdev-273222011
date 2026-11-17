package handler_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

func TestAC10_LabCardsShowTitleDescriptionPeriodAndFreePlaces(t *testing.T) {
	base, st := newFakeApp(t)
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	st.Labs[0].Description = "Titration and safe handling of reagents."
	st.Labs = append(st.Labs, store.Lab{
		ID: 4, Title: "Night shift", Description: "Runs past midnight.",
		StartsAt: day.Add(22 * time.Hour), EndsAt: day.Add(26 * time.Hour), PlaceLimit: 3,
	})
	st.Bookers[1] = []store.Booker{{Name: "Ana Example"}, {Name: "Ben Example"}, {Name: "Cleo Example"}}
	st.Bookers[2] = []store.Booker{{Name: "Ana Example"}, {Name: "Ben Example"}}

	c := signedIn(t, base, studentEmail, studentPass)
	r := do(t, c, http.MethodGet, base+"/labs", nil)
	for _, want := range []string{
		"Chemistry basics", "Titration and safe handling of reagents.", "09:00–12:00", "7 of 10 places free",
		"Circuits workshop", "13:00–17:00", "0 of 2 places free",
		"Night shift", "Runs past midnight.", "22 Oct 22:00 – 23 Oct 02:00", "3 of 3 places free",
	} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if strings.Contains(r.body, "No labs available today") {
		t.Error("the empty state is shown next to labs")
	}
}

func TestAC23_NoLabTodayShowsEmptyState(t *testing.T) {
	base, st := newFakeApp(t)
	// Keep only the lab that starts tomorrow.
	st.Labs = st.Labs[2:]
	c := signedIn(t, base, studentEmail, studentPass)
	r := do(t, c, http.MethodGet, base+"/labs", nil)
	wantStatus(t, r, http.StatusOK, "No labs available today")
	if strings.Contains(r.body, "Robotics intro") || strings.Contains(r.body, "lab-list__item") {
		t.Errorf("the empty state also shows a lab\n%s", r.body)
	}
}
