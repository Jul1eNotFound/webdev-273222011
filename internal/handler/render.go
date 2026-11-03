package handler

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/web"
)

const msgInternal = "Something went wrong on our side. Please try again."

// pageData is what every template receives. Title must differ per page (AC-42).
type pageData struct {
	Title string
	User  *service.Caller
	Error string
	Email string
	Labs  []store.Lab
}

type renderer struct {
	pages map[string]*template.Template
}

func newRenderer() (*renderer, error) {
	rn := &renderer{pages: map[string]*template.Template{}}
	for _, name := range []string{"login", "labs", "error"} {
		t, err := template.ParseFS(web.FS, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		rn.pages[name] = t
	}
	return rn, nil
}

// page renders into a buffer first, so a template error never sends half a page.
func (h *handler) page(w http.ResponseWriter, status int, name string, data pageData) {
	var buf bytes.Buffer
	if err := h.rn.pages[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		h.Log.Error("render page", "page", name, "err", err)
		http.Error(w, msgInternal, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// errorPage shows one readable sentence (AC-43).
func (h *handler) errorPage(w http.ResponseWriter, r *http.Request, status int, msg string) {
	data := pageData{Title: "Error · Labwiz", Error: msg}
	if c, ok := auth.CallerFrom(r.Context()); ok {
		data.User = &c
	}
	h.page(w, status, "error", data)
}

// fail maps a service error to its status. Anything else is logged and shown as a generic 500.
func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var se *service.Error
	if errors.As(err, &se) {
		h.errorPage(w, r, statusFor(se.Kind), se.Message)
		return
	}
	h.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	h.errorPage(w, r, http.StatusInternalServerError, msgInternal)
}

func statusFor(k service.Kind) int {
	switch k {
	case service.KindValidation:
		return http.StatusBadRequest
	case service.KindUnauthenticated:
		return http.StatusUnauthorized
	case service.KindForbidden:
		return http.StatusForbidden
	case service.KindNotFound:
		return http.StatusNotFound
	case service.KindConflict:
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}
