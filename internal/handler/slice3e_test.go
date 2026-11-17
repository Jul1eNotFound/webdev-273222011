package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

const msgExpired = "This page has expired. Please reload it and try again."

// changingPosts are the six kinds of form that change something, for Stella's or Sam's session in
// newTwoStaffApp. Each is sent with whatever csrf a test chooses.
var changingPosts = []struct {
	who, path string
	form      url.Values
}{
	{"student", "/sessions/1/book", url.Values{}},
	{"staff", "/staff/labs/new", url.Values{"title": {"Optics"}, "place_limit": {"3"}}},
	{"staff", "/staff/labs/1/sessions", url.Values{"starts": {"2026-10-24T14:00"}, "ends": {"2026-10-24T16:00"}}},
	{"staff", "/staff/sessions/3/cancel", url.Values{}},
	{"staff", "/staff/labs/3/delete", url.Values{}},
	{"staff", "/logout", url.Values{}},
}

// postWithout sends a form with no csrf field at all, which do would otherwise add.
func postWithout(t *testing.T, c *http.Client, u string, form url.Values) response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return response{res.StatusCode, res.Header, string(b)}
}

func TestAC85_ChangesWithoutTheRightTokenAre403AndChangeNothing(t *testing.T) {
	base, st := newTwoStaffApp(t)
	clients := map[string]*http.Client{
		"student": signedIn(t, base, studentEmail, studentPass),
		"staff":   signedIn(t, base, staffEmail, staffPass),
	}
	for _, p := range changingPosts {
		c := clients[p.who]
		for name, send := range map[string]func() response{
			"no field":    func() response { return postWithout(t, c, base+p.path, p.form) },
			"empty token": func() response { return do(t, c, http.MethodPost, base+p.path, cloneWith(p.form, "csrf", "")) },
			"wrong token": func() response {
				return do(t, c, http.MethodPost, base+p.path, cloneWith(p.form, "csrf", "not-the-token"))
			},
		} {
			r := send()
			if r.status != http.StatusForbidden || !strings.Contains(r.body, msgExpired) {
				t.Errorf("%s %s: got %d, want 403 with %q", p.path, name, r.status, msgExpired)
			}
		}
	}
	if st.LabCount() != 4 || st.SessionCount(1) != 1 || st.SessionCount(3) != 1 || len(st.Bookers[1]) != 2 {
		t.Fatalf("something changed: labs %d, sessions of lab 1: %d, of lab 3: %d, bookers of session 1: %d",
			st.LabCount(), st.SessionCount(1), st.SessionCount(3), len(st.Bookers[1]))
	}
	if v, _ := st.SessionByID(t.Context(), 3); v.CancelledAt != nil {
		t.Fatal("session 3 was cancelled")
	}
	// The refused sign-out left the session working.
	wantStatus(t, do(t, clients["staff"], http.MethodGet, base+"/labs", nil), http.StatusOK, "Stella Staff")
}

func TestAC85_ScriptBookingWithoutTheHeaderIs403AsJSON(t *testing.T) {
	base, st := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	req, err := http.NewRequest(http.MethodPost, base+"/sessions/1/book", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var reply struct{ Message string }
	if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
		t.Fatalf("reply is not JSON: %v", err)
	}
	if res.StatusCode != http.StatusForbidden || reply.Message != msgExpired {
		t.Fatalf("got %d %q, want 403 %q", res.StatusCode, reply.Message, msgExpired)
	}
	if len(st.Bookers[1]) != 0 {
		t.Fatal("the booking was made")
	}
}

func TestAC86_AnotherUsersTokenIsRefused(t *testing.T) {
	base, st := newTwoStaffApp(t)
	stella := signedIn(t, base, staffEmail, staffPass)
	omar := signedIn(t, base, omarEmail, omarPass)
	stellasToken := tokenFor(stella, base+"/labs")
	if stellasToken == "" || stellasToken == tokenFor(omar, base+"/labs") {
		t.Fatal("the two sessions should have different tokens")
	}
	r := do(t, omar, http.MethodPost, base+"/staff/labs/new", url.Values{"title": {"Optics"}, "place_limit": {"3"}, "csrf": {stellasToken}})
	wantStatus(t, r, http.StatusForbidden, msgExpired)
	if st.LabCount() != 4 {
		t.Fatal("a lab was added with someone else's token")
	}
	// And Stella's own token still works for her.
	wantRedirect(t, do(t, stella, http.MethodPost, base+"/staff/labs/new", url.Values{"title": {"Optics"}, "place_limit": {"3"}}),
		http.StatusSeeOther, "/staff/labs/5?created=1")
}

func TestAC87_EveryChangingFormCarriesTheSessionsToken(t *testing.T) {
	base, _ := newStaffApp(t)
	for who, c := range map[string]*http.Client{
		"student": signedIn(t, base, studentEmail, studentPass),
		"staff":   signedIn(t, base, staffEmail, staffPass),
	} {
		token := tokenFor(c, base+"/labs")
		paths := []string{"/labs", "/labs/1", "/labs?date=2026-10-23"}
		if who == "staff" {
			paths = append(paths, "/staff/labs", "/staff/labs/new", "/staff/labs/1", "/staff/sessions/3/cancel", "/staff/labs/3/delete")
		}
		withToken := regexp.MustCompile(`(?s)<form method="post"[^>]*>\s*<input type="hidden" name="csrf" value="` + regexp.QuoteMeta(token) + `">`)
		for _, path := range paths {
			r := do(t, c, http.MethodGet, base+path, nil)
			forms := strings.Count(r.body, `<form method="post"`)
			if forms == 0 || len(withToken.FindAllString(r.body, -1)) != forms {
				t.Errorf("%s %s: %d post forms, %d carry the session's token", who, path, forms, len(withToken.FindAllString(r.body, -1)))
			}
			if !strings.Contains(r.body, `<meta name="csrf-token" content="`+token+`">`) {
				t.Errorf("%s %s: no csrf-token meta tag for the booking script", who, path)
			}
		}
	}
	login := do(t, newClient(t), http.MethodGet, base+"/login", nil)
	if strings.Contains(login.body, `name="csrf"`) || strings.Contains(login.body, "csrf-token") {
		t.Error("the sign-in page carries a token, but there is no session yet")
	}
}

func TestAC87_BookingScriptSendsTheTokenInAHeader(t *testing.T) {
	base, _ := newFakeApp(t)
	js := do(t, signedIn(t, base, studentEmail, studentPass), http.MethodGet, base+"/static/booking.js", nil)
	for _, want := range []string{`meta[name="csrf-token"]`, `'X-CSRF-Token': token`} {
		if !strings.Contains(js.body, want) {
			t.Errorf("booking.js lacks %q", want)
		}
	}
}

func TestAC19_BookingWithoutASessionIsStill401BeforeAnyTokenCheck(t *testing.T) {
	base, _ := newFakeApp(t)
	r := postWithout(t, newClient(t), base+"/sessions/1/book", url.Values{})
	if r.status != http.StatusUnauthorized || strings.Contains(r.body, msgExpired) {
		t.Fatalf("got %d, want 401 without the token message\n%s", r.status, r.body)
	}
}
