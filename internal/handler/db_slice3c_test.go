package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// Slice 3c against real PostgreSQL.

func labExistsDB(t *testing.T, pool *pgxpool.Pool, labID int64) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM labs WHERE id = $1)`, labID).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func cancelledDB(t *testing.T, pool *pgxpool.Pool, sessionID int64) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `SELECT cancelled_at IS NOT NULL FROM lab_sessions WHERE id = $1`, sessionID).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func cancelPath(sessionID int64) string {
	return "/staff/sessions/" + strconv.FormatInt(sessionID, 10) + "/cancel"
}

func TestAC73_DB_CancelThroughTheAppKeepsTheBookings(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	robotics := firstSessionOfView(t, st, pool, "Robotics intro") // tomorrow
	bookByForm(t, signedIn(t, base, seed.StudentEmail, seedPasswords.Student), base, bookPath(robotics.SessionID))
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	wantRedirect(t, do(t, c, http.MethodPost, base+cancelPath(robotics.SessionID), url.Values{}), http.StatusSeeOther,
		staffLabPath(robotics.LabID)+"?cancelled=1")
	if !cancelledDB(t, pool, robotics.SessionID) || bookingRows(t, pool, robotics.SessionID, 0) != 1 {
		t.Fatal("want the session cancelled with its booking kept")
	}
	// A second student cannot book it any more (AC-74).
	ana := addStudents(t, st, 1)[0]
	if _, err := service.NewLabs(st, clock, time.UTC).Book(context.Background(), service.Caller{ID: ana, Role: service.RoleStudent}, robotics.SessionID); err != service.ErrSessionCancelled {
		t.Fatalf("booking a cancelled session: err = %v", err)
	}
}

func TestAC75_DB_StartedSessionCannotBeCancelled(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	microscopy := seeded(t, st, "Open microscopy").SessionID // 08:00–20:00; the clock says 08:30
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	wantStatus(t, do(t, c, http.MethodPost, base+cancelPath(microscopy), url.Values{}), http.StatusConflict, msgCancelStarted)
	if cancelledDB(t, pool, microscopy) {
		t.Fatal("the started session was cancelled")
	}
}

func TestAC77_DB_DeleteAnUnbookedLabAndRefuseABookedOne(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	c := signedIn(t, base, seed.StaffEmail, seedPasswords.Staff)
	robotics := firstSessionOfView(t, st, pool, "Robotics intro")
	wantRedirect(t, do(t, c, http.MethodPost, base+staffLabPath(robotics.LabID)+"/delete", url.Values{}), http.StatusSeeOther, "/staff/labs?deleted=1")
	if labExistsDB(t, pool, robotics.LabID) {
		t.Fatal("the lab is still there")
	}
	var sessions int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM lab_sessions WHERE id = $1`, robotics.SessionID).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("its session is still there (%d, %v)", sessions, err)
	}

	chemistry := seeded(t, st, "Chemistry basics")
	storetest.InsertBooking(t, pool, userIDOf(t, st, seed.StudentEmail), chemistry.SessionID)
	wantStatus(t, do(t, c, http.MethodPost, base+staffLabPath(chemistry.LabID)+"/delete", url.Values{}), http.StatusConflict, msgLabHasBookings)
	if !labExistsDB(t, pool, chemistry.LabID) || bookingRows(t, pool, chemistry.SessionID, 0) != 1 {
		t.Fatal("the booked lab or its booking is gone")
	}
}

func TestAC79_DB_AnotherStaffMemberCannotCancelOrDelete(t *testing.T) {
	base, st, pool := newDBAppWithPool(t)
	robotics := firstSessionOfView(t, st, pool, "Robotics intro")
	addStaff(t, st, omarEmail, omarPass)
	omar := signedIn(t, base, omarEmail, omarPass)
	wantStatus(t, do(t, omar, http.MethodPost, base+cancelPath(robotics.SessionID), url.Values{}), http.StatusForbidden, msgForbidden)
	wantStatus(t, do(t, omar, http.MethodPost, base+staffLabPath(robotics.LabID)+"/delete", url.Values{}), http.StatusForbidden, msgForbidden)
	if cancelledDB(t, pool, robotics.SessionID) || !labExistsDB(t, pool, robotics.LabID) {
		t.Fatal("something changed")
	}
}

// TestAC78_DB_DeleteAndBookAtTheSameTimeNeverLoseABooking races a delete of a lab nobody booked against a
// booking of its session, many times. Each round must end in one of the two allowed outcomes.
func TestAC78_DB_DeleteAndBookAtTheSameTimeNeverLoseABooking(t *testing.T) {
	_, st, pool := newDBAppWithPool(t)
	ctx := context.Background()
	labs := service.NewLabs(st, clock, time.UTC)
	stella := service.Caller{ID: userIDOf(t, st, seed.StaffEmail), Role: service.RoleStaff}
	sam := service.Caller{ID: userIDOf(t, st, seed.StudentEmail), Role: service.RoleStudent}
	day := time.Date(fixedNow.Year(), fixedNow.Month(), fixedNow.Day(), 0, 0, 0, 0, time.UTC)
	storetest.Warm(t, pool, 4)

	outcomes := map[string]int{}
	for round := range 20 {
		lab, err := st.CreateLab(ctx, store.Lab{Title: fmt.Sprintf("Race %d", round), PlaceLimit: 3, OwnerID: stella.ID})
		if err != nil {
			t.Fatal(err)
		}
		ids, err := st.AddSessions(ctx, lab.ID, []store.Period{{StartsAt: day.Add(14 * time.Hour), EndsAt: day.Add(16 * time.Hour)}})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		var deleteErr, bookErr error
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, deleteErr = labs.DeleteLab(ctx, stella, lab.ID) }()
		go func() { defer wg.Done(); <-start; _, bookErr = labs.Book(ctx, sam, ids[0]) }()
		close(start)
		wg.Wait()

		exists, booked := labExistsDB(t, pool, lab.ID), bookingRows(t, pool, ids[0], 0)
		switch {
		case deleteErr == nil && bookErr == service.ErrSessionNotFound && !exists && booked == 0:
			outcomes["deleted, booking 404"]++
		case deleteErr == service.ErrLabHasBookings && bookErr == nil && exists && booked == 1:
			outcomes["booked, delete 409"]++
		default:
			t.Fatalf("round %d: delete err %v, booking err %v, lab exists %v, bookings %d", round, deleteErr, bookErr, exists, booked)
		}
	}
	t.Logf("outcomes over 20 rounds: %v", outcomes)
}

// firstSessionOfView is firstSessionOf with the lab's and session's ids together.
func firstSessionOfView(t *testing.T, st *store.Store, pool *pgxpool.Pool, title string) store.SessionView {
	t.Helper()
	_, id := firstSessionOf(t, pool, title)
	v, err := st.SessionByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
