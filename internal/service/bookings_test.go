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

// bookingStore has, on the fixed clock's day: lab 1 with free places, lab 2 full, lab 3 tomorrow, lab 4 yesterday.
func bookingStore() *servicetest.Store {
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: 7, Email: "sam@labwiz.test", Name: "Sam", Role: "student"})
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return day.Add(time.Duration(h) * time.Hour) }
	st.Labs = []store.Lab{
		{ID: 1, Title: "Optics", StartsAt: at(9), EndsAt: at(12), PlaceLimit: 3},
		{ID: 2, Title: "Circuits", StartsAt: at(13), EndsAt: at(17), PlaceLimit: 1},
		{ID: 3, Title: "Robotics", StartsAt: at(33), EndsAt: at(36), PlaceLimit: 8},
		{ID: 4, Title: "Yesterday", StartsAt: at(-10), EndsAt: at(-6), PlaceLimit: 8},
	}
	st.Bookers[2] = []store.Booker{{Name: "Ana"}}
	return st
}

func book(st *servicetest.Store, c service.Caller, labID int64) (service.LabCard, error) {
	return service.NewLabs(st, clock, time.UTC).Book(context.Background(), c, labID)
}

func TestBookTakesOnePlace_AC4(t *testing.T) {
	st := bookingStore()
	lab, err := book(st, student, 1)
	if err != nil {
		t.Fatal(err)
	}
	if lab.ID != 1 || lab.FreePlaces != 2 {
		t.Fatalf("lab = %+v, want lab 1 with 2 free places", lab)
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

func TestBookUnknownLabIsNotFound_AC47(t *testing.T) {
	if _, err := book(bookingStore(), student, 99); err != service.ErrLabNotFound {
		t.Fatalf("err = %v, want ErrLabNotFound", err)
	}
}

func TestBookLabNotAvailableTodayIsRefused_AC48(t *testing.T) {
	for _, id := range []int64{3, 4} {
		st := bookingStore()
		if _, err := book(st, student, id); err != service.ErrLabNotOpen {
			t.Errorf("lab %d: err = %v, want ErrLabNotOpen", id, err)
		}
		if n := len(st.Bookers[id]); n != 0 {
			t.Errorf("lab %d: bookings = %d, want 0", id, n)
		}
	}
}

func TestBookStoreErrors(t *testing.T) {
	st := bookingStore()
	st.ErrBookLab = store.ErrDuplicate // the unique constraint caught what the check did not
	if _, err := book(st, student, 1); err != service.ErrAlreadyBooked {
		t.Fatalf("duplicate: err = %v, want ErrAlreadyBooked", err)
	}
	st.ErrBookLab = errDB
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
	l := service.NewLabs(st, clock, time.UTC)
	lab, err := l.Get(context.Background(), student, 2)
	if err != nil {
		t.Fatal(err)
	}
	if lab.Title != "Circuits" || lab.FreePlaces != 0 {
		t.Fatalf("lab = %+v, want Circuits with 0 free places", lab)
	}
	if _, err := l.Get(context.Background(), student, 99); err != service.ErrLabNotFound {
		t.Fatalf("missing lab: err = %v, want ErrLabNotFound", err)
	}
	if _, err := l.Get(context.Background(), service.Caller{}, 1); err != service.ErrUnauthenticated {
		t.Fatalf("no caller: err = %v, want ErrUnauthenticated", err)
	}
	st.ErrLabByID = errDB
	if _, err := l.Get(context.Background(), student, 1); !errors.Is(err, errDB) {
		t.Fatalf("db: err = %v, want errDB wrapped", err)
	}
}
