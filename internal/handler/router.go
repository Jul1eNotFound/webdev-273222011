// Package handler holds the Chi routes. Handlers read the request, call a service and turn the result into a status.
package handler

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/mcpserver"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/web"
)

type Deps struct {
	Auth          *service.Auth
	Labs          *service.Labs
	SecureCookies bool
	// CSRF makes and checks the token on every form that changes something (Slice 3e).
	CSRF auth.CSRF
	Log  *slog.Logger
}

type handler struct {
	Deps
	rn *renderer
}

func New(d Deps) (http.Handler, error) {
	rn, err := newRenderer(d.Labs.Location())
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(web.FS, "static")
	if err != nil {
		return nil, err
	}
	h := &handler{Deps: d, rn: rn}
	mw := auth.Middleware{
		Sessions: d.Auth,
		Unauthorized: func(w http.ResponseWriter, r *http.Request) {
			if wantsJSON(r) {
				h.failJSON(w, r, service.ErrUnauthenticated)
				return
			}
			h.fail(w, r, service.ErrUnauthenticated)
		},
		// A refused token or a failed lookup: JSON for the booking script, the error page otherwise.
		Fail: func(w http.ResponseWriter, r *http.Request, err error) {
			if wantsJSON(r) {
				h.failJSON(w, r, err)
				return
			}
			h.fail(w, r, err)
		},
		CSRF: d.CSRF,
	}

	r := chi.NewRouter()
	r.Use(h.recoverer)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		h.errorPage(w, r, http.StatusNotFound, "This page does not exist.")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		h.errorPage(w, r, http.StatusMethodNotAllowed, "This action is not allowed on this page.")
	})

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(static)))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/labs", http.StatusFound)
	})
	// The MCP tools (Slice 5) authenticate each call by its bearer token, never by the session cookie, so
	// they sit outside the cookie and CSRF groups.
	r.Handle("/mcp", mcpserver.New(h.Labs, h.Auth, h.Log))
	r.Get("/login", h.loginForm)
	r.Post("/login", h.login)

	r.Group(func(r chi.Router) {
		r.Use(mw.RequirePage)
		// Signing out changes something too, so it needs the session and its token (AC-85).
		r.Post("/logout", h.logout)
		r.Get("/labs", h.labsPage)
		r.Get("/bookings", h.bookingsPage)
		r.Post("/sessions/{id}/cancel-booking", h.cancelBooking)
		r.Get("/labs/{id}", h.labPage)
		r.Get("/staff/labs", h.myLabsPage)
		r.Get("/staff/labs/new", h.newLabForm)
		r.Post("/staff/labs/new", h.addLab)
		r.Get("/staff/labs/{id}", h.staffLabPage)
		r.Post("/staff/labs/{id}/sessions", h.addSessions)
		r.Get("/staff/labs/{id}/delete", h.deletePage)
		r.Post("/staff/labs/{id}/delete", h.deleteLab)
		r.Get("/staff/sessions/{id}/cancel", h.cancelPage)
		r.Post("/staff/sessions/{id}/cancel", h.cancelSession)
	})
	r.Group(func(r chi.Router) {
		r.Use(mw.RequireAPI)
		r.Post("/sessions/{id}/book", h.book)
	})
	return r, nil
}

// recoverer turns a panic into the readable 500 page instead of a stack trace (AC-43).
func (h *handler) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				h.Log.Error("panic", "method", r.Method, "path", r.URL.Path, "panic", v)
				h.errorPage(w, r, http.StatusInternalServerError, msgInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
