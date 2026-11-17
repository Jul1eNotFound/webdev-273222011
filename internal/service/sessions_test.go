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

// scheduleStore has Stella's lab 1 with one session tomorrow 09:00–11:00, and Omar's lab 3.
func scheduleStore() *servicetest.Store {
	st := servicetest.NewStore()
	st.Labs = []store.Lab{{ID: 1, Title: "Optics", PlaceLimit: 4, OwnerID: staff.ID}, {ID: 3, Title: "Circuits", PlaceLimit: 2, OwnerID: otherStaff.ID}}
	st.LabSessions = []servicetest.LabSession{sess(1, 1, 33, 35)}
	return st
}

func schedule(st *servicetest.Store, c service.Caller, labID int64, in service.NewSession) (int, error) {
	return service.NewLabs(st, clock, time.UTC).AddSessions(context.Background(), c, labID, in)
}

func TestAddSessionAddsOneSession_AC67(t *testing.T) {
	st := scheduleStore()
	n, err := schedule(st, staff, 1, service.NewSession{Starts: " 2026-10-24T13:00 ", Ends: "2026-10-24T15:30"})
	if err != nil || n != 1 {
		t.Fatalf("n = %d, err = %v; want 1 session", n, err)
	}
	added := st.LabSessions[1]
	if added.LabID != 1 || !added.StartsAt.Equal(at(61)) || !added.EndsAt.Equal(at(63).Add(30*time.Minute)) {
		t.Fatalf("session = %+v, want lab 1 on 24 Oct 13:00–15:30", added)
	}
}

func TestAddSessionMayRunPastMidnight_AC67(t *testing.T) {
	st := scheduleStore()
	if _, err := schedule(st, staff, 1, service.NewSession{Starts: "2026-10-22T21:00", Ends: "2026-10-23T01:00"}); err != nil {
		t.Fatalf("21:00 to 01:00 the next day: %v", err)
	}
}

func TestAddSessionRejectsABadForm_AC68(t *testing.T) {
	for name, tc := range map[string]struct {
		in   service.NewSession
		want error
	}{
		"unreadable start":    {service.NewSession{Starts: "tomorrow", Ends: "2026-10-24T15:00"}, service.ErrInvalidPeriod},
		"missing end":         {service.NewSession{Starts: "2026-10-24T13:00"}, service.ErrInvalidPeriod},
		"end before start":    {service.NewSession{Starts: "2026-10-24T13:00", Ends: "2026-10-24T12:00"}, service.ErrPeriodOrder},
		"end equals start":    {service.NewSession{Starts: "2026-10-24T13:00", Ends: "2026-10-24T13:00"}, service.ErrPeriodOrder},
		"starts in the past":  {service.NewSession{Starts: "2026-10-22T09:00", Ends: "2026-10-22T12:00"}, service.ErrSessionInPast},
		"starts right now":    {service.NewSession{Starts: "2026-10-22T10:00", Ends: "2026-10-22T12:00"}, service.ErrSessionInPast},
		"unreadable repeat":   {service.NewSession{Starts: "2026-10-24T13:00", Ends: "2026-10-24T15:00", RepeatUntil: "soon"}, service.ErrInvalidRepeat},
		"repeat before start": {service.NewSession{Starts: "2026-10-24T13:00", Ends: "2026-10-24T15:00", RepeatUntil: "2026-10-23"}, service.ErrRepeatBeforeStart},
		"repeat for 27 weeks": {service.NewSession{Starts: "2026-10-24T13:00", Ends: "2026-10-24T15:00", RepeatUntil: "2027-04-24"}, service.ErrRepeatTooLong},
	} {
		st := scheduleStore()
		if _, err := schedule(st, staff, 1, tc.in); err != tc.want {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
		if n := st.SessionCount(1); n != 1 {
			t.Errorf("%s: %d sessions, want only the 1 that was there", name, n)
		}
	}
}

func TestAddSessionOverlappingAnotherIsConflict_AC69(t *testing.T) {
	st := scheduleStore()
	_, err := schedule(st, staff, 1, service.NewSession{Starts: "2026-10-23T10:00", Ends: "2026-10-23T12:00"})
	if err != service.ErrSessionOverlap || err.Error() != "This lab already has a session at that time." {
		t.Fatalf("err = %v, want ErrSessionOverlap", err)
	}
	if n := st.SessionCount(1); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
	// Right after the existing one is fine.
	if _, err := schedule(st, staff, 1, service.NewSession{Starts: "2026-10-23T11:00", Ends: "2026-10-23T12:00"}); err != nil {
		t.Fatalf("right after: %v", err)
	}
}

func TestAddSessionRepeatsWeeklyUpToTheDate_AC70(t *testing.T) {
	for until, want := range map[string]int{"2026-11-13": 3, "2026-11-12": 2, "2026-10-30": 1, "2027-04-23": 26} {
		st := scheduleStore()
		n, err := schedule(st, staff, 1, service.NewSession{Starts: "2026-10-30T13:00", Ends: "2026-10-30T15:00", RepeatUntil: until})
		if err != nil || n != want || st.SessionCount(1) != want+1 {
			t.Errorf("until %s: n = %d, err = %v, sessions = %d; want %d added", until, n, err, st.SessionCount(1), want)
		}
	}
}

func TestAddSessionRepeatThatClashesAddsNone_AC70(t *testing.T) {
	st := scheduleStore()
	// The existing session is on 23 Oct 09:00–11:00; a weekly repeat from 16 Oct would clash in week 2.
	st.LabSessions[0] = sess(1, 1, 24*8+9, 24*8+11) // 30 Oct 09:00–11:00
	_, err := schedule(st, staff, 1, service.NewSession{Starts: "2026-10-23T10:00", Ends: "2026-10-23T12:00", RepeatUntil: "2026-11-13"})
	if err != service.ErrSessionOverlap {
		t.Fatalf("err = %v, want ErrSessionOverlap", err)
	}
	if n := st.SessionCount(1); n != 1 {
		t.Fatalf("sessions = %d, want only the 1 that was there", n)
	}
}

func TestAddSessionRepeatKeepsTheLocalTimeAcrossADaylightSavingChange_AC70(t *testing.T) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		t.Skipf("no time zone data: %v", err)
	}
	st := scheduleStore()
	st.LabSessions = nil
	// Summer time ends on 25 Oct 2026 in Kyiv.
	n, err := service.NewLabs(st, clock, kyiv).AddSessions(context.Background(), staff, 1,
		service.NewSession{Starts: "2026-10-23T13:00", Ends: "2026-10-23T15:00", RepeatUntil: "2026-10-30"})
	if err != nil || n != 2 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	for _, s := range st.LabSessions {
		if h := s.StartsAt.In(kyiv).Hour(); h != 13 {
			t.Errorf("session on %s starts at %d:00 local, want 13:00", s.StartsAt.In(kyiv).Format("2 Jan"), h)
		}
	}
}

func TestAddSessionIsForTheLabsOwnerOnly_AC72(t *testing.T) {
	in := service.NewSession{Starts: "2026-10-24T13:00", Ends: "2026-10-24T15:00"}
	for name, tc := range map[string]struct {
		c    service.Caller
		want error
	}{
		"student":       {student, service.ErrForbidden},
		"another staff": {otherStaff, service.ErrForbidden},
		"nobody":        {service.Caller{}, service.ErrUnauthenticated},
	} {
		st := scheduleStore()
		if _, err := schedule(st, tc.c, 1, in); err != tc.want {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
		if n := st.SessionCount(1); n != 1 {
			t.Errorf("%s: %d sessions, want 1", name, n)
		}
	}
	if _, err := schedule(scheduleStore(), staff, 99, in); err != service.ErrLabNotFound {
		t.Errorf("missing lab: err = %v, want ErrLabNotFound", err)
	}
	// Omar may schedule his own lab.
	if _, err := schedule(scheduleStore(), otherStaff, 3, in); err != nil {
		t.Errorf("owner of lab 3: %v", err)
	}
}

func TestAddSessionStoreErrors(t *testing.T) {
	in := service.NewSession{Starts: "2026-10-24T13:00", Ends: "2026-10-24T15:00"}
	st := scheduleStore()
	st.ErrAddSessions = store.ErrNotFound // the lab was deleted between the lookup and the insert
	if _, err := schedule(st, staff, 1, in); err != service.ErrLabNotFound {
		t.Errorf("deleted lab: err = %v, want ErrLabNotFound", err)
	}
	st.ErrAddSessions = errDB
	if _, err := schedule(st, staff, 1, in); !errors.Is(err, errDB) {
		t.Errorf("db: err = %v, want errDB wrapped", err)
	}
	st.ErrAddSessions, st.ErrLabByID = nil, errDB
	if _, err := schedule(st, staff, 1, in); !errors.Is(err, errDB) {
		t.Errorf("lookup: err = %v, want errDB wrapped", err)
	}
}
