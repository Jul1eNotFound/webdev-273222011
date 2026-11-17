package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

func TestListTodayReturnsOnlyLabsOverlappingToday_AC3(t *testing.T) {
	st := servicetest.NewStore()
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	st.Labs = []store.Lab{
		{ID: 1, Title: "Today", StartsAt: day.Add(9 * time.Hour), EndsAt: day.Add(12 * time.Hour)},
		{ID: 2, Title: "Since yesterday", StartsAt: day.Add(-2 * time.Hour), EndsAt: day.Add(time.Hour)},
		{ID: 3, Title: "Tomorrow", StartsAt: day.Add(33 * time.Hour), EndsAt: day.Add(36 * time.Hour)},
		{ID: 4, Title: "Yesterday", StartsAt: day.Add(-5 * time.Hour), EndsAt: day},
	}
	l := service.NewLabs(st, clock, time.UTC)
	labs, err := l.ListToday(context.Background(), service.Caller{ID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(labs) != 2 || labs[0].ID != 1 || labs[1].ID != 2 {
		t.Fatalf("labs = %+v, want ids 1 and 2", labs)
	}
}

func TestListTodayFreePlacesAreLimitMinusBookings_AC10(t *testing.T) {
	st := servicetest.NewStore()
	day := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return day.Add(time.Duration(h) * time.Hour) }
	st.Labs = []store.Lab{
		{ID: 1, Title: "None booked", StartsAt: at(9), EndsAt: at(12), PlaceLimit: 5},
		{ID: 2, Title: "Some booked", StartsAt: at(10), EndsAt: at(12), PlaceLimit: 5},
		{ID: 3, Title: "Full", StartsAt: at(11), EndsAt: at(12), PlaceLimit: 2},
		{ID: 4, Title: "Over-full", StartsAt: at(12), EndsAt: at(14), PlaceLimit: 1},
	}
	st.Bookers[2] = []store.Booker{{Name: "A"}, {Name: "B"}}
	st.Bookers[3] = []store.Booker{{Name: "A"}, {Name: "B"}}
	st.Bookers[4] = []store.Booker{{Name: "A"}, {Name: "B"}}
	labs, err := service.NewLabs(st, clock, time.UTC).ListToday(context.Background(), service.Caller{ID: 7})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]int{1: 5, 2: 3, 3: 0, 4: 0}
	if len(labs) != len(want) {
		t.Fatalf("got %d labs, want %d", len(labs), len(want))
	}
	for _, l := range labs {
		if l.FreePlaces != want[l.ID] {
			t.Errorf("%s: free places = %d, want %d", l.Title, l.FreePlaces, want[l.ID])
		}
	}
}

func TestListTodayNeedsASignedInCaller_AC9(t *testing.T) {
	l := service.NewLabs(servicetest.NewStore(), clock, time.UTC)
	if _, err := l.ListToday(context.Background(), service.Caller{}); err != service.ErrUnauthenticated {
		t.Fatalf("err = %v", err)
	}
}

func TestListTodayStoreError(t *testing.T) {
	st := servicetest.NewStore()
	st.ErrLabs = errDB
	l := service.NewLabs(st, clock, time.UTC)
	if _, err := l.ListToday(context.Background(), service.Caller{ID: 7}); !errors.Is(err, errDB) {
		t.Fatalf("err = %v", err)
	}
}

var (
	staff   = service.Caller{ID: 2, Name: "Stella", Role: service.RoleStaff}
	student = service.Caller{ID: 7, Name: "Sam", Role: service.RoleStudent}
)

func validLab() service.NewLab {
	return service.NewLab{
		Title: "  Optics  ", Description: " Lenses and lasers. ",
		Starts: "2026-10-23T09:00", Ends: "2026-10-23T11:30", PlaceLimit: "12",
	}
}

func TestAddLabCreatesOneLab_AC12(t *testing.T) {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 4, Title: "Chemistry basics"}}
	loc := time.FixedZone("UTC+2", 2*60*60)
	l := service.NewLabs(st, clock, loc)

	lab, err := l.Add(context.Background(), staff, validLab())
	if err != nil {
		t.Fatal(err)
	}
	want := store.Lab{
		ID: 5, Title: "Optics", Description: "Lenses and lasers.",
		StartsAt: time.Date(2026, 10, 23, 9, 0, 0, 0, loc), EndsAt: time.Date(2026, 10, 23, 11, 30, 0, 0, loc), PlaceLimit: 12,
		OwnerID: staff.ID, // AC-60
	}
	if lab != want {
		t.Fatalf("lab = %+v, want %+v", lab, want)
	}
	if n := st.LabCount(); n != 2 {
		t.Fatalf("labs = %d, want 2", n)
	}
}

func TestAddLabRejectsBadPlaceLimit_AC13(t *testing.T) {
	for _, limit := range []string{"0", "-1", "abc", "", "1.5", "2147483648"} {
		st := servicetest.NewStore()
		in := validLab()
		in.PlaceLimit = limit
		_, err := service.NewLabs(st, clock, time.UTC).Add(context.Background(), staff, in)
		if err != service.ErrInvalidPlaceLimit {
			t.Errorf("place limit %q: err = %v, want ErrInvalidPlaceLimit", limit, err)
		}
		if n := st.LabCount(); n != 0 {
			t.Errorf("place limit %q: %d labs created", limit, n)
		}
	}
}

func TestAddLabAcceptsAPlaceLimitOfOne_AC12(t *testing.T) {
	in := validLab()
	in.PlaceLimit = "1"
	lab, err := service.NewLabs(servicetest.NewStore(), clock, time.UTC).Add(context.Background(), staff, in)
	if err != nil || lab.PlaceLimit != 1 {
		t.Fatalf("lab = %+v, err = %v", lab, err)
	}
}

func TestAddLabValidatesTitleAndPeriod(t *testing.T) {
	cases := []struct {
		name string
		edit func(*service.NewLab)
		want error
	}{
		{"blank title", func(in *service.NewLab) { in.Title = "   " }, service.ErrTitleRequired},
		{"long title", func(in *service.NewLab) { in.Title = strings.Repeat("é", 201) }, service.ErrTitleTooLong},
		{"bad start", func(in *service.NewLab) { in.Starts = "tomorrow" }, service.ErrInvalidPeriod},
		{"missing end", func(in *service.NewLab) { in.Ends = "" }, service.ErrInvalidPeriod},
		{"end before start", func(in *service.NewLab) { in.Ends = "2026-10-23T08:00" }, service.ErrPeriodOrder},
		{"end equals start", func(in *service.NewLab) { in.Ends = in.Starts }, service.ErrPeriodOrder},
	}
	for _, tc := range cases {
		st := servicetest.NewStore()
		in := validLab()
		tc.edit(&in)
		_, err := service.NewLabs(st, clock, time.UTC).Add(context.Background(), staff, in)
		if err != tc.want {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if n := st.LabCount(); n != 0 {
			t.Errorf("%s: %d labs created", tc.name, n)
		}
	}
	in := validLab()
	in.Title = strings.Repeat("é", 200)
	if _, err := service.NewLabs(servicetest.NewStore(), clock, time.UTC).Add(context.Background(), staff, in); err != nil {
		t.Errorf("200-character title: err = %v", err)
	}
}

func TestAddLabDuplicateTitleIsConflict_AC2(t *testing.T) {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Optics"}}
	_, err := service.NewLabs(st, clock, time.UTC).Add(context.Background(), staff, validLab())
	if err != service.ErrDuplicateTitle {
		t.Fatalf("err = %v, want ErrDuplicateTitle", err)
	}
	if !strings.Contains(err.Error(), "Please choose a different title") {
		t.Fatalf("message = %q", err.Error())
	}
	if n := st.LabCount(); n != 1 {
		t.Fatalf("labs = %d, want 1", n)
	}
}

func TestAddLabStudentIsForbidden_AC15(t *testing.T) {
	st := servicetest.NewStore()
	l := service.NewLabs(st, clock, time.UTC)
	if _, err := l.Add(context.Background(), student, validLab()); err != service.ErrForbidden {
		t.Fatalf("student: err = %v, want ErrForbidden", err)
	}
	if _, err := l.Add(context.Background(), service.Caller{}, validLab()); err != service.ErrUnauthenticated {
		t.Fatalf("nobody: err = %v, want ErrUnauthenticated", err)
	}
	if n := st.LabCount(); n != 0 {
		t.Fatalf("labs = %d, want 0", n)
	}
}

func TestRequireStaff_AC15(t *testing.T) {
	if err := service.RequireStaff(staff); err != nil {
		t.Errorf("staff: %v", err)
	}
	if err := service.RequireStaff(student); err != service.ErrForbidden {
		t.Errorf("student: %v", err)
	}
	if err := service.RequireStaff(service.Caller{Role: service.RoleStaff}); err != service.ErrUnauthenticated {
		t.Errorf("no id: %v", err)
	}
}

func TestAddLabStoreError(t *testing.T) {
	st := servicetest.NewStore()
	st.ErrCreateLab = errDB
	if _, err := service.NewLabs(st, clock, time.UTC).Add(context.Background(), staff, validLab()); !errors.Is(err, errDB) {
		t.Fatalf("err = %v", err)
	}
}

func TestLabBookingsListsExactlyTheBookers_AC14(t *testing.T) {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Optics", OwnerID: staff.ID}, {ID: 2, Title: "Circuits", OwnerID: staff.ID}}
	st.Bookers[1] = []store.Booker{{Name: "Ana", Email: "ana@labwiz.test"}, {Name: "Ben", Email: "ben@labwiz.test"}}
	st.Bookers[2] = []store.Booker{{Name: "Cleo", Email: "cleo@labwiz.test"}}

	lab, bookers, err := service.NewLabs(st, clock, time.UTC).Bookings(context.Background(), staff, 1)
	if err != nil {
		t.Fatal(err)
	}
	if lab.Title != "Optics" || len(bookers) != 2 || bookers[0].Name != "Ana" || bookers[1].Name != "Ben" {
		t.Fatalf("lab = %+v, bookers = %+v", lab, bookers)
	}
}

func TestLabBookingsStudentIsForbidden_AC45(t *testing.T) {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Optics"}}
	st.Bookers[1] = []store.Booker{{Name: "Ana"}}
	l := service.NewLabs(st, clock, time.UTC)
	for _, id := range []int64{1, 99} {
		_, bookers, err := l.Bookings(context.Background(), student, id)
		if err != service.ErrForbidden || bookers != nil {
			t.Errorf("lab %d: err = %v, bookers = %+v", id, err, bookers)
		}
	}
	if _, _, err := l.Bookings(context.Background(), service.Caller{}, 1); err != service.ErrUnauthenticated {
		t.Errorf("nobody: err = %v", err)
	}
}

func TestLabBookingsUnknownLabIsNotFound(t *testing.T) {
	_, _, err := service.NewLabs(servicetest.NewStore(), clock, time.UTC).Bookings(context.Background(), staff, 99)
	if err != service.ErrLabNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestLabBookingsStoreErrors(t *testing.T) {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Optics", OwnerID: staff.ID}}
	l := service.NewLabs(st, clock, time.UTC)

	st.ErrLabByID = errDB
	if _, _, err := l.Bookings(context.Background(), staff, 1); !errors.Is(err, errDB) {
		t.Errorf("lab lookup: err = %v", err)
	}
	st.ErrLabByID, st.ErrLabBookers = nil, errDB
	if _, _, err := l.Bookings(context.Background(), staff, 1); !errors.Is(err, errDB) {
		t.Errorf("bookers: err = %v", err)
	}
}

func TestLocationIsTheConfiguredTimezone(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*60*60)
	if got := service.NewLabs(servicetest.NewStore(), clock, loc).Location(); got != loc {
		t.Fatalf("Location() = %v", got)
	}
}
