package handler_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// The MCP tools that change something, against the fake store (see slice5_test.go for the data).

const (
	msgTitleTaken      = "A lab with this title already exists. Please choose a different title."
	msgSessionOverlap  = "This lab already has a session at that time."
	msgBadPeriod       = "Please enter a valid start and end time."
	msgSessionInPast   = "A session cannot start in the past."
	msgAlreadyBookedIt = "You have already booked this lab."
)

// fakeState is everything a tool could change in the fake store, to compare before and after.
func fakeState(st *servicetest.Store) string {
	var cancelled []int64
	for _, s := range st.LabSessions {
		if s.CancelledAt != nil {
			cancelled = append(cancelled, s.ID)
		}
	}
	return fmt.Sprintf("labs=%d sessions=%d cancelled=%v bookers=%v", st.LabCount(), len(st.LabSessions), cancelled, st.Bookers)
}

func wantFakeUnchanged(t *testing.T, st *servicetest.Store, before string) {
	t.Helper()
	if after := fakeState(st); after != before {
		t.Fatalf("the store changed:\nbefore %s\nafter  %s", before, after)
	}
}

// bookingOut is what book_lab and cancel_booking return.
type bookingOut struct {
	Message    string `json:"message"`
	FreePlaces int    `json:"free_places"`
	PlaceLimit int    `json:"place_limit"`
}

func allowAll(store.SessionView, bool) error { return nil }

func TestAC37_BookLabAndCancelBookingSayWhatTheWebPagesSay(t *testing.T) {
	base, st := newStaffApp(t)
	cs := mcpSession(t, base, tokenOf(t, base, studentEmail, studentPass))
	var got bookingOut
	toolOut(t, "book_lab", callTool(t, cs, "book_lab", map[string]any{"session_id": 1}), &got)
	if got != (bookingOut{"Booking confirmed.", 7, 10}) || len(st.Bookers[1]) != 3 {
		t.Fatalf("book_lab = %+v, bookers %v", got, st.Bookers[1])
	}
	wantToolError(t, "book_lab again", callTool(t, cs, "book_lab", map[string]any{"session_id": 1}), msgAlreadyBookedIt)
	toolOut(t, "cancel_booking", callTool(t, cs, "cancel_booking", map[string]any{"session_id": 1}), &got)
	if got != (bookingOut{"Booking cancelled.", 8, 10}) || len(st.Bookers[1]) != 2 {
		t.Fatalf("cancel_booking = %+v, bookers %v", got, st.Bookers[1])
	}
}

func TestAC25_BookLabOnAFullSessionIsRefused(t *testing.T) {
	base, st := newStaffApp(t)
	addZoe(t, st)
	// Circuits workshop has 2 places: Cleo and now Zoe fill them.
	if _, err := st.BookSession(context.Background(), 9, 2, allowAll); err != nil {
		t.Fatal(err)
	}
	before := fakeState(st)
	cs := mcpSession(t, base, tokenOf(t, base, studentEmail, studentPass))
	wantToolError(t, "book_lab", callTool(t, cs, "book_lab", map[string]any{"session_id": 2}), "This lab is full.")
	wantFakeUnchanged(t, st, before)
}

func TestAC26_AC51_ToolsRefuseTheWrongRole(t *testing.T) {
	base, st := newStaffApp(t)
	before := fakeState(st)
	student := mcpSession(t, base, tokenOf(t, base, studentEmail, studentPass))
	wantToolError(t, "add_lab by a student", callTool(t, student, "add_lab", allTools["add_lab"]), msgForbidden)
	staff := mcpSession(t, base, tokenOf(t, base, staffEmail, staffPass))
	wantToolError(t, "book_lab by staff", callTool(t, staff, "book_lab", map[string]any{"session_id": 1}), msgForbidden)
	wantToolError(t, "cancel_booking by staff", callTool(t, staff, "cancel_booking", map[string]any{"session_id": 1}), msgForbidden)
	wantFakeUnchanged(t, st, before)
}

func TestAC38_AddLabWithATakenTitleIsRefused(t *testing.T) {
	base, st := newStaffApp(t)
	cs := mcpSession(t, base, tokenOf(t, base, staffEmail, staffPass))
	before := fakeState(st)
	wantToolError(t, "add_lab", callTool(t, cs, "add_lab", map[string]any{"title": "Chemistry basics", "place_limit": 5}), msgTitleTaken)
	wantToolError(t, "add_lab", callTool(t, cs, "add_lab", map[string]any{"title": "Optics", "place_limit": 0}), msgBadLimit)
	wantFakeUnchanged(t, st, before)

	var lab struct {
		LabID      int64  `json:"lab_id"`
		Owner      string `json:"owner"`
		PlaceLimit int    `json:"place_limit"`
	}
	toolOut(t, "add_lab", callTool(t, cs, "add_lab", allTools["add_lab"]), &lab)
	if lab.LabID != 4 || lab.Owner != "Stella Staff" || lab.PlaceLimit != 4 || st.LabCount() != 4 {
		t.Fatalf("add_lab = %+v, labs %d", lab, st.LabCount())
	}
}

func TestAC35_CancelBookingOfAnotherStudentsBookingIsRefused(t *testing.T) {
	base, st := newStaffApp(t)
	addZoe(t, st)
	if _, err := st.BookSession(context.Background(), 9, 3, allowAll); err != nil {
		t.Fatal(err)
	}
	before := fakeState(st)
	cs := mcpSession(t, base, tokenOf(t, base, studentEmail, studentPass))
	wantToolError(t, "cancel_booking", callTool(t, cs, "cancel_booking", map[string]any{"session_id": 3}), msgNoBooking)
	wantFakeUnchanged(t, st, before)
}

func TestAC92_ScheduleSessionAddsAndRefusesLikeTheWebPage(t *testing.T) {
	base, st := newStaffApp(t)
	cs := mcpSession(t, base, tokenOf(t, base, staffEmail, staffPass))
	var out struct {
		Added int `json:"added"`
	}
	toolOut(t, "schedule_session", callTool(t, cs, "schedule_session", map[string]any{
		"lab_id": 1, "starts": "2026-10-22T18:00", "ends": "2026-10-22T19:00", "repeat_weekly_until": "2026-11-05",
	}), &out)
	if out.Added != 3 || len(st.LabSessions) != 6 {
		t.Fatalf("added %d, sessions %d; want 3 and 6", out.Added, len(st.LabSessions))
	}
	before := fakeState(st)
	for _, tc := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"lab_id": 1, "starts": "22.10.2026 18:00", "ends": "2026-10-22T19:00"}, msgBadPeriod},
		{map[string]any{"lab_id": 1, "starts": "2026-10-21T18:00", "ends": "2026-10-21T19:00"}, msgSessionInPast},
		{map[string]any{"lab_id": 1, "starts": "2026-10-22T18:30", "ends": "2026-10-22T19:30"}, msgSessionOverlap},
		{map[string]any{"lab_id": 99, "starts": "2026-10-22T18:30", "ends": "2026-10-22T19:30"}, msgLabNotFound},
	} {
		wantToolError(t, fmt.Sprint("schedule_session ", tc.args), callTool(t, cs, "schedule_session", tc.args), tc.want)
	}
	wantFakeUnchanged(t, st, before)
}

func TestAC93_CancelSessionKeepsItsBookings(t *testing.T) {
	base, st := newStaffApp(t)
	cs := mcpSession(t, base, tokenOf(t, base, staffEmail, staffPass))
	var got toolSession
	toolOut(t, "cancel_session", callTool(t, cs, "cancel_session", map[string]any{"session_id": 2}), &got)
	if !got.Cancelled || got.SessionID != 2 || len(st.Bookers[2]) != 1 {
		t.Fatalf("cancel_session = %+v, bookers %v", got, st.Bookers[2])
	}
	if res := callTool(t, cs, "cancel_session", map[string]any{"session_id": 2}); !res.IsError {
		t.Error("cancelling a cancelled session again succeeded")
	}
}

func TestAC94_OnlyTheOwnerSchedulesAndCancelsSessions(t *testing.T) {
	base, st := newTwoStaffApp(t)
	before := fakeState(st)
	for _, who := range []struct{ email, pass string }{{studentEmail, studentPass}, {omarEmail, omarPass}} {
		cs := mcpSession(t, base, tokenOf(t, base, who.email, who.pass))
		wantToolError(t, "schedule_session by "+who.email, callTool(t, cs, "schedule_session", allTools["schedule_session"]), msgForbidden)
		wantToolError(t, "cancel_session by "+who.email, callTool(t, cs, "cancel_session", allTools["cancel_session"]), msgForbidden)
	}
	wantFakeUnchanged(t, st, before)
}
