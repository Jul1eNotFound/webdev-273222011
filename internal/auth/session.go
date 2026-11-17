package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

const CookieName = "labwiz_session"

// SetCookie sets a relative lifetime (Max-Age), not Expires: the browser checks it against its own clock,
// which need not match the server's. The sessions table still enforces the real expiry.
func SetCookie(w http.ResponseWriter, token string, lifetime time.Duration, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: token, Path: "/", MaxAge: int(lifetime / time.Second),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

func ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

// TokenFrom returns the session token from the request, or "".
func TokenFrom(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

type callerKey struct{}

type csrfKey struct{}

// CSRFFrom returns the CSRF token RequirePage or RequireAPI put in the context, for the forms of the page.
func CSRFFrom(ctx context.Context) string {
	t, _ := ctx.Value(csrfKey{}).(string)
	return t
}

// ErrBadToken refuses a change whose CSRF token is missing or wrong (AC-85, AC-86).
var ErrBadToken = &service.Error{Kind: service.KindForbidden, Message: "This page has expired. Please reload it and try again."}

// ErrUnreadableForm is a change whose form cannot be read, so its token cannot be checked.
var ErrUnreadableForm = &service.Error{Kind: service.KindValidation, Message: "The form could not be read. Please try again."}

// maxForm is the largest form body read, before any handler sees it.
const maxForm = 64 << 10

// CallerFrom returns the caller that RequirePage or RequireAPI put in the context.
func CallerFrom(ctx context.Context) (service.Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(service.Caller)
	return c, ok
}

type Sessions interface {
	CurrentUser(ctx context.Context, token string) (service.Caller, error)
}

// Middleware loads the caller from the session cookie. It does not check roles: the service does.
type Middleware struct {
	Sessions Sessions
	// Unauthorized is RequireAPI's response when nobody is signed in.
	Unauthorized http.HandlerFunc
	// Fail handles any other lookup error, and a refused CSRF token.
	Fail func(http.ResponseWriter, *http.Request, error)
	// CSRF checks the token of every request that changes something (Slice 3e).
	CSRF CSRF
}

// RequirePage redirects visitors who are not signed in to /login (AC-9).
func (m Middleware) RequirePage(next http.Handler) http.Handler {
	return m.require(next, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
}

// RequireAPI answers 401 to visitors who are not signed in, before the handler runs (AC-19).
func (m Middleware) RequireAPI(next http.Handler) http.Handler {
	return m.require(next, m.Unauthorized)
}

func (m Middleware) require(next http.Handler, unauthorized http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Back after sign-out must not show a cached protected page (AC-21).
		w.Header().Set("Cache-Control", "no-store")
		c, err := m.Sessions.CurrentUser(r.Context(), TokenFrom(r))
		if errors.Is(err, service.ErrUnauthenticated) {
			unauthorized(w, r)
			return
		}
		if err != nil {
			m.Fail(w, r, err)
			return
		}
		// Every request that may change something carries the session's CSRF token: in the
		// X-CSRF-Token header (the booking script) or the csrf form field. Checked here, after the
		// session, so a visitor who is not signed in still gets 401 or the redirect (AC-19), and before
		// any handler, so no route can forget it (AC-85).
		token := TokenFrom(r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			got := r.Header.Get("X-CSRF-Token")
			if got == "" {
				r.Body = http.MaxBytesReader(w, r.Body, maxForm)
				if err := r.ParseForm(); err != nil {
					m.Fail(w, r, ErrUnreadableForm)
					return
				}
				got = r.PostFormValue("csrf")
			}
			if !m.CSRF.Valid(token, got) {
				m.Fail(w, r, ErrBadToken)
				return
			}
		}
		ctx := context.WithValue(r.Context(), callerKey{}, c)
		ctx = context.WithValue(ctx, csrfKey{}, m.CSRF.Token(token))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
