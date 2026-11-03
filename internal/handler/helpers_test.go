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

var fixedNow = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

// newServer serves the app over the given stores, which may be the fake or the real one.
func newServer(t *testing.T, as service.AuthStore, ls service.LabStore) *httptest.Server {
	t.Helper()
	h, err := handler.New(handler.Deps{
		Auth: service.NewAuth(as, auth.Bcrypt{}, clock),
		Labs: service.NewLabs(ls, clock, time.UTC),
		Log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
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
