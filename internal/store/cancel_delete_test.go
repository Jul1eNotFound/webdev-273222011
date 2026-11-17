package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

var errNo = errors.New("refused by the check")

func student(t *testing.T, st *store.Store, name string) int64 {
	t.Helper()
	email := name + "@labwiz.test"
	if err := st.UpsertUser(context.Background(), store.User{Email: email, Name: name, PasswordHash: "unused", Role: "student"}); err != nil {
		t.Fatal(err)
	}
	u, err := st.UserByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// labWithSessions creates a lab with sessions at the given hours of the fixed future day (two hours each).
func labWithSessions(t *testing.T, st *store.Store, title string, hours ...int) (int64, []int64) {
	t.Helper()
	lab := newLab(t, st, title, 4)
	var periods []store.Period
	for _, h := range hours {
		periods = append(periods, store.Period{StartsAt: at(h), EndsAt: at(h + 2)})
	}
	ids, err := st.AddSessions(context.Background(), lab, periods)
	if err != nil {
		t.Fatal(err)
	}
	return lab, ids
}

func allow(store.SessionView, bool) error { return nil }

func labExists(t *testing.T, pool *pgxpool.Pool, labID int64) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM labs WHERE id = $1)`, labID).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestAC73_DB_CancelSessionKeepsItsBookings(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	_, ids := labWithSessions(t, st, "Optics", 0)
	if _, err := st.BookSession(ctx, student(t, st, "sam"), ids[0], allow); err != nil {
		t.Fatal(err)
	}
	now := hour.Add(-time.Hour)
	v, err := st.CancelSession(ctx, ids[0], now, func(s store.SessionView) error {
		if s.Booked != 1 || s.CancelledAt != nil {
			return fmt.Errorf("check saw %+v", s)
		}
		return nil
	})
	if err != nil || v.CancelledAt == nil || !v.CancelledAt.Equal(now) || v.Booked != 1 {
		t.Fatalf("v = %+v, err = %v", v, err)
	}
	got, err := st.SessionByID(ctx, ids[0])
	if err != nil || got.CancelledAt == nil || got.Booked != 1 || got.Title != "Optics" {
		t.Fatalf("after: %+v, err = %v; want cancelled with its booking", got, err)
	}
	if n := storetest.Count(t, pool, "bookings"); n != 1 {
		t.Fatalf("bookings = %d, want 1", n)
	}
}

func TestCancelSessionRefusedByTheCheckChangesNothing(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	_, ids := labWithSessions(t, st, "Optics", 0)
	if _, err := st.CancelSession(ctx, ids[0], hour, func(store.SessionView) error { return errNo }); !errors.Is(err, errNo) {
		t.Fatalf("err = %v, want the check's error", err)
	}
	if v, _ := st.SessionByID(ctx, ids[0]); v.CancelledAt != nil {
		t.Fatal("a refused cancel changed the session")
	}
	if _, err := st.CancelSession(ctx, 999999, hour, func(store.SessionView) error { return nil }); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing session: err = %v", err)
	}
	if _, err := st.SessionByID(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("SessionByID of a missing session: err = %v", err)
	}
}

// TestAC74_DB_BookingThatWaitedForACancelIsRefused holds the cancel's lock with a slow check while a booking
// waits for it. The booking must then see the session cancelled.
func TestAC74_DB_BookingThatWaitedForACancelIsRefused(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	_, ids := labWithSessions(t, st, "Optics", 0)
	sam := student(t, st, "sam")
	storetest.Warm(t, pool, 2)

	locked := make(chan struct{})
	var wg sync.WaitGroup
	var cancelErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, cancelErr = st.CancelSession(ctx, ids[0], hour, func(store.SessionView) error {
			close(locked)
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()
	<-locked
	_, bookErr := st.BookSession(ctx, sam, ids[0], func(s store.SessionView, _ bool) error {
		if s.CancelledAt != nil {
			return errNo
		}
		return nil
	})
	wg.Wait()
	if cancelErr != nil {
		t.Fatalf("cancel: %v", cancelErr)
	}
	if !errors.Is(bookErr, errNo) {
		t.Fatalf("booking after the cancel: err = %v, want it to see the session cancelled", bookErr)
	}
	if n := storetest.Count(t, pool, "bookings"); n != 0 {
		t.Fatalf("bookings = %d, want 0", n)
	}
}

func TestAC77_DB_DeleteLabRemovesItAndItsSessions(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	lab, _ := labWithSessions(t, st, "Optics", 0, 24)
	if err := st.DeleteLab(context.Background(), lab); err != nil {
		t.Fatal(err)
	}
	if labExists(t, pool, lab) || countSessions(t, pool, lab) != 0 {
		t.Fatal("the lab or its sessions are still there")
	}
	if err := st.DeleteLab(context.Background(), lab); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleting it again: err = %v, want ErrNotFound", err)
	}
}

func TestAC77_DB_DeleteLabWithAnyBookingIsRefused(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	sam := student(t, st, "sam")
	for name, setup := range map[string]func(ids []int64){
		"an upcoming booking": func(ids []int64) { storetest.InsertBooking(t, pool, sam, ids[1]) },
		"a past booking":      func(ids []int64) { storetest.InsertBooking(t, pool, sam, ids[0]) },
		"a cancelled session's booking": func(ids []int64) {
			storetest.InsertBooking(t, pool, sam, ids[1])
			if _, err := st.CancelSession(ctx, ids[1], hour, func(store.SessionView) error { return nil }); err != nil {
				t.Fatal(err)
			}
		},
	} {
		lab, ids := labWithSessions(t, st, name, -48, 24)
		setup(ids)
		if err := st.DeleteLab(ctx, lab); !errors.Is(err, store.ErrInUse) {
			t.Errorf("%s: err = %v, want ErrInUse", name, err)
		}
		if !labExists(t, pool, lab) || countSessions(t, pool, lab) != 2 {
			t.Errorf("%s: something was deleted", name)
		}
	}
}

// TestAC78_DB_DeleteWhileABookingHoldsTheLockIsRefused lets a booking hold its session's lock with a slow
// check while the lab is deleted. The cascading delete must wait, meet the new booking, and be refused.
func TestAC78_DB_DeleteWhileABookingHoldsTheLockIsRefused(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	lab, ids := labWithSessions(t, st, "Optics", 0)
	sam := student(t, st, "sam")
	storetest.Warm(t, pool, 2)

	locked := make(chan struct{})
	var wg sync.WaitGroup
	var bookErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, bookErr = st.BookSession(ctx, sam, ids[0], func(store.SessionView, bool) error {
			close(locked)
			time.Sleep(300 * time.Millisecond)
			return nil
		})
	}()
	<-locked
	deleteErr := st.DeleteLab(ctx, lab)
	wg.Wait()
	if bookErr != nil {
		t.Fatalf("booking: %v", bookErr)
	}
	if !errors.Is(deleteErr, store.ErrInUse) {
		t.Fatalf("delete: err = %v, want ErrInUse", deleteErr)
	}
	if !labExists(t, pool, lab) || storetest.Count(t, pool, "bookings") != 1 {
		t.Fatal("the lab or the booking is gone")
	}
}

func TestAC78_DB_BookingAfterTheLabWasDeletedIsNotFound(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	lab, ids := labWithSessions(t, st, "Optics", 0)
	if err := st.DeleteLab(ctx, lab); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BookSession(ctx, student(t, st, "sam"), ids[0], allow); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
