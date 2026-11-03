// Package handler holds the Chi routes. Handlers read the request, call a service and turn the result into a status.
package handler

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/web"
)

type Deps struct {
	Auth          *service.Auth
	Labs          *service.Labs
	SecureCookies bool
	Log           *slog.Logger
}

type handler struct {
	Deps
	rn *renderer
}

func New(d Deps) (http.Handler, error) {
	rn, err := newRenderer()
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
			h.fail(w, r, service.ErrUnauthenticated)
		},
		Fail: h.fail,
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
	r.Get("/login", h.loginForm)
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)

	r.Group(func(r chi.Router) {
		r.Use(mw.RequirePage)
		r.Get("/labs", h.labsPage)
	})
	r.Group(func(r chi.Router) {
		r.Use(mw.RequireAPI)
		r.Post("/labs/{id}/book", h.bookStub)
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
