package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
)

// Google sign-in against real PostgreSQL, with a fake Google. seed.Run gives student@labwiz.test a local
// account with no Google account linked.

func newGoogleDBApp(t *testing.T) (string, *store.Store, *pgxpool.Pool, *fakeGoogle) {
	t.Helper()
	pool := storetest.Open(t)
	st := store.New(pool)
	if err := seed.Run(context.Background(), st, fixedNow, time.UTC, seedPasswords); err != nil {
		t.Fatalf("seed: %v", err)
	}
	g := &fakeGoogle{id: service.GoogleIdentity{Subject: "g-sam", Email: seed.StudentEmail, EmailVerified: true}}
	return newServer(t, st, st, withGoogle(g)).URL, st, pool, g
}

// googleIDOf is the user's google_id, or "NULL".
func googleIDOf(t *testing.T, pool *pgxpool.Pool, email string) string {
	t.Helper()
	var id *string
	if err := pool.QueryRow(context.Background(), `SELECT google_id FROM users WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id == nil {
		return "NULL"
	}
	return *id
}

func TestAC20_DB_FirstGoogleSignInAttachesTheIDToTheSameRow(t *testing.T) {
	base, _, pool, _ := newGoogleDBApp(t)
	users := storetest.Count(t, pool, "users")
	c := newClient(t)
	wantRedirect(t, signInWithGoogle(t, c, base), http.StatusSeeOther, "/labs")
	if n := storetest.Count(t, pool, "users"); n != users {
		t.Fatalf("users = %d, want still %d", n, users)
	}
	if got := googleIDOf(t, pool, seed.StudentEmail); got != "g-sam" {
		t.Fatalf("google_id = %s, want g-sam", got)
	}
	if n := storetest.Count(t, pool, "sessions"); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
	wantStatus(t, do(t, c, http.MethodGet, base+"/labs", nil), http.StatusOK, "Labs available today")
}

func TestAC52_DB_UnverifiedEmailAttachesNothing(t *testing.T) {
	base, _, pool, g := newGoogleDBApp(t)
	g.id.EmailVerified = false
	wantRedirect(t, signInWithGoogle(t, newClient(t), base), http.StatusSeeOther, "/login?google=unverified")
	if got := googleIDOf(t, pool, seed.StudentEmail); got != "NULL" {
		t.Fatalf("google_id = %s, want NULL", got)
	}
	if n := storetest.Count(t, pool, "sessions"); n != 0 {
		t.Fatalf("sessions = %d, want 0", n)
	}
}

func TestAC53_DB_ChangedEmailUsesTheSameRow(t *testing.T) {
	base, st, pool, g := newGoogleDBApp(t)
	sam := userIDOf(t, st, seed.StudentEmail)
	if err := st.LinkGoogle(context.Background(), sam, "g-sam"); err != nil {
		t.Fatal(err)
	}
	users := storetest.Count(t, pool, "users")
	g.id = service.GoogleIdentity{Subject: "g-sam", Email: "sam.renamed@gmail.com", EmailVerified: true}
	c := newClient(t)
	wantRedirect(t, signInWithGoogle(t, c, base), http.StatusSeeOther, "/labs")
	if n := storetest.Count(t, pool, "users"); n != users {
		t.Fatalf("users = %d, want still %d", n, users)
	}
	var owner int64
	if err := pool.QueryRow(context.Background(), `SELECT user_id FROM sessions`).Scan(&owner); err != nil || owner != sam {
		t.Fatalf("the session belongs to %d (%v), want Sam %d", owner, err, sam)
	}
}

func TestAC54_DB_CancelledOrForgedCallbackCreatesNoSession(t *testing.T) {
	base, _, pool, _ := newGoogleDBApp(t)
	c := newClient(t)
	state := startGoogle(t, c, base)
	wantRedirect(t, googleCallback(t, c, base, url.Values{"error": {"access_denied"}, "state": {state}}),
		http.StatusSeeOther, "/login?google=cancelled")
	startGoogle(t, c, base)
	wantRedirect(t, googleCallback(t, c, base, url.Values{"state": {"forged"}, "code": {"the-code"}}),
		http.StatusSeeOther, "/login?google=state")
	if n := storetest.Count(t, pool, "sessions"); n != 0 {
		t.Fatalf("sessions = %d, want 0", n)
	}
	if got := googleIDOf(t, pool, seed.StudentEmail); got != "NULL" {
		t.Fatalf("google_id = %s, want NULL", got)
	}
}
