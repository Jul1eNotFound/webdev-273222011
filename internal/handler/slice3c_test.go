package handler_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

const (
	msgCancelled      = "This session has been cancelled."
	msgCancelStarted  = "This session has already started."
	msgLabHasBookings = "This lab has bookings. Cancel its sessions instead."
)

// In newStaffApp, Stella owns lab 1 (Chemistry basics: session 1 today 09:00–12:00, booked by Ana and Ben),
// lab 2 (Circuits workshop: session 2, Cleo) and lab 3 (Robotics intro: session 3 tomorrow, never booked).
// The clock says 08:30.

func TestAC76_CancelPageNamesTheSessionAndChangesNothing(t *testing.T) {
	base, st := newStaffApp(t)
	r := do(t, signedIn(t, base, staffEmail, staffPass), http.MethodGet, base+"/staff/sessions/1/cancel", nil)
	for _, want := range []string{
		"<title>Cancel a session · Labwiz</title>", "Chemistry basics", "Thu 22 Oct, 09:00–12:00", "Its 2 bookings are kept",
		`action="/staff/sessions/1/cancel"`, ">Cancel session</button>", `<a href="/staff/labs/1">Keep it</a>`,
	} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if v, _ := st.SessionByID(t.Context(), 1); v.CancelledAt != nil {
		t.Fatal("opening the page cancelled the session")
	}
}

func TestAC76_DeletePageNamesTheLabAndChangesNothing(t *testing.T) {
	base, st := newStaffApp(t)
	r := do(t, signedIn(t, base, staffEmail, staffPass), http.MethodGet, base+"/staff/labs/3/delete", nil)
	for _, want := range []string{
		"<title>Delete a lab · Labwiz</title>", "Delete <strong>Robotics intro</strong> and its 1 session?", "This cannot be undone.",
		`action="/staff/labs/3/delete"`, ">Delete lab</button>", `<a href="/staff/labs/3">Keep it</a>`,
	} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if st.LabCount() != 3 || st.SessionCount(3) != 1 {
		t.Fatal("opening the page deleted something")
	}
}

func TestAC73_OwnerCancelsASessionAndStudentsSeeItCancelled(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	wantRedirect(t, do(t, c, http.MethodPost, base+"/staff/sessions/1/cancel", url.Values{}), http.StatusSeeOther, "/staff/labs/1?cancelled=1")
	page := do(t, c, http.MethodGet, base+"/staff/labs/1?cancelled=1", nil)
	for _, want := range []string{"Session cancelled.", `class="lab-card__cancelled">Cancelled`, "Ana Example", "Ben Example"} {
		wantStatus(t, page, http.StatusOK, want)
	}
	if strings.Contains(page.body, "/staff/sessions/1/cancel") {
		t.Error("the staff page still offers to cancel the cancelled session")
	}
	if len(st.Bookers[1]) != 2 {
		t.Fatal("the bookings were not kept")
	}
	student := signedIn(t, base, studentEmail, studentPass)
	for _, path := range []string{"/labs", "/labs/1"} {
		r := do(t, student, http.MethodGet, base+path, nil)
		wantStatus(t, r, http.StatusOK, `<p class="lab-card__cancelled">Cancelled</p>`)
		if strings.Contains(r.body, `action="/sessions/1/book"`) {
			t.Errorf("%s still has a Book form for the cancelled session", path)
		}
	}
}

func TestAC74_BookingACancelledSessionIs409(t *testing.T) {
	base, st := newStaffApp(t)
	do(t, signedIn(t, base, staffEmail, staffPass), http.MethodPost, base+"/staff/sessions/3/cancel", url.Values{})
	c := signedIn(t, base, studentEmail, studentPass)
	wantStatus(t, bookByForm(t, c, base, "/sessions/3/book"), http.StatusConflict, msgCancelled)
	if reply := bookByScript(t, c, base, "/sessions/3/book"); reply.status != http.StatusConflict || reply.Message != msgCancelled {
		t.Fatalf("script: got %d %q", reply.status, reply.Message)
	}
	if n := len(st.Bookers[3]); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC75_CancellingAStartedSessionIs409(t *testing.T) {
	base, st := newStaffApp(t)
	st.LabSessions = append(st.LabSessions, fakeSession(4, 3, 8, 9)) // Robotics, started at 08:00
	c := signedIn(t, base, staffEmail, staffPass)
	wantStatus(t, do(t, c, http.MethodPost, base+"/staff/sessions/4/cancel", url.Values{}), http.StatusConflict, msgCancelStarted)
	wantStatus(t, do(t, c, http.MethodGet, base+"/staff/sessions/4/cancel", nil), http.StatusConflict, msgCancelStarted)
	if v, _ := st.SessionByID(t.Context(), 4); v.CancelledAt != nil {
		t.Fatal("the started session was cancelled")
	}
	page := do(t, c, http.MethodGet, base+"/staff/labs/3", nil)
	if strings.Contains(page.body, "/staff/sessions/4/cancel") || !strings.Contains(page.body, "/staff/sessions/3/cancel") {
		t.Error("the staff page should offer to cancel session 3 but not the started session 4")
	}
}

func TestAC77_OwnerDeletesALabNobodyBooked(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	wantStatus(t, do(t, c, http.MethodGet, base+"/staff/labs/3", nil), http.StatusOK, `<a href="/staff/labs/3/delete">Delete lab</a>`)
	wantRedirect(t, do(t, c, http.MethodPost, base+"/staff/labs/3/delete", url.Values{}), http.StatusSeeOther, "/staff/labs?deleted=1")
	r := do(t, c, http.MethodGet, base+"/staff/labs?deleted=1", nil)
	wantStatus(t, r, http.StatusOK, "Lab deleted.")
	if strings.Contains(r.body, "Robotics intro") || st.LabCount() != 2 || st.SessionCount(3) != 0 {
		t.Fatal("the lab or its session is still there")
	}
	if strings.Contains(do(t, c, http.MethodGet, base+"/staff/labs", nil).body, "Lab deleted.") {
		t.Error("the success message shows without ?deleted=1")
	}
}

func TestAC77_DeletingABookedLabIs409(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	if strings.Contains(do(t, c, http.MethodGet, base+"/staff/labs/1", nil).body, "/staff/labs/1/delete") {
		t.Error("the staff page offers to delete a lab with bookings")
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/staff/labs/1/delete", nil), http.StatusConflict, msgLabHasBookings)
	wantStatus(t, do(t, c, http.MethodPost, base+"/staff/labs/1/delete", url.Values{}), http.StatusConflict, msgLabHasBookings)
	if st.LabCount() != 3 || st.SessionCount(1) != 1 {
		t.Fatal("something was deleted")
	}
}

func TestAC79_OnlyTheOwnerMayCancelOrDelete(t *testing.T) {
	base, st := newTwoStaffApp(t)
	for name, c := range map[string]*http.Client{
		"student":       signedIn(t, base, studentEmail, studentPass),
		"another staff": signedIn(t, base, omarEmail, omarPass),
	} {
		for _, req := range []struct{ method, path string }{
			{http.MethodGet, "/staff/sessions/1/cancel"}, {http.MethodPost, "/staff/sessions/1/cancel"},
			{http.MethodGet, "/staff/labs/3/delete"}, {http.MethodPost, "/staff/labs/3/delete"},
		} {
			var form url.Values
			if req.method == http.MethodPost {
				form = url.Values{}
			}
			r := do(t, c, req.method, base+req.path, form)
			wantStatus(t, r, http.StatusForbidden, msgForbidden)
			if strings.Contains(r.body, "Ana Example") {
				t.Errorf("%s %s %s shows who booked", name, req.method, req.path)
			}
		}
	}
	if v, _ := st.SessionByID(t.Context(), 1); v.CancelledAt != nil || st.LabCount() != 4 {
		t.Fatal("something changed")
	}
	staff := signedIn(t, base, staffEmail, staffPass)
	wantStatus(t, do(t, staff, http.MethodGet, base+"/staff/sessions/99/cancel", nil), http.StatusNotFound, msgSessionNotFound)
	wantStatus(t, do(t, staff, http.MethodPost, base+"/staff/labs/99/delete", url.Values{}), http.StatusNotFound, msgLabNotFound)
}

func TestConfirmPagesMeetTheDefinitionOfDone_AC30_AC41_AC42(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	titles := map[string]string{}
	for _, path := range []string{"/staff/sessions/1/cancel", "/staff/labs/3/delete", "/staff/labs/1", "/staff/labs"} {
		r := do(t, c, http.MethodGet, base+path, nil)
		if m := regexp.MustCompile(`(?i)transition|animation|@keyframes`).FindString(r.body); m != "" {
			t.Errorf("%s contains %q", path, m)
		}
		if !strings.Contains(r.body, `<html lang="en">`) {
			t.Errorf("%s lacks lang=en", path)
		}
		title := regexp.MustCompile(`<title>([^<]+)</title>`).FindStringSubmatch(r.body)
		if title == nil {
			t.Fatalf("%s has no title", path)
		}
		if other, dup := titles[title[1]]; dup {
			t.Errorf("%s and %s share the title %q", path, other, title[1])
		}
		titles[title[1]] = path
	}
}
