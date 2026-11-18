package store_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

func TestAC7_DB_CancelBookingRemovesTheRowAndFreesThePlace(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	_, ids := labWithSessions(t, st, "Optics", 0)
	sam, ana := student(t, st, "sam"), student(t, st, "ana")
	for _, u := range []int64{sam, ana} {
		if _, err := st.BookSession(ctx, u, ids[0], allow); err != nil {
			t.Fatal(err)
		}
	}
	v, err := st.CancelBooking(ctx, sam, ids[0], func(store.SessionView) error { return nil })
	if err != nil || v.Booked != 1 || v.Title != "Optics" {
		t.Fatalf("v = %+v, err = %v; want Optics with 1 booking left", v, err)
	}
	if n := storetest.Count(t, pool, "bookings"); n != 1 {
		t.Fatalf("bookings = %d, want Ana's only", n)
	}
	if mine, _ := st.SessionsBookedBy(ctx, ana, ids); len(mine) != 1 {
		t.Fatal("Ana's booking was cancelled too")
	}
}

func TestAC49_DB_CancellingABookingYouDoNotHave(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	_, ids := labWithSessions(t, st, "Optics", 0)
	sam, ana := student(t, st, "sam"), student(t, st, "ana")
	if _, err := st.BookSession(ctx, ana, ids[0], allow); err != nil {
		t.Fatal(err)
	}
	ok := func(store.SessionView) error { return nil }
	// Sam has no booking: Ana's must not be touched (MCP's AC-35 relies on this).
	if _, err := st.CancelBooking(ctx, sam, ids[0], ok); !errors.Is(err, store.ErrNoBooking) {
		t.Fatalf("no booking: err = %v, want ErrNoBooking", err)
	}
	if _, err := st.CancelBooking(ctx, sam, 999999, ok); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing session: err = %v, want ErrNotFound", err)
	}
	if n := storetest.Count(t, pool, "bookings"); n != 1 {
		t.Fatalf("bookings = %d, want Ana's 1", n)
	}
}

func TestCancelBookingRefusedByTheCheckKeepsTheRow_AC88(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	_, ids := labWithSessions(t, st, "Optics", 0)
	sam := student(t, st, "sam")
	if _, err := st.BookSession(ctx, sam, ids[0], allow); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CancelBooking(ctx, sam, ids[0], func(store.SessionView) error { return errNo }); !errors.Is(err, errNo) {
		t.Fatalf("err = %v, want the check's error", err)
	}
	if n := storetest.Count(t, pool, "bookings"); n != 1 {
		t.Fatalf("bookings = %d, want 1", n)
	}
}

func TestBookingsOfAndSessionsBookedByAreTheUsersOwn_AC89_AC90(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	_, ids := labWithSessions(t, st, "Optics", 0, 24, 48)
	sam, ana := student(t, st, "sam"), student(t, st, "ana")
	for _, b := range []struct{ user, session int64 }{{sam, ids[2]}, {sam, ids[0]}, {ana, ids[1]}} {
		if _, err := st.BookSession(ctx, b.user, b.session, allow); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CancelSession(ctx, ids[2], hour, func(store.SessionView) error { return nil }); err != nil {
		t.Fatal(err)
	}
	mine, err := st.BookingsOf(ctx, sam)
	if err != nil || len(mine) != 2 || mine[0].SessionID != ids[0] || mine[1].SessionID != ids[2] {
		t.Fatalf("Sam's bookings = %+v, err = %v; want sessions 1 and 3 in time order", mine, err)
	}
	if mine[1].CancelledAt == nil || mine[0].Title != "Optics" {
		t.Errorf("the cancelled session is not marked, or the lab is missing: %+v", mine)
	}
	booked, err := st.SessionsBookedBy(ctx, sam, ids)
	slices.Sort(booked)
	if err != nil || !slices.Equal(booked, []int64{ids[0], ids[2]}) {
		t.Fatalf("SessionsBookedBy = %v, %v", booked, err)
	}
	if none, err := st.SessionsBookedBy(ctx, sam, nil); err != nil || len(none) != 0 {
		t.Fatalf("no ids: %v, %v", none, err)
	}
}

// TestAC50_DB_ABookingThatWaitedForACancelGetsTheFreedPlace fills a session, holds a cancel's lock with a
// slow check while another student's booking waits, and checks the booking then gets the freed place.
func TestAC50_DB_ABookingThatWaitedForACancelGetsTheFreedPlace(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	lab := newLab(t, st, "Last place", 1)
	ids, err := st.AddSessions(ctx, lab, []store.Period{{StartsAt: at(0), EndsAt: at(2)}})
	if err != nil {
		t.Fatal(err)
	}
	sam, ana := student(t, st, "sam"), student(t, st, "ana")
	if _, err := st.BookSession(ctx, sam, ids[0], allow); err != nil {
		t.Fatal(err)
	}
	storetest.Warm(t, pool, 2)

	locked := make(chan struct{})
	var wg sync.WaitGroup
	var cancelErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, cancelErr = st.CancelBooking(ctx, sam, ids[0], func(store.SessionView) error {
			close(locked)
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()
	<-locked
	full := errors.New("full")
	_, bookErr := st.BookSession(ctx, ana, ids[0], func(s store.SessionView, _ bool) error {
		if s.Booked >= s.PlaceLimit {
			return full
		}
		return nil
	})
	wg.Wait()
	if cancelErr != nil || bookErr != nil {
		t.Fatalf("cancel err = %v, booking err = %v; want both to succeed", cancelErr, bookErr)
	}
	if n := storetest.Count(t, pool, "bookings"); n != 1 {
		t.Fatalf("bookings = %d, want 1, the limit", n)
	}
}
