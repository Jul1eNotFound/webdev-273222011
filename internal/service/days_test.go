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

// daysStore has sessions today (22 Oct, the clock says 10:00), tomorrow, and the day after tomorrow.
func daysStore() *servicetest.Store {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Optics", PlaceLimit: 4}, {ID: 2, Title: "Robotics", PlaceLimit: 8}}
	st.LabSessions = []servicetest.LabSession{sess(1, 1, 13, 15), sess(2, 2, 33, 36), sess(3, 1, 37, 39), sess(4, 2, 57, 59)}
	return st
}

func TestListDayShowsTheChosenDay_AC80(t *testing.T) {
	l := service.NewLabs(daysStore(), clock, time.UTC)
	list, err := l.ListDay(context.Background(), student, "2026-10-23")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Sessions) != 2 || list.Sessions[0].SessionID != 2 || list.Sessions[1].SessionID != 3 {
		t.Fatalf("sessions = %+v, want 2 and 3 (tomorrow)", list.Sessions)
	}
	if list.Today || list.Date != "2026-10-23" || list.Prev != "2026-10-22" || list.Next != "2026-10-24" {
		t.Errorf("list = %+v, want 23 Oct, not today, with 22 and 24 around it", list)
	}
	if !list.Day.Equal(time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Day = %v", list.Day)
	}
}

func TestListDayWithoutADateIsToday_AC80(t *testing.T) {
	l := service.NewLabs(daysStore(), clock, time.UTC)
	for _, date := range []string{"", "2026-10-22"} {
		list, err := l.ListDay(context.Background(), student, date)
		if err != nil {
			t.Fatal(err)
		}
		if !list.Today || list.Date != "2026-10-22" || len(list.Sessions) != 1 || list.Sessions[0].SessionID != 1 {
			t.Errorf("date %q: list = %+v, want today with session 1", date, list)
		}
	}
	today, err := l.ListToday(context.Background(), student)
	if err != nil || len(today) != 1 {
		t.Fatalf("ListToday = %+v, %v", today, err)
	}
}

func TestListDayRefusesAnInvalidDate_AC81(t *testing.T) {
	st := daysStore()
	l := service.NewLabs(st, clock, time.UTC)
	for _, date := range []string{"2026-13-01", "2026-02-30", "22.10.2026", "tomorrow", "2026-10-2", "2026-10-22T10:00", " 2026-10-22"} {
		if _, err := l.ListDay(context.Background(), student, date); err != service.ErrInvalidDate {
			t.Errorf("%q: err = %v, want ErrInvalidDate", date, err)
		}
	}
	if got := service.ErrInvalidDate.Error(); got != "Please choose a valid date." {
		t.Errorf("message = %q", got)
	}
}

func TestListDayKeepsADaylightSavingDayWhole_AC80(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Skipf("no time zone data: %v", err)
	}
	// Summer time ends on 25 Oct 2026 in Kyiv, so that day is 25 hours long.
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Late", PlaceLimit: 4}}
	late := time.Date(2026, 10, 25, 23, 30, 0, 0, kyiv)
	st.LabSessions = []servicetest.LabSession{{ID: 1, LabID: 1, StartsAt: late, EndsAt: late.Add(20 * time.Minute)}}
	l := service.NewLabs(st, clock, kyiv)
	on, err := l.ListDay(context.Background(), student, "2026-10-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(on.Sessions) != 1 {
		t.Errorf("25 Oct has %d sessions; the 23:30 session is missing from its own day", len(on.Sessions))
	}
	after, err := l.ListDay(context.Background(), student, "2026-10-26")
	if err != nil || len(after.Sessions) != 0 {
		t.Errorf("26 Oct: %+v, %v; want the 25 Oct session left out", after.Sessions, err)
	}
}

func TestListDayNeedsASignedInCallerAndWrapsStoreErrors_AC9(t *testing.T) {
	st := daysStore()
	l := service.NewLabs(st, clock, time.UTC)
	if _, err := l.ListDay(context.Background(), service.Caller{}, "2026-10-23"); err != service.ErrUnauthenticated {
		t.Errorf("nobody: err = %v", err)
	}
	st.ErrLabs = errDB
	if _, err := l.ListDay(context.Background(), student, "2026-10-23"); !errors.Is(err, errDB) {
		t.Errorf("store: err = %v, want errDB wrapped", err)
	}
}
