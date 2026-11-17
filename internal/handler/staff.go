package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

const newLabTitle = "Add a lab · Labwiz"

// myLabsPage lists the labs the signed-in staff member owns (AC-62, AC-63).
func (h *handler) myLabsPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	labs, err := h.Labs.MyLabs(r.Context(), c)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, http.StatusOK, "my_labs", pageData{Title: "My labs · Labwiz", User: &c, MyLabs: labs})
}

func (h *handler) newLabForm(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	if err := h.Labs.NewLabForm(c); err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, http.StatusOK, "lab_new", pageData{Title: newLabTitle, User: &c})
}

// readForm caps the body and parses a posted form. It answers 400 itself when the form cannot be read.
func (h *handler) readForm(w http.ResponseWriter, r *http.Request, what string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		h.errorPage(w, r, http.StatusBadRequest, "The "+what+" form could not be read. Please try again.")
		return false
	}
	return true
}

// formError reports whether err is one the form shows next to what was typed (400 or 409), and its message.
func formError(err error) (*service.Error, bool) {
	var se *service.Error
	if errors.As(err, &se) && (se.Kind == service.KindValidation || se.Kind == service.KindConflict) {
		return se, true
	}
	return nil, false
}

func (h *handler) addLab(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	// No role check here: Labs.Add refuses students with 403 (AC-15), as it does for MCP.
	if !h.readForm(w, r, "lab") {
		return
	}
	in := service.NewLab{
		Title:       r.PostFormValue("title"),
		Description: r.PostFormValue("description"),
		PlaceLimit:  r.PostFormValue("place_limit"),
	}
	lab, err := h.Labs.Add(r.Context(), c, in)
	if se, ok := formError(err); ok {
		// Show the form again with what was typed, so nothing is lost (AC-13, AC-2).
		h.page(w, statusFor(se.Kind), "lab_new", pageData{Title: newLabTitle, User: &c, Error: se.Message, Form: in})
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// The new lab has no sessions yet: its owner adds them on its page (AC-12, AC-67).
	http.Redirect(w, r, fmt.Sprintf("/staff/labs/%d?created=1", lab.ID), http.StatusSeeOther)
}

// staffLabPage shows the owner a lab, a form to add sessions, and each session with who booked it.
func (h *handler) staffLabPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	data := pageData{User: &c}
	q := r.URL.Query()
	switch n, _ := strconv.Atoi(q.Get("scheduled")); {
	case q.Get("created") == "1":
		data.Success = "Lab added. Add its first session below."
	case n == 1:
		data.Success = "Session added."
	case n > 1:
		data.Success = fmt.Sprintf("%d sessions added.", n)
	}
	h.staffLab(w, r, http.StatusOK, data)
}

// staffLab renders the owner's page of the lab in the URL with the given status and data.
func (h *handler) staffLab(w http.ResponseWriter, r *http.Request, status int, data pageData) {
	// The id goes to the service even when it is not a number: the service answers 403 to a student
	// before it looks anything up (AC-45), and 404 to staff.
	page, err := h.Labs.Bookings(r.Context(), *data.User, idParam(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data.Title = page.Lab.Title + " sessions · Labwiz"
	data.Staff = &page
	h.page(w, status, "lab_staff", data)
}

// addSessions adds a session, or a weekly series of them, to the owner's lab (AC-67 to AC-72).
func (h *handler) addSessions(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	if !h.readForm(w, r, "session") {
		return
	}
	in := service.NewSession{
		Starts:      r.PostFormValue("starts"),
		Ends:        r.PostFormValue("ends"),
		RepeatUntil: r.PostFormValue("repeat_until"),
	}
	id := idParam(r)
	n, err := h.Labs.AddSessions(r.Context(), c, id, in)
	if se, ok := formError(err); ok {
		// The page again, with the message and what was typed (AC-68, AC-69).
		h.staffLab(w, r, statusFor(se.Kind), pageData{User: &c, Error: se.Message, SessionForm: in})
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/staff/labs/%d?scheduled=%d", id, n), http.StatusSeeOther)
}
