package handler_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

const (
	msgBooked        = "Booking confirmed"
	msgAlreadyBooked = "You have already booked this lab"
	msgFull          = "This lab is full"
	msgNotOpen       = "This lab is not open for booking today."
	// bookForm marks a Book button's form; the page also loads /static/booking.js, so "/book" alone is not enough.
	bookForm = `class="book-form"`
)

// In newFakeApp, lab 1 (Chemistry basics, 10 places) and lab 2 (Circuits workshop, 2 places) are
// available today, and lab 3 (Robotics intro) starts tomorrow.

type bookingReply struct {
	status     int
	Message    string `json:"message"`
	FreePlaces *int   `json:"free_places"`
}

// bookByScript posts a booking the way booking.js does, asking for JSON.
func bookByScript(t *testing.T, c *http.Client, base, path string) bookingReply {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var reply bookingReply
	if err := json.Unmarshal(b, &reply); err != nil {
		t.Fatalf("reply is not JSON: %v\n%s", err, b)
	}
	reply.status = res.StatusCode
	return reply
}

// bookByForm posts a booking the way the plain HTML form does.
func bookByForm(t *testing.T, c *http.Client, base, path string) response {
	t.Helper()
	return do(t, c, http.MethodPost, base+path, url.Values{})
}

func fillLab(st *servicetest.Store, labID int64, n int) {
	for range n {
		st.Bookers[labID] = append(st.Bookers[labID], store.Booker{Name: "Someone Else"})
	}
}

func TestAC4_ScriptBookingConfirmsAndReturnsTheNewCount(t *testing.T) {
	base, st := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	reply := bookByScript(t, c, base, "/labs/1/book")
	if reply.status != http.StatusOK || !strings.Contains(reply.Message, msgBooked) {
		t.Fatalf("got %d %q, want 200 %q", reply.status, reply.Message, msgBooked)
	}
	if reply.FreePlaces == nil || *reply.FreePlaces != 9 {
		t.Fatalf("free_places = %v, want 9", reply.FreePlaces)
	}
	if got := st.Bookers[1]; len(got) != 1 || got[0].Name != "Sam Student" {
		t.Fatalf("bookers = %+v, want only Sam Student", got)
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "9 of 10 places free")
}

func TestAC4_FormBookingRedirectsToTheLabWithConfirmation(t *testing.T) {
	base, _ := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	wantRedirect(t, bookByForm(t, c, base, "/labs/1/book"), http.StatusSeeOther, "/labs/1?booked=1")
	r := do(t, c, http.MethodGet, base+"/labs/1?booked=1", nil)
	wantStatus(t, r, http.StatusOK, msgBooked)
	wantStatus(t, r, http.StatusOK, "9 of 10 places free")
}

func TestAC5_SecondBookingIs409(t *testing.T) {
	base, st := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	bookByForm(t, c, base, "/labs/1/book")
	wantStatus(t, bookByForm(t, c, base, "/labs/1/book"), http.StatusConflict, msgAlreadyBooked)
	if reply := bookByScript(t, c, base, "/labs/1/book"); reply.status != http.StatusConflict || !strings.Contains(reply.Message, msgAlreadyBooked) {
		t.Fatalf("script: got %d %q", reply.status, reply.Message)
	}
	if n := len(st.Bookers[1]); n != 1 {
		t.Fatalf("bookings = %d, want 1", n)
	}
}

func TestAC1_FullLabIs409WithLinksToTheListAndHome(t *testing.T) {
	base, st := newFakeApp(t)
	fillLab(st, 2, 2)
	c := signedIn(t, base, studentEmail, studentPass)
	r := bookByForm(t, c, base, "/labs/2/book")
	wantStatus(t, r, http.StatusConflict, msgFull)
	for _, link := range []string{`href="/labs"`, `href="/"`} {
		if !strings.Contains(r.body, link) {
			t.Errorf("the full page has no %s link", link)
		}
	}
	if reply := bookByScript(t, c, base, "/labs/2/book"); reply.status != http.StatusConflict || !strings.Contains(reply.Message, msgFull) {
		t.Fatalf("script: got %d %q", reply.status, reply.Message)
	}
	if n := len(st.Bookers[2]); n != 2 {
		t.Fatalf("bookings = %d, want 2", n)
	}
}

func TestAC8_FullLabShowsADisabledFullButtonAndRefusesAPost(t *testing.T) {
	base, st := newFakeApp(t)
	fillLab(st, 2, 2)
	c := signedIn(t, base, studentEmail, studentPass)
	r := do(t, c, http.MethodGet, base+"/labs", nil)
	full := regexp.MustCompile(`action="/labs/2/book"[^<]*<button[^>]*disabled>Full</button>`)
	if !full.MatchString(r.body) {
		t.Errorf("the full lab has no disabled Full button\n%s", r.body)
	}
	open := regexp.MustCompile(`action="/labs/1/book"[^<]*<button[^>]*>Book</button>`)
	if !open.MatchString(r.body) {
		t.Errorf("the lab with free places has no Book button\n%s", r.body)
	}
	wantStatus(t, bookByForm(t, c, base, "/labs/2/book"), http.StatusConflict, msgFull)
}

func TestAC46_StaffBookingIs403(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	wantStatus(t, bookByForm(t, c, base, "/labs/1/book"), http.StatusForbidden, msgForbidden)
	if reply := bookByScript(t, c, base, "/labs/1/book"); reply.status != http.StatusForbidden {
		t.Fatalf("script: got %d %q", reply.status, reply.Message)
	}
	if n := len(st.Bookers[1]); n != 2 {
		t.Fatalf("bookings = %d, want the 2 seeded ones", n)
	}
	if strings.Contains(do(t, c, http.MethodGet, base+"/labs", nil).body, bookForm) {
		t.Error("staff are shown a Book button")
	}
}

func TestAC47_UnknownLabIs404(t *testing.T) {
	base, _ := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	wantStatus(t, bookByForm(t, c, base, "/labs/999/book"), http.StatusNotFound, msgLabNotFound)
	wantStatus(t, bookByForm(t, c, base, "/labs/abc/book"), http.StatusNotFound, msgLabNotFound)
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs/999", nil), http.StatusNotFound, msgLabNotFound)
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs/abc", nil), http.StatusNotFound, msgLabNotFound)
	if reply := bookByScript(t, c, base, "/labs/999/book"); reply.status != http.StatusNotFound || reply.Message != msgLabNotFound {
		t.Fatalf("script: got %d %q", reply.status, reply.Message)
	}
}

func TestAC48_LabNotAvailableTodayIsRefused(t *testing.T) {
	base, st := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	wantStatus(t, bookByForm(t, c, base, "/labs/3/book"), http.StatusConflict, msgNotOpen)
	if n := len(st.Bookers[3]); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC19_ScriptBookingWithoutSessionIs401(t *testing.T) {
	base, st := newFakeApp(t)
	reply := bookByScript(t, newClient(t), base, "/labs/1/book")
	if reply.status != http.StatusUnauthorized || reply.Message != "Please sign in to continue." {
		t.Fatalf("got %d %q", reply.status, reply.Message)
	}
	if n := len(st.Bookers[1]); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestLabPageShowsTheLabAndABookButton(t *testing.T) {
	base, st := newFakeApp(t)
	st.Labs[0].Description = "Titration and safe handling of reagents."
	c := signedIn(t, base, studentEmail, studentPass)
	r := do(t, c, http.MethodGet, base+"/labs/1", nil)
	for _, want := range []string{"Chemistry basics", "Titration and safe handling of reagents.", "09:00–12:00", "10 of 10 places free", ">Book</button>"} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if strings.Contains(r.body, msgBooked) {
		t.Error("the page confirms a booking that was not made")
	}
}

func TestLabPageForStaffLinksToTheBookingsInsteadOfBooking(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, staffEmail, staffPass)
	r := do(t, c, http.MethodGet, base+"/labs/1", nil)
	wantStatus(t, r, http.StatusOK, `href="/staff/labs/1"`)
	if strings.Contains(r.body, bookForm) {
		t.Error("staff are shown a Book button")
	}
}

func TestBookingStoreErrorIsAReadable500(t *testing.T) {
	base, st := newFakeApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	st.ErrBookLab = errors.New(`pq: SQLSTATE 40P01 deadlock detected`)
	r := bookByForm(t, c, base, "/labs/1/book")
	wantStatus(t, r, http.StatusInternalServerError, "Something went wrong on our side. Please try again.")
	if strings.Contains(r.body, "SQLSTATE") {
		t.Error("the page leaks the database error")
	}
	if reply := bookByScript(t, c, base, "/labs/1/book"); reply.status != http.StatusInternalServerError || strings.Contains(reply.Message, "SQLSTATE") {
		t.Fatalf("script: got %d %q", reply.status, reply.Message)
	}
}
