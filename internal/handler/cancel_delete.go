package handler

import (
	"fmt"
	"net/http"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
)

// The owner's confirmation pages (AC-76). A GET only shows what will change; the POST does it. Both refuse
// with the same 403, 404 and 409 as the service, through the error page (AC-43).

func (h *handler) cancelPage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	s, err := h.Labs.CancelPage(r.Context(), c, idParam(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, http.StatusOK, "confirm", pageData{Title: "Cancel a session · Labwiz", User: &c, CancelSession: &s})
}

// cancelSession cancels the session and goes back to its lab's staff page (AC-73).
func (h *handler) cancelSession(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	s, err := h.Labs.CancelSession(r.Context(), c, idParam(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/staff/labs/%d?cancelled=1", s.LabID), http.StatusSeeOther)
}

func (h *handler) deletePage(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	lab, err := h.Labs.DeletePage(r.Context(), c, idParam(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.page(w, http.StatusOK, "confirm", pageData{Title: "Delete a lab · Labwiz", User: &c, DeleteLab: &lab})
}

// deleteLab deletes the lab and lands on My labs (AC-77).
func (h *handler) deleteLab(w http.ResponseWriter, r *http.Request) {
	c, _ := auth.CallerFrom(r.Context())
	if _, err := h.Labs.DeleteLab(r.Context(), c, idParam(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/staff/labs?deleted=1", http.StatusSeeOther)
}
