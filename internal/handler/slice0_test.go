package handler_test

import (
	"errors"
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
	studentEmail = "student@labwiz.test"
	studentPass  = "right-password"
)

// newFakeApp serves the app over the in-memory store: one student, labs today and tomorrow.
func newFakeApp(t *testing.T) (string, *servicetest.Store) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(studentPass), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: 1, Email: studentEmail, Name: "Sam Student", PasswordHash: string(hash), Role: "student"})
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	st.Labs = []store.Lab{
		{ID: 1, Title: "Chemistry basics", StartsAt: day.Add(9 * time.Hour), EndsAt: day.Add(12 * time.Hour), PlaceLimit: 10},
		{ID: 2, Title: "Circuits workshop", StartsAt: day.Add(13 * time.Hour), EndsAt: day.Add(17 * time.Hour), PlaceLimit: 2},
		{ID: 3, Title: "Robotics intro", StartsAt: day.Add(33 * time.Hour), EndsAt: day.Add(36 * time.Hour), PlaceLimit: 8},
	}
	return newServer(t, st, st).URL, st
}

func TestAC3_StudentSignsInAndLandsOnTodaysLabs(t *testing.T) {
	base, _ := newFakeApp(t)
	c := newClient(t)
	wantRedirect(t, signIn(t, c, base, studentEmail, studentPass), http.StatusSeeOther, "/labs")

	r := do(t, c, http.MethodGet, base+"/labs", nil)
	if r.status != http.StatusOK {
		t.Fatalf("GET /labs = %d", r.status)
	}
	for _, title := range []string{"Chemistry basics", "Circuits workshop", "Sam Student"} {
		if !strings.Contains(r.body, title) {
			t.Errorf("/labs does not show %q", title)
		}
	}
	if strings.Contains(r.body, "Robotics intro") {
		t.Error("/labs shows a lab that starts tomorrow")
	}
	wantRedirect(t, do(t, c, http.MethodGet, base+"/", nil), http.StatusFound, "/labs")
	wantRedirect(t, do(t, c, http.MethodGet, base+"/login", nil), http.StatusSeeOther, "/labs")
}

func TestAC9_ProtectedPageRedirectsVisitorToLogin(t *testing.T) {
	base, _ := newFakeApp(t)
	c := newClient(t)
	wantRedirect(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusSeeOther, "/login")

	u, _ := url.Parse(base)
	c.Jar.SetCookies(u, []*http.Cookie{{Name: "labwiz_session", Value: "forged-token"}})
	wantRedirect(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusSeeOther, "/login")
}

func TestAC17_WrongPasswordAndUnknownEmailShowTheSameText(t *testing.T) {
	base, _ := newFakeApp(t)
	wrong := signIn(t, newClient(t), base, studentEmail, "not-it")
	unknown := signIn(t, newClient(t), base, "nobody@labwiz.test", studentPass)
	for name, r := range map[string]response{"wrong password": wrong, "unknown email": unknown} {
		if r.status != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, r.status)
		}
		if !strings.Contains(r.body, "Invalid email or password") {
			t.Errorf("%s: body lacks the message", name)
		}
		if r.header.Get("Set-Cookie") != "" {
			t.Errorf("%s: a cookie was set", name)
		}
	}
	errLine := regexp.MustCompile(`role="alert">([^<]*)<`)
	if a, b := errLine.FindStringSubmatch(wrong.body), errLine.FindStringSubmatch(unknown.body); a == nil || b == nil || a[1] != b[1] {
		t.Fatalf("error lines differ: %q vs %q", a, b)
	}
}

func TestAC19_BookingWithoutSessionIs401(t *testing.T) {
	base, _ := newFakeApp(t)
	r := do(t, newClient(t), http.MethodPost, base+"/labs/1/book", url.Values{})
	if r.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", r.status)
	}
	if !strings.Contains(r.body, "Please sign in to continue.") {
		t.Fatalf("body lacks the sentence:\n%s", r.body)
	}
}

func TestAC21_SignOutThenBackAndReloadRedirectsToLogin(t *testing.T) {
	base, _ := newFakeApp(t)
	c := newClient(t)
	signIn(t, c, base, studentEmail, studentPass)
	labs := do(t, c, http.MethodGet, base+"/labs", nil)
	if labs.status != http.StatusOK || labs.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("GET /labs = %d, Cache-Control %q", labs.status, labs.header.Get("Cache-Control"))
	}
	u, _ := url.Parse(base)
	oldCookies := c.Jar.Cookies(u)

	wantRedirect(t, do(t, c, http.MethodPost, base+"/logout", url.Values{}), http.StatusSeeOther, "/login")
	wantRedirect(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusSeeOther, "/login")

	// A browser that kept the old cookie still gets nothing: the session row is gone.
	replay := newClient(t)
	replay.Jar.SetCookies(u, oldCookies)
	wantRedirect(t, do(t, replay, http.MethodGet, base+"/labs", nil), http.StatusSeeOther, "/login")
}

func TestAC30_NoAnimationThemeToggleOrLanguageSwitcher(t *testing.T) {
	base, _ := newFakeApp(t)
	c := newClient(t)
	signIn(t, c, base, studentEmail, studentPass)
	forbidden := regexp.MustCompile(`(?i)transition|animation|@keyframes|prefers-color-scheme|data-theme|hreflang`)
	for _, path := range []string{"/static/style.css", "/labs", "/login"} {
		r := do(t, c, http.MethodGet, base+path, nil)
		if r.status != http.StatusOK && path != "/login" {
			t.Fatalf("GET %s = %d", path, r.status)
		}
		if m := forbidden.FindString(r.body); m != "" {
			t.Errorf("%s contains %q", path, m)
		}
	}
}

func TestAC41_EveryInputHasALabel(t *testing.T) {
	base, _ := newFakeApp(t)
	body := do(t, newClient(t), http.MethodGet, base+"/login", nil).body
	ids := regexp.MustCompile(`<input id="([^"]+)"`).FindAllStringSubmatch(body, -1)
	if len(ids) == 0 {
		t.Fatal("no inputs found on /login")
	}
	for _, m := range ids {
		if !strings.Contains(body, `<label for="`+m[1]+`">`) {
			t.Errorf("input %q has no label", m[1])
		}
	}
}

func TestAC42_EachPageHasLangEnAndItsOwnTitle(t *testing.T) {
	base, _ := newFakeApp(t)
	c := newClient(t)
	pages := map[string]string{"/login": do(t, c, http.MethodGet, base+"/login", nil).body}
	signIn(t, c, base, studentEmail, studentPass)
	pages["/labs"] = do(t, c, http.MethodGet, base+"/labs", nil).body
	pages["error"] = do(t, c, http.MethodGet, base+"/no-such-page", nil).body

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

func TestAC43_FailuresShowOneReadableSentence(t *testing.T) {
	base, st := newFakeApp(t)
	c := newClient(t)
	signIn(t, c, base, studentEmail, studentPass)
	st.ErrLabs = errors.New(`pq: SQLSTATE 42P01 relation "labs" does not exist`)

	cases := []struct {
		method, path string
		status       int
		sentence     string
	}{
		{http.MethodGet, "/no-such-page", http.StatusNotFound, "This page does not exist."},
		{http.MethodDelete, "/login", http.StatusMethodNotAllowed, "This action is not allowed on this page."},
		{http.MethodGet, "/labs", http.StatusInternalServerError, "Something went wrong on our side. Please try again."},
	}
	leak := regexp.MustCompile(`SQLSTATE|pq:|relation|goroutine|panic|\{"`)
	for _, tc := range cases {
		r := do(t, c, tc.method, base+tc.path, nil)
		if r.status != tc.status {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, r.status, tc.status)
		}
		if !strings.Contains(r.body, tc.sentence) {
			t.Errorf("%s %s lacks %q", tc.method, tc.path, tc.sentence)
		}
		if m := leak.FindString(r.body); m != "" {
			t.Errorf("%s %s leaks %q", tc.method, tc.path, m)
		}
	}
}

func TestSignedInBookingIsNotImplementedYet(t *testing.T) {
	base, _ := newFakeApp(t)
	c := newClient(t)
	signIn(t, c, base, studentEmail, studentPass)
	r := do(t, c, http.MethodPost, base+"/labs/1/book", url.Values{})
	if r.status != http.StatusNotImplemented || !strings.Contains(r.body, "Booking is not available yet.") {
		t.Fatalf("got %d\n%s", r.status, r.body)
	}
}
