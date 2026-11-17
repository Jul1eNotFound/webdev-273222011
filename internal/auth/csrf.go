package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// minSecret is the shortest SESSION_SECRET the server accepts: 32 bytes, the size of the HMAC's own key
// strength.
const minSecret = 32

// CSRF makes and checks the token every form that changes something carries (Slice 3e). The token is the
// HMAC-SHA256 of the sign-in session's cookie token under SESSION_SECRET, so it differs per sign-in
// session (AC-86), needs no storage, and stops working when the session is signed out. A page on another
// site can read neither the HttpOnly cookie nor the page, so it cannot make the token.
type CSRF struct {
	secret []byte
}

// NewCSRF refuses a secret shorter than 32 bytes, so a placeholder such as "change-me" cannot ship.
func NewCSRF(secret string) (CSRF, error) {
	if len(secret) < minSecret {
		return CSRF{}, errors.New("SESSION_SECRET must be at least 32 characters; set it to a long random value")
	}
	return CSRF{secret: []byte(secret)}, nil
}

// Token returns the CSRF token for a sign-in session's cookie token.
func (c CSRF) Token(sessionToken string) string {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(sessionToken))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Valid reports whether got is the token for sessionToken. The comparison takes the same time however
// much of got is right.
func (c CSRF) Valid(sessionToken, got string) bool {
	if sessionToken == "" || got == "" || len(c.secret) == 0 {
		return false
	}
	return hmac.Equal([]byte(c.Token(sessionToken)), []byte(got))
}
