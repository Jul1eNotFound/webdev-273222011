package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"time"

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
	// CSRF is the signed-in session's token for the page's forms and the booking script (AC-87).
	CSRF string
	// Path is the request path, so the header can mark the page the user is on.
	Path    string
	Error   string
	Success string
	Email   string
	// GoogleEnabled shows "Sign in with Google" on the sign-in page (Slice 6).
	GoogleEnabled bool
	// Sessions is the lab list (/labs).
	Sessions []service.SessionCard
	// Day is the day the lab list shows, with its neighbours (AC-80).
	Day *service.DayList
	// LabPage is one lab and its upcoming sessions (/labs/{id}).
	LabPage *service.LabPage
	// Staff is one owned lab with its sessions and who booked them (/staff/labs/{id}).
	Staff *service.StaffLabPage
	// MyLabs are the caller's own labs (/staff/labs).
	MyLabs []store.OwnedLab
	// Form and SessionForm are what was typed, shown again with a validation message.
	Form        service.NewLab
	SessionForm service.NewSession
	// CancelSession and DeleteLab are what a confirmation page is about (AC-76).
	CancelSession *service.SessionCard
	DeleteLab     *service.LabPage
	// Bookings is the student's own bookings (/bookings, AC-90).
	Bookings *service.BookingsPage
}

type renderer struct {
	pages map[string]*template.Template
}

// newRenderer takes the timezone that lab times are shown in.
func newRenderer(loc *time.Location) (*renderer, error) {
	rn := &renderer{pages: map[string]*template.Template{}}
	funcs := template.FuncMap{"period": period(loc), "day": day(loc), "longDay": longDay(loc), "bookArgs": bookArgs}
	for _, name := range []string{"login", "labs", "lab", "error", "my_labs", "lab_new", "lab_staff", "confirm", "bookings"} {
		t, err := template.New(name).Funcs(funcs).ParseFS(web.FS, "templates/layout.html", "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		rn.pages[name] = t
	}
	return rn, nil
}

// period shows a lab's time period as "HH:MM–HH:MM" within one day, and as
// "D Mon HH:MM – D Mon HH:MM" across days, in the server's timezone (SPEC decisions).
func period(loc *time.Location) func(from, to time.Time) string {
	return func(from, to time.Time) string {
		from, to = from.In(loc), to.In(loc)
		if from.Year() == to.Year() && from.YearDay() == to.YearDay() {
			return from.Format("15:04") + "–" + to.Format("15:04")
		}
		return from.Format("2 Jan 15:04") + " – " + to.Format("2 Jan 15:04")
	}
}

// day names a session's date, as in "Thu 22 Oct", for the staff page's date headings.
func day(loc *time.Location) func(t time.Time) string {
	return func(t time.Time) string { return t.In(loc).Format("Mon 2 Jan") }
}

// bookForm is what the Book button's template part gets: the session and the page's CSRF token.
type bookForm struct {
	service.SessionCard
	CSRF string
}

// bookArgs lets a template pass a session together with the page's token to the bookForm part.
func bookArgs(s service.SessionCard, csrf string) bookForm {
	return bookForm{SessionCard: s, CSRF: csrf}
}

// longDay names the day the lab list shows, as in "Friday 23 October 2026" (AC-80).
func longDay(loc *time.Location) func(t time.Time) string {
	return func(t time.Time) string { return t.In(loc).Format("Monday 2 January 2006") }
}

// page renders into a buffer first, so a template error never sends half a page.
func (h *handler) page(w http.ResponseWriter, r *http.Request, status int, name string, data pageData) {
	data.CSRF = auth.CSRFFrom(r.Context())
	data.Path = r.URL.Path
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

// writeJSON is for the booking script only; pages are always HTML.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errorPage shows one readable sentence (AC-43).
func (h *handler) errorPage(w http.ResponseWriter, r *http.Request, status int, msg string) {
	data := pageData{Title: "Error · Labwiz", Error: msg}
	if c, ok := auth.CallerFrom(r.Context()); ok {
		data.User = &c
	}
	h.page(w, r, status, "error", data)
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
