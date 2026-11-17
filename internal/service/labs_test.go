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
	st.Labs = []store.Lab{{ID: 1, Title: "Optics"}, {ID: 2, Title: "Circuits"}}
	st.LabSessions = []servicetest.LabSession{
		sess(1, 1, 9, 12),   // today
		sess(2, 2, -2, 1),   // since yesterday
		sess(3, 1, 33, 36),  // tomorrow
		sess(4, 2, -5, 0),   // ended at midnight
		sess(5, 2, 13, 15),  // today, a second lab
		sess(6, 1, 46, 48),  // later
		sess(7, 2, -20, -8), // yesterday
	}
	sessions, err := service.NewLabs(st, clock, time.UTC).ListToday(context.Background(), service.Caller{ID: 7})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, s := range sessions {
		ids = append(ids, s.SessionID)
	}
	if len(ids) != 3 || ids[0] != 2 || ids[1] != 1 || ids[2] != 5 {
		t.Fatalf("sessions = %v, want 2, 1, 5 in time order", ids)
	}
	if sessions[1].Title != "Optics" || sessions[1].LabID != 1 {
		t.Errorf("session 1 = %+v, want lab 1, Optics", sessions[1])
	}
}

func TestListTodayFreePlacesAreLimitMinusBookings_AC10(t *testing.T) {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{
		{ID: 1, Title: "None booked", PlaceLimit: 5}, {ID: 2, Title: "Some booked", PlaceLimit: 5},
		{ID: 3, Title: "Full", PlaceLimit: 2}, {ID: 4, Title: "Over-full", PlaceLimit: 1},
	}
	st.LabSessions = []servicetest.LabSession{sess(1, 1, 9, 12), sess(2, 2, 10, 12), sess(3, 3, 11, 12), sess(4, 4, 12, 14)}
	st.Bookers[2] = []store.Booker{{Name: "A"}, {Name: "B"}}
	st.Bookers[3] = []store.Booker{{Name: "A"}, {Name: "B"}}
	st.Bookers[4] = []store.Booker{{Name: "A"}, {Name: "B"}}
	sessions, err := service.NewLabs(st, clock, time.UTC).ListToday(context.Background(), service.Caller{ID: 7})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]int{1: 5, 2: 3, 3: 0, 4: 0}
	if len(sessions) != len(want) {
		t.Fatalf("got %d sessions, want %d", len(sessions), len(want))
	}
	for _, s := range sessions {
		if s.FreePlaces != want[s.SessionID] {
			t.Errorf("%s: free places = %d, want %d", s.Title, s.FreePlaces, want[s.SessionID])
		}
	}
}

func TestListTodayMarksStartedSessions_AC83(t *testing.T) {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Optics", PlaceLimit: 5}}
	st.LabSessions = []servicetest.LabSession{sess(1, 1, 8, 9), sess(2, 1, 10, 11), sess(3, 1, 12, 13)}
	sessions, err := service.NewLabs(st, clock, time.UTC).ListToday(context.Background(), service.Caller{ID: 7})
	if err != nil || len(sessions) != 3 {
		t.Fatalf("sessions = %+v, err = %v", sessions, err)
	}
	// The clock says 10:00: ended, starting right now, and later.
	for i, want := range []bool{true, true, false} {
		if sessions[i].Started != want {
			t.Errorf("session %d: Started = %v, want %v", sessions[i].SessionID, sessions[i].Started, want)
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

func validLab() service.NewLab {
	return service.NewLab{Title: "  Optics  ", Description: " Lenses and lasers. ", PlaceLimit: "12"}
}

func TestAddLabCreatesOneLab_AC12(t *testing.T) {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 4, Title: "Chemistry basics"}}
	lab, err := service.NewLabs(st, clock, time.UTC).Add(context.Background(), staff, validLab())
	if err != nil {
		t.Fatal(err)
	}
	want := store.Lab{ID: 5, Title: "Optics", Description: "Lenses and lasers.", PlaceLimit: 12, OwnerID: staff.ID}
	if lab != want {
		t.Fatalf("lab = %+v, want %+v", lab, want)
	}
	if n := st.LabCount(); n != 2 {
		t.Fatalf("labs = %d, want 2", n)
	}
	// A new lab has no sessions: its owner adds them (AC-67).
	if n := st.SessionCount(5); n != 0 {
		t.Fatalf("sessions = %d, want 0", n)
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

func TestAddLabValidatesTitle(t *testing.T) {
	for name, tc := range map[string]struct {
		title string
		want  error
	}{
		"blank title": {"   ", service.ErrTitleRequired},
		"long title":  {strings.Repeat("é", 201), service.ErrTitleTooLong},
	} {
		st := servicetest.NewStore()
		in := validLab()
		in.Title = tc.title
		if _, err := service.NewLabs(st, clock, time.UTC).Add(context.Background(), staff, in); err != tc.want {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
		if n := st.LabCount(); n != 0 {
			t.Errorf("%s: %d labs created", name, n)
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

// bookersStore has Stella's lab 1 with an upcoming session 1 (Ana, Ben), a later session 3 (Cleo),
// and yesterday's session 4 (Dan); and lab 2 with session 2 (Eve).
func bookersStore() *servicetest.Store {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Optics", PlaceLimit: 4, OwnerID: staff.ID}, {ID: 2, Title: "Circuits", PlaceLimit: 2, OwnerID: staff.ID}}
	st.LabSessions = []servicetest.LabSession{sess(1, 1, 13, 15), sess(2, 2, 13, 15), sess(3, 1, 37, 39), sess(4, 1, -14, -12)}
	st.Bookers[1] = []store.Booker{{Name: "Ana", Email: "ana@labwiz.test"}, {Name: "Ben", Email: "ben@labwiz.test"}}
	st.Bookers[2] = []store.Booker{{Name: "Eve"}}
	st.Bookers[3] = []store.Booker{{Name: "Cleo"}}
	st.Bookers[4] = []store.Booker{{Name: "Dan"}}
	return st
}

func TestLabBookingsListsExactlyTheBookers_AC14(t *testing.T) {
	page, err := service.NewLabs(bookersStore(), clock, time.UTC).Bookings(context.Background(), staff, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Lab.Title != "Optics" || len(page.Upcoming) != 2 || len(page.Past) != 1 {
		t.Fatalf("page = %+v, want Optics with 2 upcoming sessions and 1 past", page)
	}
	first, later, past := page.Upcoming[0], page.Upcoming[1], page.Past[0]
	if first.SessionID != 1 || len(first.Bookers) != 2 || first.Bookers[0].Name != "Ana" || first.Bookers[1].Name != "Ben" {
		t.Errorf("first session = %+v, want session 1 with Ana and Ben", first)
	}
	if later.SessionID != 3 || len(later.Bookers) != 1 || later.Bookers[0].Name != "Cleo" {
		t.Errorf("later session = %+v, want session 3 with Cleo", later)
	}
	if past.SessionID != 4 || len(past.Bookers) != 1 || past.Bookers[0].Name != "Dan" || first.FreePlaces != 2 {
		t.Errorf("past = %+v, first free places = %d", past, first.FreePlaces)
	}
}

func TestLabBookingsListsPastSessionsMostRecentFirst_AC14(t *testing.T) {
	st := bookersStore()
	st.LabSessions = append(st.LabSessions, sess(5, 1, -40, -38))
	page, err := service.NewLabs(st, clock, time.UTC).Bookings(context.Background(), staff, 1)
	if err != nil || len(page.Past) != 2 || page.Past[0].SessionID != 4 || page.Past[1].SessionID != 5 {
		t.Fatalf("past = %+v, err = %v; want session 4 then 5", page.Past, err)
	}
}

func TestLabBookingsStudentIsForbidden_AC45(t *testing.T) {
	l := service.NewLabs(bookersStore(), clock, time.UTC)
	for _, id := range []int64{1, 99} {
		page, err := l.Bookings(context.Background(), student, id)
		if err != service.ErrForbidden || page.Upcoming != nil {
			t.Errorf("lab %d: err = %v, page = %+v", id, err, page)
		}
	}
	if _, err := l.Bookings(context.Background(), service.Caller{}, 1); err != service.ErrUnauthenticated {
		t.Errorf("nobody: err = %v", err)
	}
}

func TestLabBookingsUnknownLabIsNotFound(t *testing.T) {
	_, err := service.NewLabs(servicetest.NewStore(), clock, time.UTC).Bookings(context.Background(), staff, 99)
	if err != service.ErrLabNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestLabBookingsStoreErrors(t *testing.T) {
	st := bookersStore()
	l := service.NewLabs(st, clock, time.UTC)
	st.ErrLabByID = errDB
	if _, err := l.Bookings(context.Background(), staff, 1); !errors.Is(err, errDB) {
		t.Errorf("lab lookup: err = %v", err)
	}
	st.ErrLabByID, st.ErrSessionsOfLab = nil, errDB
	if _, err := l.Bookings(context.Background(), staff, 1); !errors.Is(err, errDB) {
		t.Errorf("sessions: err = %v", err)
	}
	st.ErrSessionsOfLab, st.ErrSessionBookers = nil, errDB
	if _, err := l.Bookings(context.Background(), staff, 1); !errors.Is(err, errDB) {
		t.Errorf("bookers: err = %v", err)
	}
}

func TestLocationIsTheConfiguredTimezone(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*60*60)
	if got := service.NewLabs(servicetest.NewStore(), clock, loc).Location(); got != loc {
		t.Fatalf("Location() = %v", got)
	}
}
