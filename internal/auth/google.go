package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

// googleEndpoint is Google's OAuth 2.0 endpoint, written out so the cloud packages behind
// golang.org/x/oauth2/google are not needed.
var googleEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// googleIssuers are the values Google puts in an id_token's "iss".
var googleIssuers = []string{"accounts.google.com", "https://accounts.google.com"}

// Google runs the server side of "Sign in with Google": the authorization-code flow with PKCE.
type Google struct {
	cfg oauth2.Config
	now func() time.Time
}

// NewGoogle is the client for one OAuth client id. redirectURL must be registered with Google exactly.
func NewGoogle(clientID, clientSecret, redirectURL string) *Google {
	return newGoogle(clientID, clientSecret, redirectURL, googleEndpoint, time.Now)
}

func newGoogle(clientID, clientSecret, redirectURL string, endpoint oauth2.Endpoint, now func() time.Time) *Google {
	return &Google{cfg: oauth2.Config{
		ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
		Endpoint: endpoint, Scopes: []string{"openid", "email", "profile"},
	}, now: now}
}

// AuthURL is where the browser goes to sign in with Google. The state comes back on the callback; the
// verifier never leaves the server, and Google only sees its hash (PKCE).
func (g *Google) AuthURL(state, verifier string) string {
	return g.cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

// Identity exchanges the callback's code and reads who signed in from the id_token.
//
// The id_token's signature is not checked: it comes straight from Google's token endpoint over TLS, in
// exchange for this client's secret and PKCE verifier, which OpenID Connect Core 3.1.3.7 accepts in place
// of a signature. Its issuer, audience and expiry are still checked.
func (g *Google) Identity(ctx context.Context, code, verifier string) (service.GoogleIdentity, error) {
	tok, err := g.cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return service.GoogleIdentity{}, fmt.Errorf("exchange code: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return service.GoogleIdentity{}, errors.New("token response has no id_token")
	}
	return g.claims(raw)
}

// idClaims are the id_token fields Labwiz reads. Google sends email_verified as a boolean; some
// providers send the string "true", which is accepted too.
type idClaims struct {
	Iss           string          `json:"iss"`
	Aud           json.RawMessage `json:"aud"`
	Exp           int64           `json:"exp"`
	Sub           string          `json:"sub"`
	Email         string          `json:"email"`
	EmailVerified any             `json:"email_verified"`
}

func (g *Google) claims(raw string) (service.GoogleIdentity, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return service.GoogleIdentity{}, errors.New("id_token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return service.GoogleIdentity{}, fmt.Errorf("id_token payload: %w", err)
	}
	var c idClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return service.GoogleIdentity{}, fmt.Errorf("id_token payload: %w", err)
	}
	switch {
	case !slices.Contains(googleIssuers, c.Iss):
		return service.GoogleIdentity{}, fmt.Errorf("id_token issuer %q", c.Iss)
	case !audienceIs(c.Aud, g.cfg.ClientID):
		return service.GoogleIdentity{}, errors.New("id_token is for another client")
	case !time.Unix(c.Exp, 0).After(g.now()):
		return service.GoogleIdentity{}, errors.New("id_token has expired")
	case c.Sub == "":
		return service.GoogleIdentity{}, errors.New("id_token has no subject")
	}
	verified := c.EmailVerified == true || c.EmailVerified == "true"
	return service.GoogleIdentity{Subject: c.Sub, Email: c.Email, EmailVerified: verified}, nil
}

// audienceIs accepts "aud" as one string or as a list, as JWTs allow.
func audienceIs(raw json.RawMessage, clientID string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == clientID
	}
	var many []string
	return json.Unmarshal(raw, &many) == nil && slices.Contains(many, clientID)
}

// GoogleCookieName holds the state and PKCE verifier between /auth/google and its callback. It ties the
// callback to the browser that started the sign-in, so nobody can sign someone into their own account.
const GoogleCookieName = "labwiz_google"

const googleCookiePath = "/auth/google"

// googleCookieLifetime is how long a Google sign-in may take.
const googleCookieLifetime = 10 * time.Minute

// NewGoogleState makes a fresh state and PKCE verifier for one sign-in.
func NewGoogleState() (state, verifier string) {
	return rand.Text(), oauth2.GenerateVerifier()
}

// SetGoogleState keeps the state and verifier in a short-lived cookie. SameSite=Lax still sends it on
// Google's redirect back, which is a top-level GET.
func SetGoogleState(w http.ResponseWriter, state, verifier string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: GoogleCookieName, Value: state + "." + verifier, Path: googleCookiePath,
		MaxAge: int(googleCookieLifetime / time.Second), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

// GoogleStateFrom reads the cookie SetGoogleState set; ok is false when there is none.
func GoogleStateFrom(r *http.Request) (state, verifier string, ok bool) {
	c, err := r.Cookie(GoogleCookieName)
	if err != nil {
		return "", "", false
	}
	state, verifier, ok = strings.Cut(c.Value, ".")
	return state, verifier, ok && state != "" && verifier != ""
}

// ClearGoogleState removes the cookie: each state is used once, whatever the callback's outcome.
func ClearGoogleState(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: GoogleCookieName, Value: "", Path: googleCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}
