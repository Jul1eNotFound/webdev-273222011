package handler_test

import (
	"context"
	"crypto/sha256"
	"slices"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// The MCP tools against the fake store. In newStaffApp, Stella owns labs 1-3 (session id = lab id):
// Chemistry basics today 09:00–12:00 (Ana and Ben booked it), Circuits workshop today 13:00–17:00 (Cleo)
// and Robotics intro tomorrow. Sam is the student. The clock says 08:30.

// readTools are the tools that change nothing, with arguments that would succeed for a caller allowed
// to use them.
var readTools = map[string]map[string]any{
	"list_labs":         {},
	"get_lab":           {"lab_id": 1},
	"my_bookings":       {},
	"list_lab_bookings": {"lab_id": 1},
}

func TestAC39_ReadToolsWithoutAValidTokenAreRefused(t *testing.T) {
	base, st := newStaffApp(t)
	// An expired session of Sam's: the token was real, but its session is over.
	sum := sha256.Sum256([]byte("expired-token"))
	if err := st.CreateSession(context.Background(), sum[:], 1, fixedNow.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "made-up-token", "expired-token"} {
		cs := mcpSession(t, base, token)
		for name, args := range readTools {
			wantToolError(t, name+" with token "+token, callTool(t, cs, name, args), msgSignIn)
		}
	}
}

func TestAC39_ACookieIsNotATokenForMCP(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	// The signed-in browser's cookie jar is sent, but no bearer header: /mcp must not use the cookie.
	client := c
	client.Transport = bearer{}
	cs := mcpSessionWith(t, base, client)
	wantToolError(t, "list_labs", callTool(t, cs, "list_labs", nil), msgSignIn)
}

func TestListLabsToolListsTheDayWithBookedByMe(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	bookByForm(t, c, base, "/sessions/1/book")
	cs := mcpSession(t, base, sessionToken(t, c, base))

	var today struct {
		Date     string        `json:"date"`
		Sessions []toolSession `json:"sessions"`
	}
	toolOut(t, "list_labs", callTool(t, cs, "list_labs", nil), &today)
	if today.Date != "2026-10-22" || len(today.Sessions) != 2 {
		t.Fatalf("today = %+v, want 2 sessions on 2026-10-22", today)
	}
	s := today.Sessions[0]
	if s.SessionID != 1 || s.Title != "Chemistry basics" || s.Owner != "Stella Staff" || s.FreePlaces != 7 ||
		s.PlaceLimit != 10 || !s.BookedByMe || s.StartsAt != "2026-10-22T09:00:00Z" || s.Bookers != nil {
		t.Errorf("session 1 = %+v", s)
	}
	if today.Sessions[1].BookedByMe {
		t.Error("session 2 is marked as Sam's")
	}

	var tomorrow struct {
		Sessions []toolSession `json:"sessions"`
	}
	toolOut(t, "list_labs", callTool(t, cs, "list_labs", map[string]any{"date": "2026-10-23"}), &tomorrow)
	if len(tomorrow.Sessions) != 1 || tomorrow.Sessions[0].Title != "Robotics intro" {
		t.Errorf("tomorrow = %+v", tomorrow.Sessions)
	}
	wantToolError(t, "list_labs", callTool(t, cs, "list_labs", map[string]any{"date": "22/10/2026"}), "Please choose a valid date.")
}

func TestAC36_StudentGetsTheLabWithoutBookersAndNoBookingList(t *testing.T) {
	base, _ := newStaffApp(t)
	cs := mcpSession(t, base, tokenOf(t, base, studentEmail, studentPass))
	wantToolError(t, "list_lab_bookings", callTool(t, cs, "list_lab_bookings", map[string]any{"lab_id": 1}), msgForbidden)

	var got struct {
		Lab struct {
			Title      string `json:"title"`
			Owner      string `json:"owner"`
			PlaceLimit int    `json:"place_limit"`
		} `json:"lab"`
		Sessions []toolSession `json:"sessions"`
	}
	res := callTool(t, cs, "get_lab", map[string]any{"lab_id": 1})
	toolOut(t, "get_lab", res, &got)
	if got.Lab.Title != "Chemistry basics" || got.Lab.PlaceLimit != 10 || len(got.Sessions) != 1 || got.Sessions[0].FreePlaces != 8 {
		t.Fatalf("get_lab = %+v", got)
	}
	if got.Sessions[0].Bookers != nil {
		t.Error("a student sees who booked")
	}
	wantNoBookerData(t, "get_lab", toolText(res))
	wantToolError(t, "get_lab", callTool(t, cs, "get_lab", map[string]any{"lab_id": 99}), msgLabNotFound)
}

func TestAC91_StaffWhoDoNotOwnTheLabGetNoBookingLists(t *testing.T) {
	base, _ := newTwoStaffApp(t)
	cs := mcpSession(t, base, tokenOf(t, base, omarEmail, omarPass))
	wantToolError(t, "list_lab_bookings", callTool(t, cs, "list_lab_bookings", map[string]any{"lab_id": 1}), msgForbidden)
	res := callTool(t, cs, "get_lab", map[string]any{"lab_id": 1})
	var got struct {
		Sessions []toolSession `json:"sessions"`
	}
	toolOut(t, "get_lab", res, &got)
	if len(got.Sessions) != 1 || got.Sessions[0].Bookers != nil {
		t.Fatalf("Omar's get_lab of Stella's lab = %+v", got.Sessions)
	}
	wantNoBookerData(t, "get_lab", toolText(res))
	// His own lab does come with its (empty) booking list.
	toolOut(t, "get_lab", callTool(t, cs, "get_lab", map[string]any{"lab_id": 4}), &got)
	if len(got.Sessions) != 1 || got.Sessions[0].Bookers == nil || len(*got.Sessions[0].Bookers) != 0 {
		t.Fatalf("Omar's own lab = %+v", got.Sessions)
	}
}

func TestOwnerGetsBookersFromGetLabAndListLabBookings_AC14(t *testing.T) {
	base, _ := newStaffApp(t)
	cs := mcpSession(t, base, tokenOf(t, base, staffEmail, staffPass))
	var got struct {
		Sessions []toolSession `json:"sessions"`
	}
	toolOut(t, "get_lab", callTool(t, cs, "get_lab", map[string]any{"lab_id": 1}), &got)
	if b := got.Sessions[0].Bookers; b == nil || len(*b) != 2 || (*b)[0].Name != "Ana Example" || (*b)[1].Email != "ben@labwiz.test" {
		t.Fatalf("get_lab bookers = %+v", got.Sessions[0].Bookers)
	}
	var list struct {
		Lab struct {
			LabID int64 `json:"lab_id"`
		} `json:"lab"`
		Upcoming []toolSession `json:"upcoming"`
		Past     []toolSession `json:"past"`
	}
	toolOut(t, "list_lab_bookings", callTool(t, cs, "list_lab_bookings", map[string]any{"lab_id": 2}), &list)
	if list.Lab.LabID != 2 || len(list.Upcoming) != 1 || list.Past == nil || len(*list.Upcoming[0].Bookers) != 1 {
		t.Fatalf("list_lab_bookings = %+v", list)
	}
}

func TestAC40_MyBookingsReturnsOnlyTheCallersBookings(t *testing.T) {
	base, st := newStaffApp(t)
	addZoe(t, st)
	allow := func(store.SessionView, bool) error { return nil }
	for _, b := range []struct{ user, session int64 }{{1, 1}, {1, 3}, {9, 2}} { // Sam: 1 and 3; Zoe: 2
		if _, err := st.BookSession(context.Background(), b.user, b.session, allow); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		email, pass string
		want        []int64
	}{{studentEmail, studentPass, []int64{1, 3}}, {zoeEmail, zoePass, []int64{2}}} {
		var got struct {
			Upcoming []toolSession `json:"upcoming"`
			Past     []toolSession `json:"past"`
		}
		cs := mcpSession(t, base, tokenOf(t, base, tc.email, tc.pass))
		toolOut(t, "my_bookings", callTool(t, cs, "my_bookings", nil), &got)
		var ids []int64
		for _, s := range got.Upcoming {
			ids = append(ids, s.SessionID)
		}
		if !slices.Equal(ids, tc.want) || len(got.Past) != 0 {
			t.Errorf("%s: upcoming %v, past %v; want %v", tc.email, ids, got.Past, tc.want)
		}
	}
	// Staff book nothing: the tool is for students.
	cs := mcpSession(t, base, tokenOf(t, base, staffEmail, staffPass))
	wantToolError(t, "my_bookings", callTool(t, cs, "my_bookings", nil), msgForbidden)
}
