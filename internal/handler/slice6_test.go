package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/handler"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
)

const (
	msgGoogleCancelled  = "Google sign-in was cancelled."
	msgGoogleState      = "Google sign-in could not be completed. Please try again."
	msgGoogleFailed     = "Google sign-in is unavailable right now. Please try again."
	msgGoogleNoAccount  = "No Labwiz account uses this Google address. Ask staff to add you."
	msgGoogleUnverified = "Google has not verified this email address, so it cannot be used to sign in."
	googleButton        = `href="/auth/google">Sign in with Google</a>`
)

// fakeGoogle stands in for Google: its auth URL carries the state back, and Identity returns whoever the
// test says signed in, after recording what it was given.
type fakeGoogle struct {
	mu       sync.Mutex
	id       service.GoogleIdentity
	err      error
	calls    int
	code     string
	verifier string
}

func (g *fakeGoogle) AuthURL(state, verifier string) string {
	return "https://accounts.google.test/auth?" + url.Values{"state": {state}, "challenge-of": {verifier}}.Encode()
}

func (g *fakeGoogle) Identity(_ context.Context, code, verifier string) (service.GoogleIdentity, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls, g.code, g.verifier = g.calls+1, code, verifier
	return g.id, g.err
}

func withGoogle(g handler.GoogleProvider) func(*handler.Deps) {
	return func(d *handler.Deps) { d.Google = g }
}

// newGoogleApp is newFakeApp with a fake Google; Sam (student@labwiz.test) has a local account.
func newGoogleApp(t *testing.T) (string, *servicetest.Store, *fakeGoogle) {
	t.Helper()
	_, st := newFakeApp(t)
	g := &fakeGoogle{id: service.GoogleIdentity{Subject: "g-sam", Email: studentEmail, EmailVerified: true}}
	return newServer(t, st, st, withGoogle(g)).URL, st, g
}

// startGoogle follows the "Sign in with Google" button and returns the state Google would send back.
func startGoogle(t *testing.T, c *http.Client, base string) string {
	t.Helper()
	r := do(t, c, http.MethodGet, base+"/auth/google", nil)
	if r.status != http.StatusSeeOther {
		t.Fatalf("GET /auth/google = %d", r.status)
	}
	to, err := url.Parse(r.header.Get("Location"))
	if err != nil || to.Host != "accounts.google.test" || to.Query().Get("state") == "" {
		t.Fatalf("redirect = %q", r.header.Get("Location"))
	}
	return to.Query().Get("state")
}

// googleCallback is Google sending the browser back with these query parameters.
func googleCallback(t *testing.T, c *http.Client, base string, q url.Values) response {
	t.Helper()
	return do(t, c, http.MethodGet, base+"/auth/google/callback?"+q.Encode(), nil)
}

// signInWithGoogle runs the whole round trip as a browser would.
func signInWithGoogle(t *testing.T, c *http.Client, base string) response {
	t.Helper()
	state := startGoogle(t, c, base)
	return googleCallback(t, c, base, url.Values{"state": {state}, "code": {"the-code"}})
}

// wantNoSession fails if c is signed in: /labs must send it to /login.
func wantNoSession(t *testing.T, c *http.Client, base string) {
	t.Helper()
	wantRedirect(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusSeeOther, "/login")
}

func setsSessionCookie(r response) bool {
	for _, v := range r.header.Values("Set-Cookie") {
		if strings.HasPrefix(v, auth.CookieName+"=") && !strings.HasPrefix(v, auth.CookieName+"=;") {
			return true
		}
	}
	return false
}

func TestAC20_GoogleSignInLinksTheAccountAndLandsOnLabs(t *testing.T) {
	base, st, g := newGoogleApp(t)
	c := newClient(t)
	wantStatus(t, do(t, c, http.MethodGet, base+"/login", nil), http.StatusOK, googleButton)
	r := signInWithGoogle(t, c, base)
	wantRedirect(t, r, http.StatusSeeOther, "/labs")
	if !setsSessionCookie(r) {
		t.Fatal("no session cookie")
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "Sam Student")
	if got := st.User(studentEmail).GoogleID; got != "g-sam" {
		t.Fatalf("google id = %q, want g-sam", got)
	}
	// The code and this browser's own verifier reached the exchange (PKCE).
	if g.code != "the-code" || len(g.verifier) < 43 {
		t.Errorf("exchange got code %q, verifier %q", g.code, g.verifier)
	}
}

func TestAC52_UnverifiedGoogleEmailIsRefusedWithASentence(t *testing.T) {
	base, st, g := newGoogleApp(t)
	g.id.EmailVerified = false
	c := newClient(t)
	r := signInWithGoogle(t, c, base)
	wantRedirect(t, r, http.StatusSeeOther, "/login?google=unverified")
	if setsSessionCookie(r) {
		t.Error("a session cookie was set")
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/login?google=unverified", nil), http.StatusOK, msgGoogleUnverified)
	wantNoSession(t, c, base)
	if st.User(studentEmail).GoogleID != "" || len(st.Sessions()) != 0 {
		t.Fatal("linked or signed in")
	}
}

func TestAC53_LinkedGoogleAccountWithAChangedEmailSignsInTheSameUser(t *testing.T) {
	base, st, g := newGoogleApp(t)
	st.SetGoogleID(studentEmail, "g-sam")
	g.id = service.GoogleIdentity{Subject: "g-sam", Email: "sam.new@gmail.com", EmailVerified: true}
	c := newClient(t)
	wantRedirect(t, signInWithGoogle(t, c, base), http.StatusSeeOther, "/labs")
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "Sam Student")
	if u := st.User(studentEmail); u.Email != studentEmail || u.GoogleID != "g-sam" {
		t.Fatalf("user = %+v", u)
	}
}

func TestAC54_CancelledGoogleSignInLandsOnLoginWithoutASession(t *testing.T) {
	base, st, g := newGoogleApp(t)
	c := newClient(t)
	state := startGoogle(t, c, base)
	r := googleCallback(t, c, base, url.Values{"error": {"access_denied"}, "state": {state}})
	wantRedirect(t, r, http.StatusSeeOther, "/login?google=cancelled")
	if setsSessionCookie(r) || g.calls != 0 || len(st.Sessions()) != 0 {
		t.Fatalf("session cookie %v, exchanges %d, sessions %d", setsSessionCookie(r), g.calls, len(st.Sessions()))
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/login?google=cancelled", nil), http.StatusOK, msgGoogleCancelled)
	wantNoSession(t, c, base)
}

func TestAC54_WrongOrMissingStateIsRefused(t *testing.T) {
	base, st, g := newGoogleApp(t)
	c := newClient(t)
	state := startGoogle(t, c, base)
	for name, q := range map[string]url.Values{
		"wrong state": {"state": {state + "x"}, "code": {"the-code"}},
		"no state":    {"code": {"the-code"}},
	} {
		startGoogle(t, c, base) // a fresh cookie each time: the callback clears it
		r := googleCallback(t, c, base, q)
		wantRedirect(t, r, http.StatusSeeOther, "/login?google=state")
		if setsSessionCookie(r) {
			t.Errorf("%s: a session cookie was set", name)
		}
	}
	// The right state from another browser, which has no state cookie: someone else's sign-in.
	wantRedirect(t, googleCallback(t, newClient(t), base, url.Values{"state": {state}, "code": {"the-code"}}),
		http.StatusSeeOther, "/login?google=state")
	// A state is used once: the callback cleared the cookie, so replaying the right one fails.
	state = startGoogle(t, c, base)
	googleCallback(t, c, base, url.Values{"state": {state + "x"}})
	wantRedirect(t, googleCallback(t, c, base, url.Values{"state": {state}, "code": {"the-code"}}),
		http.StatusSeeOther, "/login?google=state")
	if g.calls != 0 || len(st.Sessions()) != 0 {
		t.Fatalf("exchanges %d, sessions %d; want none", g.calls, len(st.Sessions()))
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/login?google=state", nil), http.StatusOK, msgGoogleState)
	wantNoSession(t, c, base)
}

func TestGoogleSignInWithoutAnAccountOrAFailedExchange(t *testing.T) {
	base, st, g := newGoogleApp(t)
	g.id = service.GoogleIdentity{Subject: "g-x", Email: "nobody@gmail.com", EmailVerified: true}
	c := newClient(t)
	wantRedirect(t, signInWithGoogle(t, c, base), http.StatusSeeOther, "/login?google=noaccount")
	wantStatus(t, do(t, c, http.MethodGet, base+"/login?google=noaccount", nil), http.StatusOK, msgGoogleNoAccount)

	g.err = errors.New("invalid_grant")
	wantRedirect(t, signInWithGoogle(t, c, base), http.StatusSeeOther, "/login?google=failed")
	wantStatus(t, do(t, c, http.MethodGet, base+"/login?google=failed", nil), http.StatusOK, msgGoogleFailed)

	g.err = nil
	st.SetGoogleID(studentEmail, "g-other")
	g.id = service.GoogleIdentity{Subject: "g-sam", Email: studentEmail, EmailVerified: true}
	wantRedirect(t, signInWithGoogle(t, c, base), http.StatusSeeOther, "/login?google=otheraccount")

	st.ErrUserByGoogleID = errors.New("connection refused")
	wantRedirect(t, signInWithGoogle(t, c, base), http.StatusSeeOther, "/login?google=failed")
	if len(st.Sessions()) != 0 {
		t.Fatal("a session was created")
	}
}

func TestGoogleStateCookieIsShortLivedAndClearedOnTheCallback_AC54(t *testing.T) {
	base, _, _ := newGoogleApp(t)
	c := newClient(t)
	r := do(t, c, http.MethodGet, base+"/auth/google", nil)
	set := r.header.Get("Set-Cookie")
	for _, want := range []string{auth.GoogleCookieName + "=", "Path=/auth/google", "Max-Age=600", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(set, want) {
			t.Errorf("Set-Cookie %q lacks %q", set, want)
		}
	}
	cleared := googleCallback(t, c, base, url.Values{"error": {"access_denied"}}).header.Get("Set-Cookie")
	if !strings.Contains(cleared, auth.GoogleCookieName+"=;") || !strings.Contains(cleared, "Max-Age=0") {
		t.Errorf("callback Set-Cookie = %q, want the state cookie cleared", cleared)
	}
}

func TestGoogleSignInIsHiddenWithoutAnOAuthClient(t *testing.T) {
	base, _ := newFakeApp(t)
	c := newClient(t)
	if strings.Contains(do(t, c, http.MethodGet, base+"/login", nil).body, "Sign in with Google") {
		t.Error("the button is shown without Google configured")
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/auth/google", nil), http.StatusNotFound, "This page does not exist.")
	wantStatus(t, do(t, c, http.MethodGet, base+"/auth/google/callback?state=x", nil), http.StatusNotFound, "This page does not exist.")
	// A wrong password keeps the page as it was: no button either.
	if strings.Contains(signIn(t, c, base, studentEmail, "wrong").body, "Sign in with Google") {
		t.Error("the button appears after a wrong password")
	}
}

func TestLoginPageShowsOnlyKnownGoogleMessages(t *testing.T) {
	base, _, _ := newGoogleApp(t)
	c := newClient(t)
	r := do(t, c, http.MethodGet, base+"/login?google="+url.QueryEscape("<b>hacked</b>"), nil)
	if strings.Contains(r.body, "hacked") || strings.Contains(r.body, `role="alert"`) {
		t.Error("an unknown code reached the page")
	}
	// The button stays after a wrong password.
	wantStatus(t, signIn(t, c, base, studentEmail, "wrong"), http.StatusUnauthorized, googleButton)
}
