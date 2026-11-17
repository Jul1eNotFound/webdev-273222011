package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// cancelStore has, with the clock at 10:00:
//   - Stella's lab 1 (Optics): session 1 today 13:00–15:00 booked by Ana, session 2 today 09:00–12:00
//     (running), session 3 tomorrow;
//   - Stella's lab 2 (Acoustics), never booked, with session 4 tomorrow;
//   - Omar's lab 3 (Circuits) with session 5 tomorrow.
func cancelStore() *servicetest.Store {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{
		{ID: 1, Title: "Optics", PlaceLimit: 4, OwnerID: staff.ID},
		{ID: 2, Title: "Acoustics", PlaceLimit: 4, OwnerID: staff.ID},
		{ID: 3, Title: "Circuits", PlaceLimit: 2, OwnerID: otherStaff.ID},
	}
	st.LabSessions = []servicetest.LabSession{
		sess(1, 1, 13, 15), sess(2, 1, 9, 12), sess(3, 1, 37, 39), sess(4, 2, 37, 39), sess(5, 3, 37, 39),
	}
	st.Bookers[1] = []store.Booker{{Name: "Ana"}}
	return st
}

func labsOf(st *servicetest.Store) *service.Labs { return service.NewLabs(st, clock, time.UTC) }

func cancelled(st *servicetest.Store, id int64) bool {
	for _, x := range st.LabSessions {
		if x.ID == id {
			return x.CancelledAt != nil
		}
	}
	return false
}

func TestCancelSessionKeepsItsBookings_AC73(t *testing.T) {
	st := cancelStore()
	s, err := labsOf(st).CancelSession(context.Background(), staff, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Cancelled || s.Title != "Optics" || !cancelled(st, 1) {
		t.Fatalf("session = %+v, want session 1 cancelled", s)
	}
	if len(st.Bookers[1]) != 1 {
		t.Fatal("the booking was not kept")
	}
	// It now shows as cancelled on the lab list.
	today, err := labsOf(st).ListToday(context.Background(), student)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range today {
		if c.SessionID == 1 && !c.Cancelled {
			t.Error("the lab list does not show session 1 as cancelled")
		}
	}
}

func TestBookCancelledSessionIsRefused_AC74(t *testing.T) {
	st := cancelStore()
	st.AddUser(store.User{ID: student.ID, Name: "Sam", Role: "student"})
	if _, err := labsOf(st).CancelSession(context.Background(), staff, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := labsOf(st).Book(context.Background(), student, 3); err != service.ErrSessionCancelled {
		t.Fatalf("err = %v, want ErrSessionCancelled", err)
	}
	if n := len(st.Bookers[3]); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestCancelRefusals_AC75(t *testing.T) {
	st := cancelStore()
	l := labsOf(st)
	if _, err := l.CancelSession(context.Background(), staff, 2); err != service.ErrCancelStarted {
		t.Errorf("running session: err = %v, want ErrCancelStarted", err)
	}
	if _, err := l.CancelPage(context.Background(), staff, 2); err != service.ErrCancelStarted {
		t.Errorf("page of a running session: err = %v, want ErrCancelStarted", err)
	}
	if cancelled(st, 2) {
		t.Error("the running session was cancelled")
	}
	if _, err := l.CancelSession(context.Background(), staff, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := l.CancelSession(context.Background(), staff, 3); err != service.ErrAlreadyCancelled {
		t.Errorf("twice: err = %v, want ErrAlreadyCancelled", err)
	}
	if _, err := l.CancelSession(context.Background(), staff, 99); err != service.ErrSessionNotFound {
		t.Errorf("missing session: err = %v, want ErrSessionNotFound", err)
	}
	if got := service.ErrCancelStarted.Error(); got != "This session has already started." {
		t.Errorf("message = %q", got)
	}
}

func TestCancelPageNamesTheSessionAndChangesNothing_AC76(t *testing.T) {
	st := cancelStore()
	s, err := labsOf(st).CancelPage(context.Background(), staff, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Optics" || !s.StartsAt.Equal(at(13)) || s.Booked != 1 || cancelled(st, 1) {
		t.Fatalf("page = %+v, cancelled = %v", s, cancelled(st, 1))
	}
	if _, err := labsOf(st).CancelPage(context.Background(), staff, 99); err != service.ErrSessionNotFound {
		t.Fatalf("missing session: err = %v", err)
	}
}

func TestCancelIsForTheLabsOwnerOnly_AC79(t *testing.T) {
	for name, c := range map[string]service.Caller{"student": student, "another staff member": otherStaff, "nobody": {}} {
		st := cancelStore()
		want := service.ErrForbidden
		if c.ID == 0 {
			want = service.ErrUnauthenticated
		}
		if _, err := labsOf(st).CancelSession(context.Background(), c, 1); err != want {
			t.Errorf("%s: cancel err = %v, want %v", name, err, want)
		}
		if _, err := labsOf(st).CancelPage(context.Background(), c, 1); err != want {
			t.Errorf("%s: page err = %v, want %v", name, err, want)
		}
		if cancelled(st, 1) {
			t.Errorf("%s: the session was cancelled", name)
		}
	}
}

func TestDeleteLabNobodyBooked_AC77(t *testing.T) {
	st := cancelStore()
	l := labsOf(st)
	page, err := l.DeletePage(context.Background(), staff, 2)
	if err != nil || page.Lab.Title != "Acoustics" || len(page.Sessions) != 1 || st.LabCount() != 3 {
		t.Fatalf("page = %+v, err = %v, labs = %d", page, err, st.LabCount())
	}
	lab, err := l.DeleteLab(context.Background(), staff, 2)
	if err != nil || lab.Title != "Acoustics" {
		t.Fatalf("lab = %+v, err = %v", lab, err)
	}
	if st.LabCount() != 2 || st.SessionCount(2) != 0 {
		t.Fatalf("labs = %d, its sessions = %d; want it and its session gone", st.LabCount(), st.SessionCount(2))
	}
}

func TestDeleteLabWithBookingsIsRefused_AC77(t *testing.T) {
	st := cancelStore()
	l := labsOf(st)
	if _, err := l.DeletePage(context.Background(), staff, 1); err != service.ErrLabHasBookings {
		t.Errorf("page: err = %v, want ErrLabHasBookings", err)
	}
	if _, err := l.DeleteLab(context.Background(), staff, 1); err != service.ErrLabHasBookings {
		t.Errorf("delete: err = %v, want ErrLabHasBookings", err)
	}
	if st.LabCount() != 3 || st.SessionCount(1) != 3 {
		t.Fatal("something was deleted")
	}
	if got := service.ErrLabHasBookings.Error(); got != "This lab has bookings. Cancel its sessions instead." {
		t.Errorf("message = %q", got)
	}
}

func TestDeleteIsForTheLabsOwnerOnly_AC79(t *testing.T) {
	for name, c := range map[string]service.Caller{"student": student, "another staff member": otherStaff} {
		st := cancelStore()
		if _, err := labsOf(st).DeleteLab(context.Background(), c, 2); err != service.ErrForbidden {
			t.Errorf("%s: delete err = %v, want ErrForbidden", name, err)
		}
		if _, err := labsOf(st).DeletePage(context.Background(), c, 2); err != service.ErrForbidden {
			t.Errorf("%s: page err = %v, want ErrForbidden", name, err)
		}
		if st.LabCount() != 3 {
			t.Errorf("%s: a lab was deleted", name)
		}
	}
	if _, err := labsOf(cancelStore()).DeleteLab(context.Background(), staff, 99); err != service.ErrLabNotFound {
		t.Errorf("missing lab: err = %v", err)
	}
}

func TestStaffPageSaysWhatMayBeCancelledOrDeleted_AC75_AC77(t *testing.T) {
	st := cancelStore()
	l := labsOf(st)
	if _, err := l.CancelSession(context.Background(), staff, 3); err != nil {
		t.Fatal(err)
	}
	page, err := l.Bookings(context.Background(), staff, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Deletable {
		t.Error("lab 1 has a booking but is shown as deletable")
	}
	want := map[int64]bool{1: true, 2: false, 3: false} // upcoming; running; cancelled
	for _, s := range page.Upcoming {
		if s.Cancellable != want[s.SessionID] {
			t.Errorf("session %d: Cancellable = %v, want %v", s.SessionID, s.Cancellable, want[s.SessionID])
		}
	}
	empty, err := l.Bookings(context.Background(), staff, 2)
	if err != nil || !empty.Deletable {
		t.Fatalf("lab 2: %+v, %v; want deletable", empty, err)
	}
}

func TestCancelAndDeleteStoreErrors(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		set  func(*servicetest.Store)
		call func(*service.Labs) error
		want error
	}{
		"page lookup": {func(st *servicetest.Store) { st.ErrSessionByID = errDB },
			func(l *service.Labs) error { _, err := l.CancelPage(ctx, staff, 1); return err }, errDB},
		"cancel": {func(st *servicetest.Store) { st.ErrCancelSession = errDB },
			func(l *service.Labs) error { _, err := l.CancelSession(ctx, staff, 1); return err }, errDB},
		"delete page sessions": {func(st *servicetest.Store) { st.ErrSessionsOfLab = errDB },
			func(l *service.Labs) error { _, err := l.DeletePage(ctx, staff, 2); return err }, errDB},
		"delete page lookup": {func(st *servicetest.Store) { st.ErrLabByID = errDB },
			func(l *service.Labs) error { _, err := l.DeletePage(ctx, staff, 2); return err }, errDB},
		"delete": {func(st *servicetest.Store) { st.ErrDeleteLab = errDB },
			func(l *service.Labs) error { _, err := l.DeleteLab(ctx, staff, 2); return err }, errDB},
		"deleted meanwhile": {func(st *servicetest.Store) { st.ErrDeleteLab = store.ErrNotFound },
			func(l *service.Labs) error { _, err := l.DeleteLab(ctx, staff, 2); return err }, service.ErrLabNotFound},
		"booked meanwhile": {func(st *servicetest.Store) { st.ErrDeleteLab = store.ErrInUse },
			func(l *service.Labs) error { _, err := l.DeleteLab(ctx, staff, 2); return err }, service.ErrLabHasBookings},
	} {
		st := cancelStore()
		tc.set(st)
		if err := tc.call(labsOf(st)); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}
