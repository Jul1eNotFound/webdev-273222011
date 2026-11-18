package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// Slice 4 against real PostgreSQL. The clock says 08:30; Chemistry basics runs 09:00–12:00 today.

func cancelBookingPath(sessionID int64) string {
	return "/sessions/" + strconv.FormatInt(sessionID, 10) + "/cancel-booking"
}

func TestAC7_DB_CancelRemovesTheRowAndFreesThePlace(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	chemistry := seeded(t, st, "Chemistry basics")
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	bookByForm(t, c, base, bookPath(chemistry.SessionID))
	wantRedirect(t, do(t, c, http.MethodPost, base+cancelBookingPath(chemistry.SessionID), url.Values{}), http.StatusSeeOther, "/bookings?cancelled=1")
	if n := bookingRows(t, pool, chemistry.SessionID, 0); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs/"+strconv.FormatInt(chemistry.LabID, 10), nil), http.StatusOK, "10 of 10 places free")
}

func TestAC88_DB_CancelAfterTheStartKeepsTheBooking(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	microscopy := seeded(t, st, "Open microscopy").SessionID // 08:00–20:00, started
	storetest.InsertBooking(t, pool, userIDOf(t, st, seed.StudentEmail), microscopy)
	c := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	wantStatus(t, do(t, c, http.MethodPost, base+cancelBookingPath(microscopy), url.Values{}), http.StatusConflict, msgCancelBookingLate)
	if n := bookingRows(t, pool, microscopy, 0); n != 1 {
		t.Fatalf("bookings = %d, want the 1 kept", n)
	}
}

func TestAC50_DB_AfterACancelAnotherStudentFillsTheLastPlace(t *testing.T) {
	_, st, pool := newDBAppWithPool(t)
	ctx := context.Background()
	labs := service.NewLabs(st, clock, time.UTC)
	day := time.Date(fixedNow.Year(), fixedNow.Month(), fixedNow.Day(), 0, 0, 0, 0, time.UTC)
	lab, err := st.CreateLab(ctx, store.Lab{Title: "Last place", PlaceLimit: 2, OwnerID: userIDOf(t, st, seed.StaffEmail)})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.AddSessions(ctx, lab.ID, []store.Period{{StartsAt: day.Add(14 * time.Hour), EndsAt: day.Add(16 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	students := addStudents(t, st, 3)
	as := func(id int64) service.Caller { return service.Caller{ID: id, Role: service.RoleStudent} }
	for _, id := range students[:2] {
		if _, err := labs.Book(ctx, as(id), ids[0]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := labs.Book(ctx, as(students[2]), ids[0]); err != service.ErrLabFull {
		t.Fatalf("third student while full: err = %v, want ErrLabFull", err)
	}
	if _, err := labs.CancelBooking(ctx, as(students[0]), ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := labs.Book(ctx, as(students[2]), ids[0]); err != nil {
		t.Fatalf("third student after the cancel: %v", err)
	}
	if n := bookingRows(t, pool, ids[0], 0); n != 2 {
		t.Fatalf("bookings = %d, want 2, the limit", n)
	}
}

func TestAC90_DB_AStaffCancelShowsOnTheStudentsBookings(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	robotics := firstSessionOfView(t, st, pool, "Robotics intro")
	student := signedIn(t, base, seed.StudentEmail, seedPasswords.Student)
	bookByForm(t, student, base, bookPath(robotics.SessionID))
	do(t, signedIn(t, base, seed.StaffEmail, seedPasswords.Staff), http.MethodPost, base+cancelPath(robotics.SessionID), url.Values{})
	r := do(t, student, http.MethodGet, base+"/bookings", nil)
	wantStatus(t, r, http.StatusOK, "Robotics intro")
	wantStatus(t, r, http.StatusOK, "Cancelled by staff")
}
