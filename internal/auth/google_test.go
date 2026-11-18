package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

var googleNow = time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)

// idToken builds an unsigned JWT with the given claims, as the fake token endpoint returns it.
func idToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	b, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"RS256"}`)) + "." + enc(b) + ".c2ln"
}

func goodClaims() map[string]any {
	return map[string]any{
		"iss": "https://accounts.google.com", "aud": "client-1", "exp": googleNow.Add(time.Hour).Unix(),
		"sub": "g-sam", "email": "sam@gmail.com", "email_verified": true,
	}
}

// fakeTokenEndpoint answers the code exchange with idTok, after checking what Labwiz sent.
func fakeTokenEndpoint(t *testing.T, idTok string) *Google {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		for k, want := range map[string]string{"code": "the-code", "code_verifier": "the-verifier", "client_id": "client-1", "client_secret": "secret-1", "grant_type": "authorization_code"} {
			if got := r.PostForm.Get(k); got != want {
				t.Errorf("token request %s = %q, want %q", k, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600}
		if idTok != "" {
			body["id_token"] = idTok
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return newGoogle("client-1", "secret-1", "http://localhost:3000/auth/google/callback",
		oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: srv.URL, AuthStyle: oauth2.AuthStyleInParams},
		func() time.Time { return googleNow })
}

func TestGoogleAuthURLAsksForOpenIDWithPKCE_AC54(t *testing.T) {
	g := NewGoogle("client-1", "secret-1", "http://localhost:3000/auth/google/callback")
	u, err := url.Parse(g.AuthURL("the-state", "the-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if u.Host != "accounts.google.com" || q.Get("client_id") != "client-1" || q.Get("state") != "the-state" ||
		q.Get("response_type") != "code" || q.Get("scope") != "openid email profile" ||
		q.Get("redirect_uri") != "http://localhost:3000/auth/google/callback" ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != oauth2.S256ChallengeFromVerifier("the-verifier") {
		t.Fatalf("auth URL = %s", u)
	}
	if strings.Contains(u.String(), "the-verifier") || strings.Contains(u.String(), "secret-1") {
		t.Error("the verifier or the secret is in the URL")
	}
}

func TestGoogleIdentityReadsTheIDToken_AC20(t *testing.T) {
	id, err := fakeTokenEndpoint(t, idToken(t, goodClaims())).Identity(context.Background(), "the-code", "the-verifier")
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "g-sam" || id.Email != "sam@gmail.com" || !id.EmailVerified {
		t.Fatalf("identity = %+v", id)
	}
	// "aud" as a list and email_verified as a string are accepted too.
	c := goodClaims()
	c["aud"], c["email_verified"], c["iss"] = []string{"other", "client-1"}, "true", "accounts.google.com"
	if id, err := fakeTokenEndpoint(t, idToken(t, c)).Identity(context.Background(), "the-code", "the-verifier"); err != nil || !id.EmailVerified {
		t.Fatalf("identity = %+v, %v", id, err)
	}
}

func TestGoogleIdentityReadsAnUnverifiedEmail_AC52(t *testing.T) {
	for _, v := range []any{false, "false", nil} {
		c := goodClaims()
		c["email_verified"] = v
		id, err := fakeTokenEndpoint(t, idToken(t, c)).Identity(context.Background(), "the-code", "the-verifier")
		if err != nil || id.EmailVerified {
			t.Errorf("email_verified %v: identity %+v, err %v; want unverified", v, id, err)
		}
	}
}

func TestGoogleIdentityRefusesABadIDToken(t *testing.T) {
	with := func(k string, v any) string {
		c := goodClaims()
		c[k] = v
		return idToken(t, c)
	}
	for name, tok := range map[string]string{
		"no id_token":    "",
		"not a JWT":      "abc.def",
		"bad base64":     "a.%%%.c",
		"bad JSON":       "a." + base64.RawURLEncoding.EncodeToString([]byte("{")) + ".c",
		"other issuer":   with("iss", "https://evil.example"),
		"other audience": with("aud", "client-2"),
		"bad audience":   with("aud", 42),
		"expired":        with("exp", googleNow.Add(-time.Second).Unix()),
		"no subject":     with("sub", ""),
	} {
		if _, err := fakeTokenEndpoint(t, tok).Identity(context.Background(), "the-code", "the-verifier"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestGoogleIdentityReportsAFailedExchange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	g := newGoogle("client-1", "secret-1", "http://x/cb", oauth2.Endpoint{TokenURL: srv.URL, AuthStyle: oauth2.AuthStyleInParams}, time.Now)
	if _, err := g.Identity(context.Background(), "bad", "v"); err == nil {
		t.Fatal("a refused exchange gave an identity")
	}
}

func TestGoogleStateCookieRoundTrip_AC54(t *testing.T) {
	state, verifier := NewGoogleState()
	if state == "" || len(verifier) < 43 || strings.Contains(state, ".") {
		t.Fatalf("state %q, verifier %q", state, verifier)
	}
	rec := httptest.NewRecorder()
	SetGoogleState(rec, state, verifier, true)
	set := rec.Result().Cookies()[0]
	if set.Path != "/auth/google" || set.MaxAge != 600 || !set.HttpOnly || !set.Secure || set.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie = %+v", set)
	}
	r := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
	r.AddCookie(set)
	if s, v, ok := GoogleStateFrom(r); !ok || s != state || v != verifier {
		t.Fatalf("read back %q %q %v", s, v, ok)
	}
	for _, value := range []string{"", "nodot", ".v", "s."} {
		r := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
		r.AddCookie(&http.Cookie{Name: GoogleCookieName, Value: value})
		if _, _, ok := GoogleStateFrom(r); ok {
			t.Errorf("cookie %q accepted", value)
		}
	}
	if _, _, ok := GoogleStateFrom(httptest.NewRequest(http.MethodGet, "/", nil)); ok {
		t.Error("no cookie accepted")
	}
	rec = httptest.NewRecorder()
	ClearGoogleState(rec, false)
	if c := rec.Result().Cookies()[0]; c.MaxAge >= 0 || c.Path != "/auth/google" {
		t.Errorf("clear = %+v", c)
	}
}
