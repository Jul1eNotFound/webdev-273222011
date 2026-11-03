package handler

import (
	"errors"
	"net/http"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

const loginTitle = "Sign in · Labwiz"

func (h *handler) loginForm(w http.ResponseWriter, r *http.Request) {
	if _, err := h.Auth.CurrentUser(r.Context(), auth.TokenFrom(r)); err == nil {
		http.Redirect(w, r, "/labs", http.StatusSeeOther)
		return
	}
	h.page(w, http.StatusOK, "login", pageData{Title: loginTitle})
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		h.errorPage(w, r, http.StatusBadRequest, "The sign-in form could not be read. Please try again.")
		return
	}
	email := r.PostFormValue("email")
	s, err := h.Auth.Login(r.Context(), email, r.PostFormValue("password"))
	if errors.Is(err, service.ErrInvalidCredentials) {
		h.page(w, http.StatusUnauthorized, "login", pageData{Title: loginTitle, Error: err.Error(), Email: email})
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	auth.SetCookie(w, s.Token, service.SessionTTL, h.SecureCookies)
	http.Redirect(w, r, "/labs", http.StatusSeeOther)
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.Auth.Logout(r.Context(), auth.TokenFrom(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	auth.ClearCookie(w, h.SecureCookies)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
