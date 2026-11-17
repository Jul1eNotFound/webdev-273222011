package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

const newLabTitle = "Add a lab · Labwiz"

func (h *handler) newLabForm(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	if err := service.RequireStaff(c); err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, http.StatusOK, "lab_new", pageData{Title: newLabTitle, User: &c})
}

func (h *handler) addLab(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	// A student gets 403 before their form is even read (AC-15).
	if err := service.RequireStaff(c); err != nil {
		h.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		h.errorPage(w, r, http.StatusBadRequest, "The lab form could not be read. Please try again.")
		return
	}
	in := service.NewLab{
		Title:       r.PostFormValue("title"),
		Description: r.PostFormValue("description"),
		Starts:      r.PostFormValue("starts"),
		Ends:        r.PostFormValue("ends"),
		PlaceLimit:  r.PostFormValue("place_limit"),
	}
	lab, err := h.Labs.Add(r.Context(), c, in)
	var se *service.Error
	if errors.As(err, &se) && (se.Kind == service.KindValidation || se.Kind == service.KindConflict) {
		// Show the form again with what was typed, so nothing is lost (AC-13, AC-2).
		h.page(w, statusFor(se.Kind), "lab_new", pageData{Title: newLabTitle, User: &c, Error: se.Message, Form: in})
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/staff/labs/%d?created=1", lab.ID), http.StatusSeeOther)
}

func (h *handler) staffLabPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	// An id that is not a number matches no lab. It still goes to the service, which answers 403
	// to a student before it looks anything up (AC-45).
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		id = 0
	}
	lab, bookers, err := h.Labs.Bookings(r.Context(), c, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := pageData{Title: lab.Title + " bookings · Labwiz", User: &c, Lab: &lab, Bookers: bookers}
	if r.URL.Query().Get("created") == "1" {
		data.Success = "Lab added."
	}
	h.page(w, http.StatusOK, "lab_staff", data)
}
