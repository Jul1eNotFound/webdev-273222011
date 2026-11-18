package handler_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

const (
	msgNoBooking          = "You have no booking for this session."
	msgCancelBookingLate  = "This session has already started, so the booking cannot be cancelled."
	bookedButton          = `disabled>Booked</button>`
	cancelBookingFormFor1 = `action="/sessions/1/cancel-booking"`
	zoeEmail              = "zoe@labwiz.test"
	zoePass               = "zoe-password"
)

// addZoe adds a second student who can sign in to the fake app.
func addZoe(t *testing.T, st *servicetest.Store) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(zoePass), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	st.AddUser(store.User{ID: 9, Email: zoeEmail, Name: "Zoe", PasswordHash: string(hash), Role: "student"})
}

// In newStaffApp, Sam (student) can book session 1 (Chemistry basics, today 09:00–12:00, 10 places, Ana and
// Ben already booked it) and session 3 (Robotics intro, tomorrow). The clock says 08:30.

func TestAC7_StudentCancelsABookingAndThePlaceComesBack(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	bookByForm(t, c, base, "/sessions/1/book")
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs/1", nil), http.StatusOK, "7 of 10 places free")

	wantRedirect(t, do(t, c, http.MethodPost, base+"/sessions/1/cancel-booking", url.Values{}), http.StatusSeeOther, "/bookings?cancelled=1")
	wantStatus(t, do(t, c, http.MethodGet, base+"/bookings?cancelled=1", nil), http.StatusOK, "Booking cancelled.")
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs/1", nil), http.StatusOK, "8 of 10 places free")
	if len(st.Bookers[1]) != 2 {
		t.Fatalf("bookers = %v, want Ana and Ben only", st.Bookers[1])
	}
	// The place can be booked again.
	wantRedirect(t, bookByForm(t, c, base, "/sessions/1/book"), http.StatusSeeOther, "/labs/1?booked=1")
}

func TestAC7_CancelBookingFormsAreOnTheLabPageAndMyBookings(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	bookByForm(t, c, base, "/sessions/1/book")
	for _, path := range []string{"/labs/1", "/bookings"} {
		r := do(t, c, http.MethodGet, base+path, nil)
		wantStatus(t, r, http.StatusOK, cancelBookingFormFor1)
		wantStatus(t, r, http.StatusOK, ">Cancel booking</button>")
	}
	// The lab list stays compact: "Booked", no cancel form.
	if strings.Contains(do(t, c, http.MethodGet, base+"/labs", nil).body, cancelBookingFormFor1) {
		t.Error("/labs has a Cancel booking form")
	}
}

// booking.js takes over every form with class "book-form" and treats any 200 as a booking. A cancel
// form with that class was cancelled by the script and then shown as "Booked", so only Book forms may
// carry it; the cancel form posts normally.
func TestAC7_OnlyBookFormsAreTakenOverByTheBookingScript(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	bookByForm(t, c, base, "/sessions/1/book")
	scripted := regexp.MustCompile(`<form[^>]*class="book-form"[^>]*>`)
	action := regexp.MustCompile(`action="/sessions/\d+/book"`)
	for _, path := range []string{"/labs", "/labs/1", "/bookings"} {
		for _, form := range scripted.FindAllString(do(t, c, http.MethodGet, base+path, nil).body, -1) {
			if !action.MatchString(form) {
				t.Errorf("%s: booking.js takes over %s", path, form)
			}
		}
	}
}

func TestAC49_CancellingWithoutABookingIs404(t *testing.T) {
	base, st := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	wantStatus(t, do(t, c, http.MethodPost, base+"/sessions/1/cancel-booking", url.Values{}), http.StatusNotFound, msgNoBooking)
	wantStatus(t, do(t, c, http.MethodPost, base+"/sessions/99/cancel-booking", url.Values{}), http.StatusNotFound, msgSessionNotFound)
	if len(st.Bookers[1]) != 2 {
		t.Fatal("someone else's booking was cancelled")
	}
}

func TestAC88_CancellingAfterTheStartIs409(t *testing.T) {
	base, st := newStaffApp(t)
	st.LabSessions = append(st.LabSessions, fakeSession(4, 3, 7, 8)) // Robotics, 07:00–08:00, over by 08:30
	c := signedIn(t, base, studentEmail, studentPass)
	// Booked before it started, straight in the store.
	if _, err := st.BookSession(t.Context(), 1, 4, func(store.SessionView, bool) error { return nil }); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, do(t, c, http.MethodPost, base+"/sessions/4/cancel-booking", url.Values{}), http.StatusConflict, msgCancelBookingLate)
	if len(st.Bookers[4]) != 1 {
		t.Fatal("the booking was not kept")
	}
	if strings.Contains(do(t, c, http.MethodGet, base+"/bookings", nil).body, `action="/sessions/4/cancel-booking"`) {
		t.Error("/bookings offers to cancel a booking of a session that is over")
	}
}

func TestAC89_BookedSessionShowsBookedAfterAReload(t *testing.T) {
	base, st := newStaffApp(t)
	addZoe(t, st)
	c := signedIn(t, base, studentEmail, studentPass)
	bookByScript(t, c, base, "/sessions/1/book")
	booked := regexp.MustCompile(`action="/sessions/1/book"(?:[^<]|<input type="hidden" name="csrf"[^>]*>)*<button[^>]*` + bookedButton)
	for _, path := range []string{"/labs", "/labs/1"} {
		if !booked.MatchString(do(t, c, http.MethodGet, base+path, nil).body) {
			t.Errorf("%s does not show a disabled Booked button for session 1", path)
		}
	}
	// Another student sees Book, not Booked.
	other := signedIn(t, base, zoeEmail, zoePass)
	if strings.Contains(do(t, other, http.MethodGet, base+"/labs", nil).body, bookedButton) {
		t.Error("another student sees Booked")
	}
}

func TestAC90_MyBookingsShowsTitleTimeAndCancelledByStaff(t *testing.T) {
	base, _ := newStaffApp(t)
	student := signedIn(t, base, studentEmail, studentPass)
	bookByForm(t, student, base, "/sessions/1/book")
	bookByForm(t, student, base, "/sessions/3/book")
	do(t, signedIn(t, base, staffEmail, staffPass), http.MethodPost, base+"/staff/sessions/3/cancel", url.Values{})

	r := do(t, student, http.MethodGet, base+"/bookings", nil)
	for _, want := range []string{
		"<title>My bookings · Labwiz</title>", `<a href="/labs/1">Chemistry basics</a>`, "Thu 22 Oct, 09:00–12:00",
		`<a href="/labs/3">Robotics intro</a>`, "Fri 23 Oct, 09:00–12:00", "Cancelled by staff",
	} {
		wantStatus(t, r, http.StatusOK, want)
	}
	if n := strings.Count(r.body, "Cancelled by staff"); n != 1 {
		t.Errorf("Cancelled by staff appears %d times, want once", n)
	}
	if strings.Contains(r.body, `action="/sessions/3/cancel-booking"`) {
		t.Error("the cancelled session still offers Cancel booking")
	}
	wantStatus(t, do(t, student, http.MethodGet, base+"/labs", nil), http.StatusOK, `href="/bookings">My bookings</a>`)
}

func TestAC90_NoBookingsYetSaysSo(t *testing.T) {
	base, _ := newStaffApp(t)
	r := do(t, signedIn(t, base, studentEmail, studentPass), http.MethodGet, base+"/bookings", nil)
	wantStatus(t, r, http.StatusOK, "You have no bookings yet.")
	wantStatus(t, r, http.StatusOK, `<a href="/labs">Find a lab to book</a>`)
}

func TestMyBookingsAndCancelBookingAreForStudents(t *testing.T) {
	base, st := newStaffApp(t)
	staff := signedIn(t, base, staffEmail, staffPass)
	wantStatus(t, do(t, staff, http.MethodGet, base+"/bookings", nil), http.StatusForbidden, msgForbidden)
	wantStatus(t, do(t, staff, http.MethodPost, base+"/sessions/1/cancel-booking", url.Values{}), http.StatusForbidden, msgForbidden)
	if strings.Contains(do(t, staff, http.MethodGet, base+"/labs", nil).body, `href="/bookings"`) {
		t.Error("staff are shown the My bookings link")
	}
	wantRedirect(t, do(t, newClient(t), http.MethodGet, base+"/bookings", nil), http.StatusSeeOther, "/login")
	if len(st.Bookers[1]) != 2 {
		t.Fatal("a booking changed")
	}
}

func TestMyBookingsPageMeetsTheDefinitionOfDone_AC30_AC42_AC87(t *testing.T) {
	base, _ := newStaffApp(t)
	c := signedIn(t, base, studentEmail, studentPass)
	bookByForm(t, c, base, "/sessions/1/book")
	r := do(t, c, http.MethodGet, base+"/bookings", nil)
	if m := regexp.MustCompile(`(?i)transition|animation|@keyframes`).FindString(r.body); m != "" {
		t.Errorf("/bookings contains %q", m)
	}
	token := tokenFor(c, base+"/bookings")
	withToken := regexp.MustCompile(`(?s)<form method="post"[^>]*>\s*<input type="hidden" name="csrf" value="` + regexp.QuoteMeta(token) + `">`)
	if forms := strings.Count(r.body, `<form method="post"`); forms < 2 || len(withToken.FindAllString(r.body, -1)) != forms {
		t.Errorf("/bookings: %d post forms, %d carry the token", forms, len(withToken.FindAllString(r.body, -1)))
	}
	// Without the token, a cancel is refused and the booking stays (AC-85).
	wantStatus(t, do(t, c, http.MethodPost, base+"/sessions/1/cancel-booking", url.Values{"csrf": {"x"}}), http.StatusForbidden, msgExpired)
}
