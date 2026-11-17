package handler_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

const (
	omarEmail = "omar@labwiz.test"
	omarPass  = "omar-password"
	omarID    = 3
)

// newTwoStaffApp is newStaffApp plus a second staff member, Omar, who owns lab 4 (today).
// Stella owns labs 1-3.
func newTwoStaffApp(t *testing.T) (string, *servicetest.Store) {
	t.Helper()
	base, st := newStaffApp(t)
	hash, err := bcrypt.GenerateFromPassword([]byte(omarPass), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	st.AddUser(store.User{ID: omarID, Email: omarEmail, Name: "Omar Haddad", PasswordHash: string(hash), Role: "staff"})
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	st.Labs = append(st.Labs, store.Lab{
		ID: 4, Title: "Soldering practice", StartsAt: day.Add(10 * time.Hour), EndsAt: day.Add(11 * time.Hour), PlaceLimit: 5, OwnerID: omarID,
	})
	return base, st
}

func TestAC61_LabsShowTheirOwner(t *testing.T) {
	base, _ := newTwoStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	list := do(t, c, http.MethodGet, base+"/labs", nil)
	for _, want := range []string{"Run by Stella Staff", "Run by Omar Haddad"} {
		wantStatus(t, list, http.StatusOK, want)
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs/4", nil), http.StatusOK, "Run by Omar Haddad")
}

func TestAC62_MyLabsListsExactlyTheCallersLabs(t *testing.T) {
	base, _ := newTwoStaffApp(t)
	stella := do(t, signedIn(t, base, staffEmail, staffPass), http.MethodGet, base+"/staff/labs", nil)
	// Robotics intro starts tomorrow: My labs is not limited to today.
	for _, want := range []string{
		`<a href="/staff/labs/1">Chemistry basics</a>`, `<a href="/staff/labs/2">Circuits workshop</a>`,
		`<a href="/staff/labs/3">Robotics intro</a>`, "2 of 10 places booked",
	} {
		wantStatus(t, stella, http.StatusOK, want)
	}
	if strings.Contains(stella.body, "Soldering practice") {
		t.Error("Stella's labs include Omar's")
	}
	omar := do(t, signedIn(t, base, omarEmail, omarPass), http.MethodGet, base+"/staff/labs", nil)
	wantStatus(t, omar, http.StatusOK, `<a href="/staff/labs/4">Soldering practice</a>`)
	if n := strings.Count(omar.body, `class="lab-list__item"`); n != 1 {
		t.Errorf("Omar's list has %d labs, want 1", n)
	}
}

func TestAC63_MyLabsWithoutLabsShowsTheEmptyState(t *testing.T) {
	base, st := newTwoStaffApp(t)
	st.Labs = st.Labs[:3] // Omar's lab is gone
	r := do(t, signedIn(t, base, omarEmail, omarPass), http.MethodGet, base+"/staff/labs", nil)
	wantStatus(t, r, http.StatusOK, "You have no labs yet.")
	wantStatus(t, r, http.StatusOK, `href="/staff/labs/new"`)
}

func TestAC64_AnotherStaffMembersLabIs403(t *testing.T) {
	base, _ := newTwoStaffApp(t)
	c := signedIn(t, base, omarEmail, omarPass)
	r := do(t, c, http.MethodGet, base+"/staff/labs/1", nil)
	wantStatus(t, r, http.StatusForbidden, msgForbidden)
	for _, leak := range []string{labBookersTable, "Ana Example", "<table"} {
		if strings.Contains(r.body, leak) {
			t.Errorf("the 403 page shows %q", leak)
		}
	}
	// The public lab page is fine, but has neither a booking-list link nor a Book button for staff.
	pub := do(t, c, http.MethodGet, base+"/labs/1", nil)
	wantStatus(t, pub, http.StatusOK, "Chemistry basics")
	for _, leak := range []string{`href="/staff/labs/1"`, bookForm} {
		if strings.Contains(pub.body, leak) {
			t.Errorf("Omar's view of Stella's lab has %q", leak)
		}
	}
	// The owner still gets the booking list (AC-14).
	wantStatus(t, do(t, signedIn(t, base, staffEmail, staffPass), http.MethodGet, base+"/labs/1", nil), http.StatusOK, `href="/staff/labs/1"`)
}

func TestAC65_StudentGetsMyLabs403(t *testing.T) {
	base, _ := newTwoStaffApp(t)
	wantStatus(t, do(t, signedIn(t, base, studentEmail, studentPass), http.MethodGet, base+"/staff/labs", nil), http.StatusForbidden, msgForbidden)
}

func TestAC62_CardTitlesLinkStaffToTheirOwnLabsOnly(t *testing.T) {
	base, _ := newTwoStaffApp(t)
	r := do(t, signedIn(t, base, omarEmail, omarPass), http.MethodGet, base+"/labs", nil)
	wantStatus(t, r, http.StatusOK, `<a href="/staff/labs/4">Soldering practice</a>`)
	wantStatus(t, r, http.StatusOK, `<a href="/labs/1">Chemistry basics</a>`)
	if strings.Contains(r.body, `href="/staff/labs/1"`) {
		t.Error("Omar's lab list links to Stella's staff page")
	}
}

func TestAC62_HeaderLinksStaffToMyLabs(t *testing.T) {
	base, _ := newTwoStaffApp(t)
	myLabs := `href="/staff/labs">My labs</a>`
	wantStatus(t, do(t, signedIn(t, base, omarEmail, omarPass), http.MethodGet, base+"/labs", nil), http.StatusOK, myLabs)
	if strings.Contains(do(t, signedIn(t, base, studentEmail, studentPass), http.MethodGet, base+"/labs", nil).body, myLabs) {
		t.Error("students are shown the My labs link")
	}
}

func TestAC15_StudentPostOfNewLabIs403WithoutHandlerRoleCheck(t *testing.T) {
	base, st := newTwoStaffApp(t)
	before := st.LabCount()
	r := do(t, signedIn(t, base, studentEmail, studentPass), http.MethodPost, base+"/staff/labs/new", url.Values{
		"title": {"Sneaky"}, "starts": {"2026-10-22T09:00"}, "ends": {"2026-10-22T10:00"}, "place_limit": {"3"},
	})
	wantStatus(t, r, http.StatusForbidden, msgForbidden)
	if st.LabCount() != before {
		t.Fatal("a student added a lab")
	}
}

func TestMyLabsPageMeetsTheDefinitionOfDone_AC30_AC41_AC42(t *testing.T) {
	base, _ := newTwoStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	r := do(t, c, http.MethodGet, base+"/staff/labs", nil)
	if m := regexp.MustCompile(`(?i)transition|animation|@keyframes`).FindString(r.body); m != "" {
		t.Errorf("/staff/labs contains %q", m)
	}
	if strings.Contains(r.body, "<input") {
		t.Error("/staff/labs has an input; AC-41 needs it labelled")
	}
	title := regexp.MustCompile(`<title>([^<]+)</title>`).FindStringSubmatch(r.body)
	if title == nil || title[1] != "My labs · Labwiz" || !strings.Contains(r.body, `<html lang="en">`) {
		t.Errorf("title = %v, want My labs · Labwiz and lang=en", title)
	}
	for _, path := range []string{"/labs", "/labs/1", "/staff/labs/new", "/staff/labs/1"} {
		if other := regexp.MustCompile(`<title>([^<]+)</title>`).FindStringSubmatch(do(t, c, http.MethodGet, base+path, nil).body); other != nil && other[1] == title[1] {
			t.Errorf("%s shares the title %q", path, title[1])
		}
	}
}
