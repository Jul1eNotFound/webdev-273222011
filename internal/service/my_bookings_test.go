package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

const anaID = 8

// myStore has, with the clock at 10:00 on 22 Oct, Sam's bookings of session 1 (today 13:00), session 2
// (tomorrow, later cancelled by staff), session 3 (running since 09:00) and session 4 (yesterday), and
// Ana's booking of session 5 (tomorrow), which Sam has not booked.
func myStore(t *testing.T) *servicetest.Store {
	t.Helper()
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: student.ID, Email: "sam@labwiz.test", Name: "Sam", Role: "student"})
	st.AddUser(store.User{ID: anaID, Email: "ana@labwiz.test", Name: "Ana", Role: "student"})
	st.Labs = []store.Lab{{ID: 1, Title: "Optics", PlaceLimit: 2, OwnerID: staff.ID}, {ID: 2, Title: "Robotics", PlaceLimit: 4, OwnerID: staff.ID}}
	st.LabSessions = []servicetest.LabSession{sess(1, 1, 13, 15), sess(2, 1, 37, 39), sess(3, 2, 9, 12), sess(4, 2, -14, -12), sess(5, 2, 33, 35)}
	allow := func(store.SessionView, bool) error { return nil }
	for _, b := range []struct{ user, session int64 }{{student.ID, 1}, {student.ID, 2}, {student.ID, 3}, {student.ID, 4}, {anaID, 5}} {
		if _, err := st.BookSession(context.Background(), b.user, b.session, allow); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestCancelBookingFreesThePlace_AC7(t *testing.T) {
	st := myStore(t)
	s, err := labsOf(st).CancelBooking(context.Background(), student, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s.SessionID != 1 || s.FreePlaces != 2 || len(st.Bookers[1]) != 0 {
		t.Fatalf("session = %+v, bookers = %v; want session 1 with both places free", s, st.Bookers[1])
	}
	// The freed place can be booked again (AC-50).
	if _, err := labsOf(st).Book(context.Background(), student, 1); err != nil {
		t.Fatalf("booking again: %v", err)
	}
}

func TestCancelBookingYouDoNotHaveIsNotFound_AC49(t *testing.T) {
	st := myStore(t)
	l := labsOf(st)
	// Session 5 is Ana's booking, not Sam's: it stays.
	if _, err := l.CancelBooking(context.Background(), student, 5); err != service.ErrBookingNotFound {
		t.Errorf("someone else's booking: err = %v, want ErrBookingNotFound", err)
	}
	if len(st.Bookers[5]) != 1 {
		t.Error("Ana's booking was cancelled")
	}
	if _, err := l.CancelBooking(context.Background(), student, 99); err != service.ErrSessionNotFound {
		t.Errorf("missing session: err = %v, want ErrSessionNotFound", err)
	}
	if got := service.ErrBookingNotFound.Error(); got != "You have no booking for this session." {
		t.Errorf("message = %q", got)
	}
}

func TestCancelBookingAfterTheStartIsRefused_AC88(t *testing.T) {
	st := myStore(t)
	for _, id := range []int64{3, 4} { // running, over
		if _, err := labsOf(st).CancelBooking(context.Background(), student, id); err != service.ErrCancelBookingStarted {
			t.Errorf("session %d: err = %v, want ErrCancelBookingStarted", id, err)
		}
		if len(st.Bookers[id]) != 1 {
			t.Errorf("session %d: the booking was not kept", id)
		}
	}
}

func TestCancelBookingIsForStudentsOnly(t *testing.T) {
	st := myStore(t)
	if _, err := labsOf(st).CancelBooking(context.Background(), staff, 1); err != service.ErrForbidden {
		t.Errorf("staff: err = %v, want ErrForbidden", err)
	}
	if _, err := labsOf(st).MyBookings(context.Background(), staff); err != service.ErrForbidden {
		t.Errorf("staff bookings: err = %v, want ErrForbidden", err)
	}
	if _, err := labsOf(st).MyBookings(context.Background(), service.Caller{}); err != service.ErrUnauthenticated {
		t.Errorf("nobody: err = %v, want ErrUnauthenticated", err)
	}
}

func TestMyBookingsListsOwnBookingsAndMarksCancelled_AC90(t *testing.T) {
	st := myStore(t)
	if _, err := labsOf(st).CancelSession(context.Background(), staff, 2); err != nil {
		t.Fatal(err)
	}
	page, err := labsOf(st).MyBookings(context.Background(), student)
	if err != nil {
		t.Fatal(err)
	}
	ids := func(cs []service.SessionCard) (out []int64) {
		for _, c := range cs {
			out = append(out, c.SessionID)
		}
		return out
	}
	// Upcoming in time order: running 3 (09:00), 1 (13:00), 2 (tomorrow). Past: 4. Never Ana's 5.
	if got := ids(page.Upcoming); len(got) != 3 || got[0] != 3 || got[1] != 1 || got[2] != 2 {
		t.Errorf("upcoming = %v, want 3, 1, 2", got)
	}
	if got := ids(page.Past); len(got) != 1 || got[0] != 4 {
		t.Errorf("past = %v, want 4", got)
	}
	if !page.Upcoming[2].Cancelled || page.Upcoming[1].Cancelled || page.Upcoming[1].Title != "Optics" || !page.Upcoming[1].Mine {
		t.Errorf("upcoming = %+v; want 2 marked cancelled, all mine", page.Upcoming)
	}
}

func TestMyBookingsPastAreMostRecentFirst_AC90(t *testing.T) {
	st := myStore(t)
	st.LabSessions = append(st.LabSessions, sess(6, 1, -40, -38))
	if _, err := st.BookSession(context.Background(), student.ID, 6, func(store.SessionView, bool) error { return nil }); err != nil {
		t.Fatal(err)
	}
	page, err := labsOf(st).MyBookings(context.Background(), student)
	if err != nil || len(page.Past) != 2 || page.Past[0].SessionID != 4 || page.Past[1].SessionID != 6 {
		t.Fatalf("past = %+v, err = %v; want 4 then 6", page.Past, err)
	}
}

func TestListsMarkTheStudentsOwnSessions_AC89(t *testing.T) {
	st := myStore(t)
	l := labsOf(st)
	today, err := l.ListDay(context.Background(), student, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range today.Sessions {
		if !c.Mine {
			t.Errorf("today's session %d is Sam's but not marked", c.SessionID)
		}
	}
	tomorrow, err := l.ListDay(context.Background(), student, "2026-10-23")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range tomorrow.Sessions {
		if want := c.SessionID == 2; c.Mine != want {
			t.Errorf("tomorrow's session %d: Mine = %v, want %v", c.SessionID, c.Mine, want)
		}
	}
	page, err := l.Get(context.Background(), student, 1)
	if err != nil || len(page.Sessions) != 2 || !page.Sessions[0].Mine {
		t.Fatalf("lab page = %+v, err = %v", page.Sessions, err)
	}
	// Staff book nothing: nothing is marked, and no query is made.
	st.ErrSessionsBookedBy = errDB
	if list, err := l.ListDay(context.Background(), staff, ""); err != nil || list.Sessions[0].Mine {
		t.Errorf("staff: %+v, %v", list.Sessions, err)
	}
}

func TestMyBookingsStoreErrors(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		set  func(*servicetest.Store)
		call func(*service.Labs) error
		want error
	}{
		"cancel": {func(st *servicetest.Store) { st.ErrCancelBooking = errDB },
			func(l *service.Labs) error { _, err := l.CancelBooking(ctx, student, 1); return err }, errDB},
		"list": {func(st *servicetest.Store) { st.ErrBookingsOf = errDB },
			func(l *service.Labs) error { _, err := l.MyBookings(ctx, student); return err }, errDB},
		"marking a day": {func(st *servicetest.Store) { st.ErrSessionsBookedBy = errDB },
			func(l *service.Labs) error { _, err := l.ListDay(ctx, student, ""); return err }, errDB},
		"marking a lab": {func(st *servicetest.Store) { st.ErrSessionsBookedBy = errDB },
			func(l *service.Labs) error { _, err := l.Get(ctx, student, 1); return err }, errDB},
	} {
		st := myStore(t)
		tc.set(st)
		if err := tc.call(labsOf(st)); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}
