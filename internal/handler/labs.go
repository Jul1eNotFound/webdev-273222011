package handler

import (
	"net/http"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
)

func (h *handler) labsPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	labs, err := h.Labs.ListToday(r.Context(), c)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, http.StatusOK, "labs", pageData{Title: "Labs · Labwiz", User: &c, Labs: labs})
}

// bookStub exists so the 401 for visitors who are not signed in can be tested now (AC-19). Slice 3 replaces it.
func (h *handler) bookStub(w http.ResponseWriter, r *http.Request) {
	h.errorPage(w, r, http.StatusNotImplemented, "Booking is not available yet.")
}
