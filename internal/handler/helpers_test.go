package handler_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/handler"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

// fixedNow is 08:30, so the seeded 09:00 and 13:00 sessions can still be booked (AC-84) while the
// 08:00–20:00 one has started (AC-83).
var fixedNow = time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

// testCSRF signs the test server's CSRF tokens, so tests can make the token a page would carry.
var testCSRF = func() auth.CSRF {
	c, err := auth.NewCSRF("test-session-secret-at-least-32-characters")
	if err != nil {
		panic(err)
	}
	return c
}()

// tokenFor is the CSRF token the server expects from client c, or "" when c is not signed in.
func tokenFor(c *http.Client, u string) string {
	if c.Jar == nil {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	for _, ck := range c.Jar.Cookies(parsed) {
		if ck.Name == auth.CookieName {
			return testCSRF.Token(ck.Value)
		}
	}
	return ""
}

// newServer serves the app over the given stores, which may be the fake or the real one. opts may change
// the dependencies, for example to add a fake Google.
func newServer(t *testing.T, as service.AuthStore, ls service.LabStore, opts ...func(*handler.Deps)) *httptest.Server {
	t.Helper()
	deps := handler.Deps{
		Auth: service.NewAuth(as, auth.Bcrypt{}, clock),
		Labs: service.NewLabs(ls, clock, time.UTC),
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		CSRF: testCSRF,
	}
	for _, o := range opts {
		o(&deps)
	}
	h, err := handler.New(deps)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// newClient keeps cookies, like a browser, but does not follow redirects so tests can see them.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

type response struct {
	status int
	header http.Header
	body   string
}

func do(t *testing.T, c *http.Client, method, u string, form url.Values) response {
	t.Helper()
	var body io.Reader
	if form != nil {
		// A POST from a signed-in client carries its session's token, as the page's form would, unless
		// the test sets csrf itself to send a missing or wrong one (AC-85, AC-86).
		if method == http.MethodPost && !form.Has("csrf") {
			if token := tokenFor(c, u); token != "" {
				form = cloneWith(form, "csrf", token)
			}
		}
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{res.StatusCode, res.Header, string(b)}
}

func cloneWith(v url.Values, key, value string) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	out.Set(key, value)
	return out
}

func signIn(t *testing.T, c *http.Client, base, email, password string) response {
	t.Helper()
	return do(t, c, http.MethodPost, base+"/login", url.Values{"email": {email}, "password": {password}})
}

func wantRedirect(t *testing.T, r response, status int, location string) {
	t.Helper()
	if r.status != status || r.header.Get("Location") != location {
		t.Fatalf("got %d Location=%q, want %d %q\n%s", r.status, r.header.Get("Location"), status, location, r.body)
	}
}
