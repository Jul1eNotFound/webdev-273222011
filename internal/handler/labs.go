package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

const msgBooked = "Booking confirmed."

func (h *handler) labsPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	// No date is today; a date that is not a real day is a 400 through fail (AC-81).
	list, err := h.Labs.ListDay(r.Context(), c, r.URL.Query().Get("date"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, http.StatusOK, "labs", pageData{Title: "Labs · Labwiz", User: &c, Sessions: list.Sessions, Day: &list})
}

// labPage shows one lab and its upcoming sessions, each with its free places and, for students, a
// Book button.
func (h *handler) labPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	lab, err := h.Labs.Get(r.Context(), c, idParam(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := pageData{Title: lab.Lab.Title + " · Labwiz", User: &c, LabPage: &lab}
	if r.URL.Query().Get("booked") == "1" {
		data.Success = msgBooked
	}
	h.page(w, http.StatusOK, "lab", data)
}

// bookingReply is what the booking script reads. It is never shown to the user as it is (AC-43).
type bookingReply struct {
	Message    string `json:"message"`
	FreePlaces *int   `json:"free_places,omitempty"`
	PlaceLimit *int   `json:"place_limit,omitempty"`
}

// book books a place in a session for the signed-in student. The script asks for JSON and updates the
// page in place (AC-4); a plain form post is redirected to the lab page, or shown the error page (AC-1).
func (h *handler) book(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	s, err := h.Labs.Book(r.Context(), c, idParam(r))
	if !wantsJSON(r) {
		if err != nil {
			h.fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/labs/%d?booked=1", s.LabID), http.StatusSeeOther)
		return
	}
	if err != nil {
		h.failJSON(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, bookingReply{Message: msgBooked, FreePlaces: &s.FreePlaces, PlaceLimit: &s.PlaceLimit})
}

// idParam reads the {id} route parameter, a lab's or a session's. Anything that is not a number becomes
// 0, which matches nothing, so it ends in the service's 404 (AC-47).
func idParam(r *http.Request) int64 {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// wantsJSON tells the booking script's requests apart from plain form posts.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// failJSON is fail for the booking script: the same status and sentence, as JSON.
func (h *handler) failJSON(w http.ResponseWriter, r *http.Request, err error) {
	var se *service.Error
	if errors.As(err, &se) {
		writeJSON(w, statusFor(se.Kind), bookingReply{Message: se.Message})
		return
	}
	h.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeJSON(w, http.StatusInternalServerError, bookingReply{Message: msgInternal})
}
