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

// bookingStore has, on the fixed clock's day (now 10:00):
//   - session 1 of Optics (lab 1, 3 places), 13:00–15:00, with free places;
//   - session 2 of Circuits (lab 2, 1 place), 13:00–17:00, full (Ana);
//   - session 3 of Robotics (lab 3, 8 places), tomorrow;
//   - session 4 of Microscopy (lab 4, 8 places), 09:00–12:00, running;
//   - session 5 of Optics, yesterday, over.
func bookingStore() *servicetest.Store {
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: 7, Email: "sam@labwiz.test", Name: "Sam", Role: "student"})
	st.Labs = []store.Lab{
		{ID: 1, Title: "Optics", PlaceLimit: 3}, {ID: 2, Title: "Circuits", PlaceLimit: 1},
		{ID: 3, Title: "Robotics", PlaceLimit: 8}, {ID: 4, Title: "Microscopy", PlaceLimit: 8},
	}
	st.LabSessions = []servicetest.LabSession{
		sess(1, 1, 13, 15), sess(2, 2, 13, 17), sess(3, 3, 33, 36), sess(4, 4, 9, 12), sess(5, 1, -14, -12),
	}
	st.Bookers[2] = []store.Booker{{Name: "Ana"}}
	return st
}

func book(st *servicetest.Store, c service.Caller, sessionID int64) (service.SessionCard, error) {
	return service.NewLabs(st, clock, time.UTC).Book(context.Background(), c, sessionID)
}

func TestBookTakesOnePlace_AC4(t *testing.T) {
	st := bookingStore()
	s, err := book(st, student, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s.SessionID != 1 || s.LabID != 1 || s.FreePlaces != 2 || s.Title != "Optics" {
		t.Fatalf("session = %+v, want session 1 of Optics with 2 free places", s)
	}
	if got := st.Bookers[1]; len(got) != 1 || got[0].Name != "Sam" {
		t.Fatalf("bookers = %+v, want only Sam", got)
	}
}

func TestBookTwiceIsAlreadyBooked_AC5(t *testing.T) {
	st := bookingStore()
	if _, err := book(st, student, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := book(st, student, 1); err != service.ErrAlreadyBooked {
		t.Fatalf("err = %v, want ErrAlreadyBooked", err)
	}
	if n := len(st.Bookers[1]); n != 1 {
		t.Fatalf("bookings = %d, want 1", n)
	}
}

func TestBookFullLabIsRefused_AC1(t *testing.T) {
	st := bookingStore()
	if _, err := book(st, student, 2); err != service.ErrLabFull {
		t.Fatalf("err = %v, want ErrLabFull", err)
	}
	if n := len(st.Bookers[2]); n != 1 {
		t.Fatalf("bookings = %d, want 1", n)
	}
}

func TestBookTheLastPlaceThenFull_AC6(t *testing.T) {
	st := bookingStore()
	st.AddUser(store.User{ID: 8, Email: "zoe@labwiz.test", Name: "Zoe", Role: "student"})
	st.Labs[0].PlaceLimit = 1
	if _, err := book(st, student, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := book(st, service.Caller{ID: 8, Role: service.RoleStudent}, 1); err != service.ErrLabFull {
		t.Fatalf("err = %v, want ErrLabFull", err)
	}
}

func TestBookByStaffIsForbidden_AC46(t *testing.T) {
	st := bookingStore()
	if _, err := book(st, staff, 1); err != service.ErrForbidden {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
	if n := len(st.Bookers[1]); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestBookWithoutCallerIsUnauthenticated_AC19(t *testing.T) {
	if _, err := book(bookingStore(), service.Caller{}, 1); err != service.ErrUnauthenticated {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
}

func TestBookUnknownSessionIsNotFound_AC47(t *testing.T) {
	if _, err := book(bookingStore(), student, 99); err != service.ErrSessionNotFound {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestBookSessionThatStartedIsRefused_AC83(t *testing.T) {
	st := bookingStore()
	// Running (4), over (5), and one starting exactly now.
	st.LabSessions = append(st.LabSessions, sess(6, 3, 10, 11))
	for _, id := range []int64{4, 5, 6} {
		if _, err := book(st, student, id); err != service.ErrSessionStarted {
			t.Errorf("session %d: err = %v, want ErrSessionStarted", id, err)
		}
		if n := len(st.Bookers[id]); n != 0 {
			t.Errorf("session %d: bookings = %d, want 0", id, n)
		}
	}
	if got := service.ErrSessionStarted.Error(); got != "Booking is closed: this session has already started." {
		t.Errorf("message = %q", got)
	}
}

func TestBookSessionTomorrowSucceeds_AC84(t *testing.T) {
	st := bookingStore()
	s, err := book(st, student, 3)
	if err != nil || s.FreePlaces != 7 || len(st.Bookers[3]) != 1 {
		t.Fatalf("session = %+v, err = %v, bookings = %d; want 7 free places and 1 booking", s, err, len(st.Bookers[3]))
	}
}

func TestBookTwoSessionsOfOneLabCountSeparately_AC71(t *testing.T) {
	st := bookingStore()
	st.LabSessions = append(st.LabSessions, sess(7, 1, 37, 39)) // Optics again, tomorrow
	first, err := book(st, student, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := book(st, student, 7)
	if err != nil {
		t.Fatalf("second session of the same lab: %v", err)
	}
	if first.FreePlaces != 2 || second.FreePlaces != 2 || len(st.Bookers[1]) != 1 || len(st.Bookers[7]) != 1 {
		t.Fatalf("free places %d and %d, bookings %d and %d; want 2, 2, 1, 1",
			first.FreePlaces, second.FreePlaces, len(st.Bookers[1]), len(st.Bookers[7]))
	}
}

func TestBookStoreErrors(t *testing.T) {
	st := bookingStore()
	st.ErrBookSession = store.ErrDuplicate // the unique constraint caught what the check did not
	if _, err := book(st, student, 1); err != service.ErrAlreadyBooked {
		t.Fatalf("duplicate: err = %v, want ErrAlreadyBooked", err)
	}
	st.ErrBookSession = errDB
	if _, err := book(st, student, 1); !errors.Is(err, errDB) {
		t.Fatalf("db: err = %v, want errDB wrapped", err)
	}
}

func TestRequireStudent_AC46(t *testing.T) {
	for _, tc := range []struct {
		c    service.Caller
		want error
	}{
		{service.Caller{}, service.ErrUnauthenticated},
		{staff, service.ErrForbidden},
		{student, nil},
	} {
		if err := service.RequireStudent(tc.c); err != tc.want {
			t.Errorf("RequireStudent(%+v) = %v, want %v", tc.c, err, tc.want)
		}
	}
}

func TestGetLabShowsFreePlaces_AC47(t *testing.T) {
	st := bookingStore()
	st.LabSessions = append(st.LabSessions, sess(7, 1, 37, 39))
	l := service.NewLabs(st, clock, time.UTC)
	page, err := l.Get(context.Background(), student, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Yesterday's session 5 is left out; 1 and 7 remain, in time order.
	if page.Lab.Title != "Optics" || len(page.Sessions) != 2 || page.Sessions[0].SessionID != 1 || page.Sessions[1].SessionID != 7 {
		t.Fatalf("page = %+v", page)
	}
	if page.Sessions[0].FreePlaces != 3 || page.Sessions[0].Started {
		t.Errorf("session 1 = %+v, want 3 free places, not started", page.Sessions[0])
	}
	running, err := l.Get(context.Background(), student, 4)
	if err != nil || len(running.Sessions) != 1 || !running.Sessions[0].Started {
		t.Fatalf("running session: %+v, %v; want it listed as started", running, err)
	}
	if _, err := l.Get(context.Background(), student, 99); err != service.ErrLabNotFound {
		t.Fatalf("missing lab: err = %v, want ErrLabNotFound", err)
	}
	if _, err := l.Get(context.Background(), service.Caller{}, 1); err != service.ErrUnauthenticated {
		t.Fatalf("no caller: err = %v, want ErrUnauthenticated", err)
	}
	st.ErrSessionsOfLab = errDB
	if _, err := l.Get(context.Background(), student, 1); !errors.Is(err, errDB) {
		t.Fatalf("sessions: err = %v, want errDB wrapped", err)
	}
	st.ErrSessionsOfLab, st.ErrLabByID = nil, errDB
	if _, err := l.Get(context.Background(), student, 1); !errors.Is(err, errDB) {
		t.Fatalf("lab: err = %v, want errDB wrapped", err)
	}
}
