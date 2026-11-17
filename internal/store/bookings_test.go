package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

var errFull = errors.New("full")

// staffID creates a staff account to own test labs, which must have an owner (AC-60).
func staffID(t *testing.T, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertUser(ctx, store.User{Email: "owner@labwiz.test", Name: "Owner", PasswordHash: "unused", Role: "staff"}); err != nil {
		t.Fatal(err)
	}
	u, err := st.UserByEmail(ctx, "owner@labwiz.test")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// labSession creates a lab with one session from now for an hour, and returns the session's id.
func labSession(t *testing.T, st *store.Store, title string, limit int) int64 {
	t.Helper()
	ctx := context.Background()
	lab, err := st.CreateLab(ctx, store.Lab{Title: title, PlaceLimit: limit, OwnerID: staffID(t, st)})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ids, err := st.AddSessions(ctx, lab.ID, []store.Period{{StartsAt: now, EndsAt: now.Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	return ids[0]
}

// TestAC6_DB_BookSessionHoldsTheSessionLockWhileTheCheckRuns makes the gap between counting and inserting wide
// on purpose: every check sleeps before deciding. Without the row lock all of them would count 0 and
// all would insert. With it, they run one at a time and only the first fits.
func TestAC6_DB_BookSessionHoldsTheSessionLockWhileTheCheckRuns(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	session := labSession(t, st, "Last place", 1)
	const n = 5
	users := make([]int64, n)
	for i := range n {
		email := fmt.Sprintf("racer%d@labwiz.test", i)
		if err := st.UpsertUser(ctx, store.User{Email: email, Name: email, PasswordHash: "unused", Role: "student"}); err != nil {
			t.Fatal(err)
		}
		u, err := st.UserByEmail(ctx, email)
		if err != nil {
			t.Fatal(err)
		}
		users[i] = u.ID
	}

	slowCheck := func(s store.SessionView, mine bool) error {
		time.Sleep(50 * time.Millisecond)
		if s.Booked >= s.PlaceLimit {
			return errFull
		}
		return nil
	}
	storetest.Warm(t, pool, n)
	start := make(chan struct{})
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i, id := range users {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = st.BookSession(ctx, id, session, slowCheck)
		}()
	}
	close(start)
	wg.Wait()

	ok := 0
	for i, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, errFull):
			t.Errorf("user %d: unexpected error %v", i, err)
		}
	}
	if ok != 1 {
		t.Errorf("%d bookings succeeded, want 1", ok)
	}
	if rows := storetest.Count(t, pool, "bookings"); rows != 1 {
		t.Fatalf("bookings = %d, want 1 (the limit)", rows)
	}
}

func TestBookSessionReportsAMissingSessionAndADuplicate_AC47_AC5(t *testing.T) {
	pool := storetest.Open(t)
	st := store.New(pool)
	ctx := context.Background()
	if err := st.UpsertUser(ctx, store.User{Email: "sam@labwiz.test", Name: "Sam", PasswordHash: "unused", Role: "student"}); err != nil {
		t.Fatal(err)
	}
	u, err := st.UserByEmail(ctx, "sam@labwiz.test")
	if err != nil {
		t.Fatal(err)
	}
	allow := func(store.SessionView, bool) error { return nil }
	if _, err := st.BookSession(ctx, u.ID, 999999, allow); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing session: err = %v, want ErrNotFound", err)
	}
	session := labSession(t, st, "Optics", 5)
	got, err := st.BookSession(ctx, u.ID, session, allow)
	if err != nil || got.Booked != 1 {
		t.Fatalf("first booking: lab = %+v, err = %v; want Booked 1", got, err)
	}
	// A check that lets a second booking through is stopped by the unique constraint.
	if _, err := st.BookSession(ctx, u.ID, session, allow); !errors.Is(err, store.ErrDuplicate) {
		t.Fatalf("second booking: err = %v, want ErrDuplicate", err)
	}
	if rows := storetest.Count(t, pool, "bookings"); rows != 1 {
		t.Fatalf("bookings = %d, want 1", rows)
	}
}
