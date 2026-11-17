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

// otherStaff is a second staff member, who owns none of staff's labs.
var otherStaff = service.Caller{ID: 3, Name: "Omar", Role: service.RoleStaff}

// ownedStore has lab 1 and 2 owned by staff (Stella, id 2) and lab 3 owned by otherStaff (Omar, id 3).
func ownedStore() *servicetest.Store {
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: staff.ID, Email: "stella@labwiz.test", Name: "Stella Staff", Role: "staff"})
	st.AddUser(store.User{ID: otherStaff.ID, Email: "omar@labwiz.test", Name: "Omar Haddad", Role: "staff"})
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	st.Labs = []store.Lab{
		{ID: 1, Title: "Optics", StartsAt: day.Add(9 * time.Hour), EndsAt: day.Add(11 * time.Hour), PlaceLimit: 4, OwnerID: staff.ID},
		{ID: 2, Title: "Robotics", StartsAt: day.Add(33 * time.Hour), EndsAt: day.Add(36 * time.Hour), PlaceLimit: 8, OwnerID: staff.ID},
		{ID: 3, Title: "Circuits", StartsAt: day.Add(13 * time.Hour), EndsAt: day.Add(15 * time.Hour), PlaceLimit: 2, OwnerID: otherStaff.ID},
	}
	st.Bookers[1] = []store.Booker{{Name: "Ana"}}
	return st
}

func TestAddLabStoresTheCallerAsOwner_AC60(t *testing.T) {
	st := servicetest.NewStore()
	lab, err := service.NewLabs(st, clock, time.UTC).Add(context.Background(), otherStaff, validLab())
	if err != nil {
		t.Fatal(err)
	}
	if lab.OwnerID != otherStaff.ID || st.Labs[0].OwnerID != otherStaff.ID {
		t.Fatalf("owner = %d (stored %d), want %d", lab.OwnerID, st.Labs[0].OwnerID, otherStaff.ID)
	}
}

func TestLabsShowTheirOwnersName_AC61(t *testing.T) {
	l := service.NewLabs(ownedStore(), clock, time.UTC)
	today, err := l.ListToday(context.Background(), student)
	if err != nil {
		t.Fatal(err)
	}
	owners := map[string]string{}
	for _, c := range today {
		owners[c.Title] = c.OwnerName
	}
	if owners["Optics"] != "Stella Staff" || owners["Circuits"] != "Omar Haddad" {
		t.Fatalf("owners = %v", owners)
	}
	lab, err := l.Get(context.Background(), student, 3)
	if err != nil || lab.OwnerName != "Omar Haddad" {
		t.Fatalf("Get: owner = %q, err = %v", lab.OwnerName, err)
	}
}

func TestMyLabsListsOnlyTheCallersLabs_AC62(t *testing.T) {
	l := service.NewLabs(ownedStore(), clock, time.UTC)
	labs, err := l.MyLabs(context.Background(), staff)
	if err != nil {
		t.Fatal(err)
	}
	// Robotics starts tomorrow: My labs is not limited to today.
	if len(labs) != 2 || labs[0].Title != "Optics" || labs[1].Title != "Robotics" {
		t.Fatalf("labs = %+v, want Optics and Robotics", labs)
	}
	if labs[0].FreePlaces != 3 || labs[0].OwnerName != "Stella Staff" {
		t.Errorf("Optics = %+v, want 3 free places and owner Stella Staff", labs[0])
	}
	other, err := l.MyLabs(context.Background(), otherStaff)
	if err != nil || len(other) != 1 || other[0].Title != "Circuits" {
		t.Fatalf("Omar's labs = %+v, err = %v; want only Circuits", other, err)
	}
}

func TestMyLabsIsEmptyForStaffWhoOwnNothing_AC63(t *testing.T) {
	labs, err := service.NewLabs(ownedStore(), clock, time.UTC).MyLabs(context.Background(), service.Caller{ID: 9, Role: service.RoleStaff})
	if err != nil || len(labs) != 0 {
		t.Fatalf("labs = %+v, err = %v; want none", labs, err)
	}
}

func TestMyLabsIsForStaffOnly_AC65(t *testing.T) {
	l := service.NewLabs(ownedStore(), clock, time.UTC)
	if _, err := l.MyLabs(context.Background(), student); err != service.ErrForbidden {
		t.Errorf("student: err = %v, want ErrForbidden", err)
	}
	if _, err := l.MyLabs(context.Background(), service.Caller{}); err != service.ErrUnauthenticated {
		t.Errorf("nobody: err = %v, want ErrUnauthenticated", err)
	}
}

func TestMyLabsStoreError(t *testing.T) {
	st := ownedStore()
	st.ErrLabsOwnedBy = errDB
	if _, err := service.NewLabs(st, clock, time.UTC).MyLabs(context.Background(), staff); !errors.Is(err, errDB) {
		t.Fatalf("err = %v, want errDB wrapped", err)
	}
}

func TestLabBookingsAreForTheOwnerOnly_AC64(t *testing.T) {
	l := service.NewLabs(ownedStore(), clock, time.UTC)
	_, bookers, err := l.Bookings(context.Background(), otherStaff, 1)
	if err != service.ErrForbidden || bookers != nil {
		t.Fatalf("another staff member: err = %v, bookers = %+v; want ErrForbidden and none", err, bookers)
	}
	if _, bookers, err := l.Bookings(context.Background(), staff, 1); err != nil || len(bookers) != 1 {
		t.Fatalf("owner: bookers = %+v, err = %v; want Ana", bookers, err)
	}
}

func TestRequireOwner_AC64(t *testing.T) {
	lab := store.Lab{ID: 1, OwnerID: staff.ID}
	for _, tc := range []struct {
		c    service.Caller
		want error
	}{
		{service.Caller{}, service.ErrUnauthenticated},
		{student, service.ErrForbidden},
		{service.Caller{ID: staff.ID, Role: service.RoleStudent}, service.ErrForbidden}, // same id, not staff
		{otherStaff, service.ErrForbidden},
		{staff, nil},
	} {
		if err := service.RequireOwner(tc.c, lab); err != tc.want {
			t.Errorf("RequireOwner(%+v) = %v, want %v", tc.c, err, tc.want)
		}
	}
}

func TestNewLabFormIsForStaffOnly_AC15(t *testing.T) {
	l := service.NewLabs(servicetest.NewStore(), clock, time.UTC)
	if err := l.NewLabForm(staff); err != nil {
		t.Errorf("staff: err = %v", err)
	}
	if err := l.NewLabForm(student); err != service.ErrForbidden {
		t.Errorf("student: err = %v, want ErrForbidden", err)
	}
}
