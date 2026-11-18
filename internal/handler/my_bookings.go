package handler

import (
	"net/http"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
)

// bookingsPage lists the signed-in student's bookings (AC-90). Staff get 403 from the service.
func (h *handler) bookingsPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	page, err := h.Labs.MyBookings(r.Context(), c)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	data := pageData{Title: "My bookings · Labwiz", User: &c, Bookings: &page}
	if r.URL.Query().Get("cancelled") == "1" {
		data.Success = "Booking cancelled."
	}
	h.page(w, r, http.StatusOK, "bookings", data)
}

// cancelBooking cancels the student's own booking and goes to their bookings (AC-7). Refusals get the
// error page: 404 without a booking (AC-49), 409 once the session has started (AC-88).
func (h *handler) cancelBooking(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	if _, err := h.Labs.CancelBooking(r.Context(), c, idParam(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/bookings?cancelled=1", http.StatusSeeOther)
}
