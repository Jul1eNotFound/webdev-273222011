package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

// GoogleProvider is "Sign in with Google" (auth.Google); tests pass a fake. Nil when the app has no
// Google OAuth client: the button is hidden and the routes are 404.
type GoogleProvider interface {
	AuthURL(state, verifier string) string
	Identity(ctx context.Context, code, verifier string) (service.GoogleIdentity, error)
}

// googleMessages are the sign-in page's sentences for /login?google=<code>. Only these fixed codes
// are shown, so no text from the URL reaches the page (AC-54).
var googleMessages = map[string]string{
	"cancelled":    "Google sign-in was cancelled.",
	"state":        "Google sign-in could not be completed. Please try again.",
	"failed":       "Google sign-in is unavailable right now. Please try again.",
	"noaccount":    service.ErrGoogleNoAccount.Message,
	"unverified":   service.ErrGoogleUnverified.Message,
	"otheraccount": service.ErrGoogleOtherAccount.Message,
}

// googleStart sends the browser to Google with a fresh state and PKCE verifier, kept in a cookie.
func (h *handler) googleStart(w http.ResponseWriter, r *http.Request) {
	if h.Google == nil {
		h.errorPage(w, r, http.StatusNotFound, "This page does not exist.")
		return
	}
	state, verifier := auth.NewGoogleState()
	auth.SetGoogleState(w, state, verifier, h.SecureCookies)
	http.Redirect(w, r, h.Google.AuthURL(state, verifier), http.StatusSeeOther)
}

// googleCallback is where Google sends the browser back. Only a matching state from this browser's
// cookie, a successful exchange and the service's linking rules (AC-20, AC-52, AC-53) start a session;
// anything else lands on /login with one sentence and no session (AC-54).
func (h *handler) googleCallback(w http.ResponseWriter, r *http.Request) {
	if h.Google == nil {
		h.errorPage(w, r, http.StatusNotFound, "This page does not exist.")
		return
	}
	state, verifier, ok := auth.GoogleStateFrom(r)
	auth.ClearGoogleState(w, h.SecureCookies) // each state is used once
	q := r.URL.Query()
	if q.Get("error") != "" {
		h.googleFailed(w, r, "cancelled")
		return
	}
	if !ok || subtle.ConstantTimeCompare([]byte(state), []byte(q.Get("state"))) != 1 {
		h.googleFailed(w, r, "state")
		return
	}
	id, err := h.Google.Identity(r.Context(), q.Get("code"), verifier)
	if err != nil {
		h.Log.Error("google sign-in: identity", "err", err)
		h.googleFailed(w, r, "failed")
		return
	}
	s, err := h.Auth.GoogleSignIn(r.Context(), id)
	switch {
	case err == nil:
		auth.SetCookie(w, s.Token, service.SessionTTL, h.SecureCookies)
		http.Redirect(w, r, "/labs", http.StatusSeeOther)
	case errors.Is(err, service.ErrGoogleNoAccount):
		h.googleFailed(w, r, "noaccount")
	case errors.Is(err, service.ErrGoogleUnverified):
		h.googleFailed(w, r, "unverified")
	case errors.Is(err, service.ErrGoogleOtherAccount):
		h.googleFailed(w, r, "otheraccount")
	default:
		h.Log.Error("google sign-in", "err", err)
		h.googleFailed(w, r, "failed")
	}
}

func (h *handler) googleFailed(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?google="+code, http.StatusSeeOther)
}
