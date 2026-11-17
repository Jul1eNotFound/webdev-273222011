package handler_test

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

const msgOverlap = "This lab already has a session at that time."

// errSQL is a database error whose text must never reach a page (AC-43).
var errSQL = errors.New(`ERROR: conflicting key value violates exclusion constraint (SQLSTATE 23P01)`)

// In newStaffApp, Stella owns lab 1 (Chemistry basics) with session 1 today 09:00–12:00 (Ana, Ben).
// The clock says 08:30.

func TestAC67_OwnerAddsASessionAndItIsListed(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	form := url.Values{"starts": {"2026-10-24T14:00"}, "ends": {"2026-10-24T16:30"}}
	wantRedirect(t, do(t, c, http.MethodPost, base+"/staff/labs/1/sessions", form), http.StatusSeeOther, "/staff/labs/1?scheduled=1")
	if n := st.SessionCount(1); n != 2 {
		t.Fatalf("sessions = %d, want 2", n)
	}
	page := do(t, c, http.MethodGet, base+"/staff/labs/1?scheduled=1", nil)
	for _, want := range []string{"Session added.", "<h3>Sat 24 Oct</h3>", "14:00–16:30", "No one has booked this session yet."} {
		wantStatus(t, page, http.StatusOK, want)
	}
	// Students see it on the lab's page with a Book button.
	pub := do(t, signedIn(t, base, studentEmail, studentPass), http.MethodGet, base+"/labs/1", nil)
	wantStatus(t, pub, http.StatusOK, "Sat 24 Oct, 14:00–16:30")
	if n := strings.Count(pub.body, `class="book-form"`); n != 2 {
		t.Errorf("/labs/1 has %d Book forms, want one per upcoming session (2)", n)
	}
}

func TestAC67_StaffPageGroupsSessionsByDayWithTheirOwnBookers(t *testing.T) {
	base, st := newStaffApp(t)
	st.LabSessions = append(st.LabSessions, fakeSession(4, 1, 14, 16), fakeSession(5, 1, -14, -12))
	st.Bookers[4] = nil
	st.Bookers[5] = st.Bookers[2] // Cleo booked yesterday's session too
	r := do(t, signedIn(t, base, staffEmail, staffPass), http.MethodGet, base+"/staff/labs/1", nil)
	// Today's two sessions under one heading, then the past one under its own.
	if n := strings.Count(r.body, "<h3>Thu 22 Oct</h3>"); n != 1 {
		t.Errorf("today's heading appears %d times, want 1", n)
	}
	order := regexp.MustCompile(`(?s)Upcoming sessions.*09:00–12:00.*Ana Example.*14:00–16:00.*No one has booked this session yet.*Past sessions.*Wed 21 Oct.*Cleo Example`)
	if !order.MatchString(r.body) {
		t.Errorf("sessions are not grouped as upcoming then past, each with its own bookers\n%s", r.body)
	}
}

func TestAC68_BadSessionFormIsShownAgainWithItsMessage(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	for name, tc := range map[string]struct {
		form url.Values
		msg  string
	}{
		"end before start": {url.Values{"starts": {"2026-10-24T14:00"}, "ends": {"2026-10-24T13:00"}}, "The end time must be after the start time."},
		"in the past":      {url.Values{"starts": {"2026-10-21T14:00"}, "ends": {"2026-10-21T16:00"}}, "A session cannot start in the past."},
		"unreadable":       {url.Values{"starts": {"soon"}, "ends": {""}}, "Please enter a valid start and end time."},
		"repeat too long":  {url.Values{"starts": {"2026-10-24T14:00"}, "ends": {"2026-10-24T16:00"}, "repeat_until": {"2027-10-24"}}, "A weekly repeat can add at most 26 sessions."},
	} {
		r := do(t, c, http.MethodPost, base+"/staff/labs/1/sessions", tc.form)
		wantStatus(t, r, http.StatusBadRequest, tc.msg)
		// What was typed is still there, and so is the rest of the page.
		if v := tc.form.Get("starts"); v != "" && !strings.Contains(r.body, `value="`+v+`"`) {
			t.Errorf("%s: the form lost what was typed", name)
		}
		wantStatus(t, r, http.StatusBadRequest, "Ana Example")
	}
	if n := st.SessionCount(1); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
}

func TestAC69_OverlappingSessionIs409(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	r := do(t, c, http.MethodPost, base+"/staff/labs/1/sessions", url.Values{"starts": {"2026-10-22T11:00"}, "ends": {"2026-10-22T13:00"}})
	wantStatus(t, r, http.StatusConflict, msgOverlap)
	if n := st.SessionCount(1); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
}

func TestAC70_WeeklyRepeatAddsOneSessionPerWeek(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	form := url.Values{"starts": {"2026-10-23T14:00"}, "ends": {"2026-10-23T16:00"}, "repeat_until": {"2026-11-13"}}
	wantRedirect(t, do(t, c, http.MethodPost, base+"/staff/labs/1/sessions", form), http.StatusSeeOther, "/staff/labs/1?scheduled=4")
	wantStatus(t, do(t, c, http.MethodGet, base+"/staff/labs/1?scheduled=4", nil), http.StatusOK, "4 sessions added.")
	if n := st.SessionCount(1); n != 5 {
		t.Fatalf("sessions = %d, want 1 + 4", n)
	}
}

func TestAC72_OnlyTheOwnerMayAddSessions(t *testing.T) {
	base, st := newTwoStaffApp(t)
	form := url.Values{"starts": {"2026-10-24T14:00"}, "ends": {"2026-10-24T16:00"}}
	wantStatus(t, do(t, signedIn(t, base, studentEmail, studentPass), http.MethodPost, base+"/staff/labs/1/sessions", form), http.StatusForbidden, msgForbidden)
	wantStatus(t, do(t, signedIn(t, base, omarEmail, omarPass), http.MethodPost, base+"/staff/labs/1/sessions", form), http.StatusForbidden, msgForbidden)
	wantRedirect(t, do(t, newClient(t), http.MethodPost, base+"/staff/labs/1/sessions", form), http.StatusSeeOther, "/login")
	if n := st.SessionCount(1); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
	wantStatus(t, do(t, signedIn(t, base, staffEmail, staffPass), http.MethodPost, base+"/staff/labs/99/sessions", form), http.StatusNotFound, msgLabNotFound)
}

func TestSessionFormThatCannotBeReadIs400(t *testing.T) {
	base, st := newStaffApp(t)
	form := url.Values{"starts": {strings.Repeat("x", 70<<10)}}
	wantStatus(t, do(t, signedIn(t, base, staffEmail, staffPass), http.MethodPost, base+"/staff/labs/1/sessions", form),
		http.StatusBadRequest, "The session form could not be read.")
	if n := st.SessionCount(1); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
}

func TestSessionStoreErrorIsAReadable500(t *testing.T) {
	base, st := newStaffApp(t)
	st.ErrAddSessions = errSQL
	r := do(t, signedIn(t, base, staffEmail, staffPass), http.MethodPost, base+"/staff/labs/1/sessions",
		url.Values{"starts": {"2026-10-24T14:00"}, "ends": {"2026-10-24T16:00"}})
	wantStatus(t, r, http.StatusInternalServerError, "Something went wrong on our side. Please try again.")
	if strings.Contains(r.body, "SQLSTATE") {
		t.Error("the page leaks the database error")
	}
}

func TestMyLabsShowsTheNextSessionOrNone_AC62(t *testing.T) {
	base, st := newStaffApp(t)
	st.LabSessions = st.LabSessions[1:] // Chemistry basics has no session left
	r := do(t, signedIn(t, base, staffEmail, staffPass), http.MethodGet, base+"/staff/labs", nil)
	for _, want := range []string{"Next: Thu 22 Oct, 13:00–17:00", "No upcoming sessions · 0 sessions"} {
		wantStatus(t, r, http.StatusOK, want)
	}
}
