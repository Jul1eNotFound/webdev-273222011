package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// The MCP tools against real PostgreSQL. The clock says 08:30; seed.Run gives Stella (staff) four labs:
// Chemistry basics 09:00–12:00 (10 places), Circuits workshop 13:00–17:00 (2), Open microscopy
// 08:00–20:00 (5, started) and Robotics intro tomorrow (8).

// dbState is every table a tool could change, to compare before and after.
func dbState(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var cancelled int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM lab_sessions WHERE cancelled_at IS NOT NULL`).Scan(&cancelled); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("labs=%d sessions=%d cancelled=%d bookings=%d",
		storetest.Count(t, pool, "labs"), storetest.Count(t, pool, "lab_sessions"), cancelled, storetest.Count(t, pool, "bookings"))
}

func wantDBUnchanged(t *testing.T, pool *pgxpool.Pool, before string) {
	t.Helper()
	if after := dbState(t, pool); after != before {
		t.Fatalf("the database changed:\nbefore %s\nafter  %s", before, after)
	}
}

// addAccount adds a user who can sign in, and returns their id.
func addAccount(t *testing.T, st *store.Store, email, password, role string) int64 {
	t.Helper()
	hash, err := auth.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(context.Background(), store.User{Email: email, Name: email, PasswordHash: hash, Role: role}); err != nil {
		t.Fatal(err)
	}
	return userIDOf(t, st, email)
}

func TestAC39_DB_ToolsWithoutAValidTokenChangeNothing(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics")
	args := map[string]map[string]any{
		"list_labs": {}, "my_bookings": {},
		"get_lab": {"lab_id": chemistry.LabID}, "list_lab_bookings": {"lab_id": chemistry.LabID},
		"book_lab": {"session_id": chemistry.SessionID}, "cancel_booking": {"session_id": chemistry.SessionID},
		"add_lab":          {"title": "Optics", "place_limit": 4},
		"schedule_session": {"lab_id": chemistry.LabID, "starts": "2026-10-22T18:00", "ends": "2026-10-22T19:00"},
		"cancel_session":   {"session_id": chemistry.SessionID},
	}
	before := dbState(t, pool)
	for _, token := range []string{"", "made-up-token"} {
		cs := mcpSession(t, base, token)
		for name, a := range args {
			wantToolError(t, name, callTool(t, cs, name, a), msgSignIn)
		}
	}
	wantDBUnchanged(t, pool, before)
}

func TestAC37_DB_BookLabMatchesTheHTTPBooking(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics")
	samID := userIDOf(t, st, seed.StudentEmail)
	addAccount(t, st, "zoe@labwiz.test", "zoe-password", "student")

	var got bookingOut
	cs := mcpSession(t, base, tokenOf(t, base, seed.StudentEmail, seedPasswords.Student))
	toolOut(t, "book_lab", callTool(t, cs, "book_lab", map[string]any{"session_id": chemistry.SessionID}), &got)
	if got != (bookingOut{"Booking confirmed.", 9, 10}) {
		t.Fatalf("book_lab = %+v", got)
	}
	if n := bookingRows(t, pool, chemistry.SessionID, samID); n != 1 {
		t.Fatalf("Sam's rows = %d, want 1", n)
	}
	// The same booking through the booking script says the same, one place later.
	web := bookByScript(t, signedIn(t, base, "zoe@labwiz.test", "zoe-password"), base, bookPath(chemistry.SessionID))
	if web.status != http.StatusOK || web.Message != got.Message || web.FreePlaces == nil || *web.FreePlaces != got.FreePlaces-1 {
		t.Fatalf("HTTP booking = %+v, MCP = %+v", web, got)
	}
	if n := bookingRows(t, pool, chemistry.SessionID, 0); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
}

func TestAC25_DB_FullSessionIsRefusedWithTheHTTPSentence(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	circuits := seeded(t, st, "Circuits workshop") // 2 places
	for _, id := range addStudents(t, st, 2) {
		storetest.InsertBooking(t, pool, id, circuits.SessionID)
	}
	before := dbState(t, pool)
	cs := mcpSession(t, base, tokenOf(t, base, seed.StudentEmail, seedPasswords.Student))
	res := callTool(t, cs, "book_lab", map[string]any{"session_id": circuits.SessionID})
	web := bookByScript(t, signedIn(t, base, seed.StudentEmail, seedPasswords.Student), base, bookPath(circuits.SessionID))
	if web.status != http.StatusConflict {
		t.Fatalf("HTTP status = %d", web.status)
	}
	wantToolError(t, "book_lab", res, web.Message)
	wantDBUnchanged(t, pool, before)
}

func TestAC51_AC26_DB_TheWrongRoleChangesNothing(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics")
	before := dbState(t, pool)
	staff := mcpSession(t, base, tokenOf(t, base, seed.StaffEmail, seedPasswords.Staff))
	wantToolError(t, "book_lab by staff", callTool(t, staff, "book_lab", map[string]any{"session_id": chemistry.SessionID}), msgForbidden)
	student := mcpSession(t, base, tokenOf(t, base, seed.StudentEmail, seedPasswords.Student))
	wantToolError(t, "add_lab by a student", callTool(t, student, "add_lab", map[string]any{"title": "Optics", "place_limit": 4}), msgForbidden)
	wantDBUnchanged(t, pool, before)
}

func TestAC38_DB_TakenTitleIsRefusedWithTheHTTPSentence(t *testing.T) {
	base, _, pool := newDBAppWithPool(t)
	before := dbState(t, pool)
	cs := mcpSession(t, base, tokenOf(t, base, seed.StaffEmail, seedPasswords.Staff))
	wantToolError(t, "add_lab", callTool(t, cs, "add_lab", map[string]any{"title": "Chemistry basics", "place_limit": 4}), msgTitleTaken)
	web := do(t, signedIn(t, base, seed.StaffEmail, seedPasswords.Staff), http.MethodPost, base+"/staff/labs/new", labForm("Chemistry basics", "4"))
	wantStatus(t, web, http.StatusConflict, msgTitleTaken)
	wantDBUnchanged(t, pool, before)
}

func TestAC35_DB_AnotherStudentsBookingIsKept(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	robotics := firstSessionOfView(t, st, pool, "Robotics intro")
	other := addStudents(t, st, 1)[0]
	storetest.InsertBooking(t, pool, other, robotics.SessionID)
	cs := mcpSession(t, base, tokenOf(t, base, seed.StudentEmail, seedPasswords.Student))
	wantToolError(t, "cancel_booking", callTool(t, cs, "cancel_booking", map[string]any{"session_id": robotics.SessionID}), msgNoBooking)
	if n := bookingRows(t, pool, robotics.SessionID, other); n != 1 {
		t.Fatalf("the other student's rows = %d, want 1", n)
	}
}

// periodsOf is a lab's sessions as "start–end" in time order.
func periodsOf(t *testing.T, pool *pgxpool.Pool, labID int64) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT starts_at, ends_at FROM lab_sessions WHERE lab_id = $1 ORDER BY starts_at`, labID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s, e any
		if err := rows.Scan(&s, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprint(s, "–", e))
	}
	return out
}

func TestAC92_DB_ScheduleSessionAddsTheSameRowsAsTheWebPage(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	ctx := context.Background()
	stella := userIDOf(t, st, seed.StaffEmail)
	viaMCP, err := st.CreateLab(ctx, store.Lab{Title: "Via MCP", PlaceLimit: 3, OwnerID: stella})
	if err != nil {
		t.Fatal(err)
	}
	viaWeb, err := st.CreateLab(ctx, store.Lab{Title: "Via the web", PlaceLimit: 3, OwnerID: stella})
	if err != nil {
		t.Fatal(err)
	}
	cs := mcpSession(t, base, tokenOf(t, base, seed.StaffEmail, seedPasswords.Staff))
	toolOut(t, "schedule_session", callTool(t, cs, "schedule_session", map[string]any{
		"lab_id": viaMCP.ID, "starts": "2026-10-22T14:00", "ends": "2026-10-22T16:00", "repeat_weekly_until": "2026-11-05",
	}), &struct{}{})
	form := url.Values{"starts": {"2026-10-22T14:00"}, "ends": {"2026-10-22T16:00"}, "repeat_until": {"2026-11-05"}}
	r := do(t, signedIn(t, base, seed.StaffEmail, seedPasswords.Staff), http.MethodPost, base+staffLabPath(viaWeb.ID)+"/sessions", form)
	if r.status != http.StatusSeeOther {
		t.Fatalf("web add = %d\n%s", r.status, r.body)
	}
	mcpRows, webRows := periodsOf(t, pool, viaMCP.ID), periodsOf(t, pool, viaWeb.ID)
	if len(mcpRows) != 3 || fmt.Sprint(mcpRows) != fmt.Sprint(webRows) {
		t.Fatalf("MCP rows %v, web rows %v; want the same 3", mcpRows, webRows)
	}

	before := dbState(t, pool)
	wantToolError(t, "overlapping schedule_session", callTool(t, cs, "schedule_session", map[string]any{
		"lab_id": viaMCP.ID, "starts": "2026-10-29T15:00", "ends": "2026-10-29T17:00",
	}), msgSessionOverlap)
	web := do(t, signedIn(t, base, seed.StaffEmail, seedPasswords.Staff), http.MethodPost, base+staffLabPath(viaWeb.ID)+"/sessions",
		url.Values{"starts": {"2026-10-29T15:00"}, "ends": {"2026-10-29T17:00"}})
	wantStatus(t, web, http.StatusConflict, msgSessionOverlap)
	wantDBUnchanged(t, pool, before)
}

func TestAC93_DB_CancelSessionKeepsItsBookingsLikeTheWebPage(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	robotics := firstSessionOfView(t, st, pool, "Robotics intro")
	storetest.InsertBooking(t, pool, userIDOf(t, st, seed.StudentEmail), robotics.SessionID)
	cs := mcpSession(t, base, tokenOf(t, base, seed.StaffEmail, seedPasswords.Staff))
	var got toolSession
	toolOut(t, "cancel_session", callTool(t, cs, "cancel_session", map[string]any{"session_id": robotics.SessionID}), &got)
	if !got.Cancelled {
		t.Fatalf("cancel_session = %+v", got)
	}
	var cancelled bool
	if err := pool.QueryRow(context.Background(), `SELECT cancelled_at IS NOT NULL FROM lab_sessions WHERE id = $1`, robotics.SessionID).Scan(&cancelled); err != nil || !cancelled {
		t.Fatalf("cancelled_at set = %v, err = %v", cancelled, err)
	}
	if n := bookingRows(t, pool, robotics.SessionID, 0); n != 1 {
		t.Fatalf("bookings = %d, want the 1 kept", n)
	}
	// The student sees it as the web page shows it.
	wantStatus(t, do(t, signedIn(t, base, seed.StudentEmail, seedPasswords.Student), http.MethodGet, base+"/bookings", nil), http.StatusOK, "Cancelled by staff")
}

func TestAC94_DB_OnlyTheOwnerSchedulesAndCancels(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	robotics := firstSessionOfView(t, st, pool, "Robotics intro")
	addAccount(t, st, "omar@labwiz.test", "omar-password", "staff")
	before := dbState(t, pool)
	for _, who := range []struct{ email, pass string }{{seed.StudentEmail, seedPasswords.Student}, {"omar@labwiz.test", "omar-password"}} {
		cs := mcpSession(t, base, tokenOf(t, base, who.email, who.pass))
		wantToolError(t, "schedule_session by "+who.email, callTool(t, cs, "schedule_session", map[string]any{
			"lab_id": robotics.LabID, "starts": "2026-10-24T09:00", "ends": "2026-10-24T10:00",
		}), msgForbidden)
		wantToolError(t, "cancel_session by "+who.email, callTool(t, cs, "cancel_session", map[string]any{"session_id": robotics.SessionID}), msgForbidden)
	}
	wantDBUnchanged(t, pool, before)
}
