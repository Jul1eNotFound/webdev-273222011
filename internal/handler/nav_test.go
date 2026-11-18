package handler_test

import (
	"net/http"
	"regexp"
	"testing"
)

// Every signed-in page has a "Labs" link in the header back to the lab list, and the header link of
// the page the user is on, if any, is marked aria-current="page" (the UI section's header).
func TestHeaderLinksToTheLabListAndMarksTheCurrentPage(t *testing.T) {
	base, _ := newStaffApp(t)
	current := regexp.MustCompile(`<a class="button button--secondary" aria-current="page" href="([^"]*)">`)
	for _, tc := range []struct {
		who, email, pass string
		path, want       string // want is the marked link, "" for none
	}{
		{"student", studentEmail, studentPass, "/labs", "/labs"},
		{"student", studentEmail, studentPass, "/labs?date=2026-10-23", "/labs"},
		{"student", studentEmail, studentPass, "/labs/1", ""},
		{"student", studentEmail, studentPass, "/bookings", "/bookings"},
		{"staff", staffEmail, staffPass, "/labs", "/labs"},
		{"staff", staffEmail, staffPass, "/staff/labs", "/staff/labs"},
		{"staff", staffEmail, staffPass, "/staff/labs/new", "/staff/labs/new"},
		{"staff", staffEmail, staffPass, "/staff/labs/1", ""},
	} {
		r := do(t, signedIn(t, base, tc.email, tc.pass), http.MethodGet, base+tc.path, nil)
		wantStatus(t, r, http.StatusOK, `href="/labs">Labs</a>`)
		marked := current.FindAllStringSubmatch(r.body, -1)
		switch {
		case tc.want == "" && len(marked) != 0:
			t.Errorf("%s %s: %d links marked current, want none", tc.who, tc.path, len(marked))
		case tc.want != "" && (len(marked) != 1 || marked[0][1] != tc.want):
			t.Errorf("%s %s: marked %v, want only %s", tc.who, tc.path, marked, tc.want)
		}
	}
}
