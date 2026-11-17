package handler_test

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

const (
	staffEmail = "staff@labwiz.test"
	staffPass  = "staff-password"

	msgForbidden    = "You are not allowed to do this."
	msgBadLimit     = "The place limit must be a whole number of 1 or more."
	msgLabNotFound  = "This lab does not exist."
	msgChooseTitle  = "Please choose a different title"
	labBookersTable = "Booked by"
)

// newStaffApp is newFakeApp plus a staff member, and bookings for lab 1 (Ana, Ben) and lab 2 (Cleo).
func newStaffApp(t *testing.T) (string, *servicetest.Store) {
	t.Helper()
	base, st := newFakeApp(t)
	hash, err := bcrypt.GenerateFromPassword([]byte(staffPass), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	st.AddUser(store.User{ID: stellaID, Email: staffEmail, Name: "Stella Staff", PasswordHash: string(hash), Role: "staff"})
	st.Bookers[1] = []store.Booker{{Name: "Ana Example", Email: "ana@labwiz.test"}, {Name: "Ben Example", Email: "ben@labwiz.test"}}
	st.Bookers[2] = []store.Booker{{Name: "Cleo Example", Email: "cleo@labwiz.test"}}
	return base, st
}

func signedIn(t *testing.T, base, email, password string) *http.Client {
	t.Helper()
	c := newClient(t)
	wantRedirect(t, signIn(t, c, base, email, password), http.StatusSeeOther, "/labs")
	return c
}

// labForm is a valid add-lab form. A lab has no time period: its sessions do.
func labForm(title, limit string) url.Values {
	return url.Values{"title": {title}, "description": {"Lenses and lasers."}, "place_limit": {limit}}
}

// sessionForm is a valid add-session form for 14:00–16:00 on the fixed clock's day.
func sessionForm() url.Values {
	return url.Values{"starts": {"2026-10-22T14:00"}, "ends": {"2026-10-22T16:00"}}
}

func wantStatus(t *testing.T, r response, status int, sentence string) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status = %d, want %d\n%s", r.status, status, r.body)
	}
	if !strings.Contains(r.body, sentence) {
		t.Fatalf("body lacks %q\n%s", sentence, r.body)
	}
}

func TestAC12_StaffAddsLabAndItAppearsOnLabs(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	wantStatus(t, do(t, c, http.MethodGet, base+"/staff/labs/new", nil), http.StatusOK, `action="/staff/labs/new"`)

	r := do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", "1"))
	wantRedirect(t, r, http.StatusSeeOther, "/staff/labs/4?created=1")
	if n := st.LabCount(); n != 4 {
		t.Fatalf("labs = %d, want 4", n)
	}

	detail := do(t, c, http.MethodGet, base+"/staff/labs/4?created=1", nil)
	for _, want := range []string{"Lab added. Add its first session below.", "Optics", "Lenses and lasers.", "No upcoming sessions."} {
		wantStatus(t, detail, http.StatusOK, want)
	}
	if strings.Contains(do(t, c, http.MethodGet, base+"/staff/labs/4", nil).body, "Lab added.") {
		t.Error("the success message shows without ?created=1")
	}
	// It appears on /labs once it has a session today (AC-67).
	if strings.Contains(do(t, c, http.MethodGet, base+"/labs", nil).body, "Optics") {
		t.Error("a lab with no session is on /labs")
	}
	wantRedirect(t, do(t, c, http.MethodPost, base+"/staff/labs/4/sessions", sessionForm()), http.StatusSeeOther, "/staff/labs/4?scheduled=1")
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "Optics")
}

func TestAC13_BadPlaceLimitShowsMessageAndCreatesNothing(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	for _, limit := range []string{"0", "-3", "many"} {
		r := do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", limit))
		wantStatus(t, r, http.StatusBadRequest, msgBadLimit)
		if !strings.Contains(r.body, `value="Optics"`) {
			t.Errorf("place limit %q: the typed title is lost", limit)
		}
		if n := st.LabCount(); n != 3 {
			t.Fatalf("place limit %q: labs = %d, want 3", limit, n)
		}
	}
}

func TestAC2_DuplicateTitleIs409(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	r := do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Chemistry basics", "5"))
	wantStatus(t, r, http.StatusConflict, msgChooseTitle)
	if n := st.LabCount(); n != 3 {
		t.Fatalf("labs = %d, want 3", n)
	}
}

func TestAC15_StudentGetAndPostNewLabAre403(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	wantStatus(t, do(t, c, http.MethodGet, base+"/staff/labs/new", nil), http.StatusForbidden, msgForbidden)
	wantStatus(t, do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", "5")), http.StatusForbidden, msgForbidden)
	if n := st.LabCount(); n != 3 {
		t.Fatalf("labs = %d, want 3", n)
	}
	if strings.Contains(do(t, c, http.MethodGet, base+"/labs", nil).body, "/staff/labs") {
		t.Error("a student is shown links to staff pages")
	}
}

func TestAC14_StaffSeesBookersTable(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	r := do(t, c, http.MethodGet, base+"/staff/labs/1", nil)
	for _, want := range []string{"Chemistry basics", labBookersTable, "Ana Example", "Ben Example", "2 of 10 places booked"} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if strings.Contains(r.body, "Cleo Example") {
		t.Error("the table lists a student who booked another lab")
	}
	if n := strings.Count(r.body, "<tr><td>"); n != 2 {
		t.Errorf("table rows = %d, want 2", n)
	}
	if !strings.Contains(do(t, c, http.MethodGet, base+"/labs", nil).body, `href="/staff/labs/1"`) {
		t.Error("/labs does not link staff to the lab's bookings")
	}
}

func TestAC45_StudentGetsStaffLabPage403(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	for _, path := range []string{"/staff/labs/1", "/staff/labs/99", "/staff/labs/abc"} {
		r := do(t, c, http.MethodGet, base+path, nil)
		wantStatus(t, r, http.StatusForbidden, msgForbidden)
		if strings.Contains(r.body, labBookersTable) || strings.Contains(r.body, "Ana Example") {
			t.Errorf("%s shows the booking list to a student", path)
		}
	}
}

func TestAC9_StaffPagesRedirectVisitorToLogin(t *testing.T) {
	base, st := newStaffApp(t)
	c := newClient(t)
	wantRedirect(t, do(t, c, http.MethodGet, base+"/staff/labs/new", nil), http.StatusSeeOther, "/login")
	wantRedirect(t, do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", "5")), http.StatusSeeOther, "/login")
	wantRedirect(t, do(t, c, http.MethodGet, base+"/staff/labs/1", nil), http.StatusSeeOther, "/login")
	if n := st.LabCount(); n != 3 {
		t.Fatalf("labs = %d, want 3", n)
	}
}

func TestStaffLabPageUnknownLabIs404(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	for _, path := range []string{"/staff/labs/99", "/staff/labs/abc"} {
		wantStatus(t, do(t, c, http.MethodGet, base+path, nil), http.StatusNotFound, msgLabNotFound)
	}
}

func TestStaffLabPageShowsAPeriodAcrossDays(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	wantRedirect(t, do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Night lab", "4")), http.StatusSeeOther, "/staff/labs/4?created=1")
	form := url.Values{"starts": {"2026-10-22T22:00"}, "ends": {"2026-10-23T02:00"}}
	wantRedirect(t, do(t, c, http.MethodPost, base+"/staff/labs/4/sessions", form), http.StatusSeeOther, "/staff/labs/4?scheduled=1")
	wantStatus(t, do(t, c, http.MethodGet, base+"/staff/labs/4", nil), http.StatusOK, "22 Oct 22:00 – 23 Oct 02:00")
}

func TestAddLabFormThatCannotBeReadIs400(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	form := labForm("Optics", "5")
	form.Set("description", strings.Repeat("x", 70<<10))
	wantStatus(t, do(t, c, http.MethodPost, base+"/staff/labs/new", form), http.StatusBadRequest, "The lab form could not be read.")
	if n := st.LabCount(); n != 3 {
		t.Fatalf("labs = %d, want 3", n)
	}
}

func TestAC30_StaffPagesHaveNoAnimationThemeToggleOrLanguageSwitcher(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	forbidden := regexp.MustCompile(`(?i)transition|animation|@keyframes|prefers-color-scheme|data-theme|hreflang`)
	for _, path := range []string{"/staff/labs/new", "/staff/labs/1", "/labs"} {
		r := do(t, c, http.MethodGet, base+path, nil)
		if r.status != http.StatusOK {
			t.Fatalf("GET %s = %d", path, r.status)
		}
		if m := forbidden.FindString(r.body); m != "" {
			t.Errorf("%s contains %q", path, m)
		}
	}
}

func TestAC41_AddLabFormFieldsHaveLabels(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	// The add-lab form (title, description, place limit) and the add-session form (starts, ends, repeat).
	for path, want := range map[string]int{"/staff/labs/new": 3, "/staff/labs/1": 3} {
		body := do(t, c, http.MethodGet, base+path, nil).body
		ids := regexp.MustCompile(`<(?:input|textarea|select) id="([^"]+)"`).FindAllStringSubmatch(body, -1)
		if len(ids) != want {
			t.Fatalf("found %d fields on %s, want %d", len(ids), path, want)
		}
		for _, m := range ids {
			if !strings.Contains(body, `<label for="`+m[1]+`">`) {
				t.Errorf("%s: field %q has no label", path, m[1])
			}
		}
	}
}

// The header's "Labwiz" also links to /labs, so look for the visible link text, not just the href.
func TestStaffPagesLinkBackToTheLabList(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	back := `<a href="/labs">Back to the lab list</a>`
	for _, path := range []string{"/staff/labs/new", "/staff/labs/1"} {
		wantStatus(t, do(t, c, http.MethodGet, base+path, nil), http.StatusOK, back)
	}
	// Also when the form comes back with a validation message.
	wantStatus(t, do(t, c, http.MethodPost, base+"/staff/labs/new", labForm("Optics", "0")), http.StatusBadRequest, back)
}

func TestAC42_StaffPagesHaveLangEnAndTheirOwnTitles(t *testing.T) {
	base, _ := newStaffApp(t)
	c := newClient(t)
	pages := map[string]string{"/login": do(t, c, http.MethodGet, base+"/login", nil).body}
	signIn(t, c, base, staffEmail, staffPass)
	for _, path := range []string{"/labs", "/staff/labs/new", "/staff/labs/1", "/staff/labs/2"} {
		pages[path] = do(t, c, http.MethodGet, base+path, nil).body
	}
	pages["error"] = do(t, c, http.MethodGet, base+"/staff/labs/99", nil).body

	titleRe := regexp.MustCompile(`<title>([^<]+)</title>`)
	seen := map[string]string{}
	for page, body := range pages {
		if !strings.Contains(body, `<html lang="en">`) {
			t.Errorf("%s lacks <html lang=\"en\">", page)
		}
		m := titleRe.FindStringSubmatch(body)
		if m == nil {
			t.Errorf("%s has no <title>", page)
			continue
		}
		if other, dup := seen[m[1]]; dup {
			t.Errorf("%s and %s share the title %q", page, other, m[1])
		}
		seen[m[1]] = page
	}
}

func TestAC43_StaffFailuresShowOneReadableSentence(t *testing.T) {
	base, st := newStaffApp(t)
	staff := signedIn(t, base, staffEmail, staffPass)
	student := signedIn(t, base, studentEmail, studentPass)
	sqlErr := errors.New(`ERROR: relation "labs" does not exist (SQLSTATE 42P01)`)

	cases := []struct {
		name     string
		c        *http.Client
		method   string
		path     string
		form     url.Values
		fail     func()
		status   int
		sentence string
	}{
		{"validation", staff, http.MethodPost, "/staff/labs/new", labForm("Optics", "0"), nil, http.StatusBadRequest, msgBadLimit},
		{"forbidden", student, http.MethodGet, "/staff/labs/new", nil, nil, http.StatusForbidden, msgForbidden},
		{"not found", staff, http.MethodGet, "/staff/labs/99", nil, nil, http.StatusNotFound, msgLabNotFound},
		{"duplicate", staff, http.MethodPost, "/staff/labs/new", labForm("Chemistry basics", "5"), nil, http.StatusConflict, msgChooseTitle},
		{"add fails", staff, http.MethodPost, "/staff/labs/new", labForm("Optics", "5"), func() { st.ErrCreateLab = sqlErr },
			http.StatusInternalServerError, "Something went wrong on our side. Please try again."},
		{"bookers fail", staff, http.MethodGet, "/staff/labs/1", nil, func() { st.ErrSessionBookers = sqlErr },
			http.StatusInternalServerError, "Something went wrong on our side. Please try again."},
	}
	leak := regexp.MustCompile(`SQLSTATE|ERROR:|relation|goroutine|panic|\{"`)
	for _, tc := range cases {
		if tc.fail != nil {
			tc.fail()
		}
		r := do(t, tc.c, tc.method, base+tc.path, tc.form)
		if r.status != tc.status || !strings.Contains(r.body, tc.sentence) {
			t.Errorf("%s: got %d, want %d with %q", tc.name, r.status, tc.status, tc.sentence)
		}
		if m := leak.FindString(r.body); m != "" {
			t.Errorf("%s: leaks %q", tc.name, m)
		}
	}
}
