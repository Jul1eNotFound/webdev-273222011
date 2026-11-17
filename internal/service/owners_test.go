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

// ownedStore has lab 1 and 2 owned by staff (Stella, id 2) and lab 3 owned by otherStaff (Omar, id 3),
// each with one session whose id is the lab's; Robotics' session is tomorrow. Lab 4, also Stella's,
// has no session.
func ownedStore() *servicetest.Store {
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: staff.ID, Email: "stella@labwiz.test", Name: "Stella Staff", Role: "staff"})
	st.AddUser(store.User{ID: otherStaff.ID, Email: "omar@labwiz.test", Name: "Omar Haddad", Role: "staff"})
	st.Labs = []store.Lab{
		{ID: 1, Title: "Optics", PlaceLimit: 4, OwnerID: staff.ID},
		{ID: 2, Title: "Robotics", PlaceLimit: 8, OwnerID: staff.ID},
		{ID: 3, Title: "Circuits", PlaceLimit: 2, OwnerID: otherStaff.ID},
		{ID: 4, Title: "Acoustics", PlaceLimit: 6, OwnerID: staff.ID},
	}
	st.LabSessions = []servicetest.LabSession{sess(1, 1, 9, 11), sess(2, 2, 33, 36), sess(3, 3, 13, 15)}
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
	page, err := l.Get(context.Background(), student, 3)
	if err != nil || page.Lab.OwnerName != "Omar Haddad" || page.Sessions[0].OwnerName != "Omar Haddad" {
		t.Fatalf("Get: page = %+v, err = %v", page, err)
	}
}

func TestMyLabsListsOnlyTheCallersLabs_AC62(t *testing.T) {
	l := service.NewLabs(ownedStore(), clock, time.UTC)
	labs, err := l.MyLabs(context.Background(), staff)
	if err != nil {
		t.Fatal(err)
	}
	// Robotics' session is tomorrow and Acoustics has none: My labs is not limited to today.
	// Labs with a next session come first, soonest first.
	if len(labs) != 3 || labs[0].Title != "Optics" || labs[1].Title != "Robotics" || labs[2].Title != "Acoustics" {
		t.Fatalf("labs = %+v, want Optics, Robotics, Acoustics", labs)
	}
	if labs[0].Next == nil || !labs[0].Next.StartsAt.Equal(at(9)) || labs[0].Sessions != 1 || labs[0].OwnerName != "Stella Staff" {
		t.Errorf("Optics = %+v, want its running session next and owner Stella Staff", labs[0])
	}
	if labs[2].Next != nil || labs[2].Sessions != 0 {
		t.Errorf("Acoustics = %+v, want no sessions", labs[2])
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
	page, err := l.Bookings(context.Background(), otherStaff, 1)
	if err != service.ErrForbidden || page.Upcoming != nil || page.Past != nil {
		t.Fatalf("another staff member: err = %v, page = %+v; want ErrForbidden and nothing", err, page)
	}
	page, err = l.Bookings(context.Background(), staff, 1)
	if err != nil || len(page.Upcoming) != 1 || len(page.Upcoming[0].Bookers) != 1 {
		t.Fatalf("owner: page = %+v, err = %v; want Ana", page, err)
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
