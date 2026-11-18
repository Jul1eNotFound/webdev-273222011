package store_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store/storetest"
	"github.com/Jul1eNotFound/webdev-273222011/migrations"
)

func addUser(t *testing.T, st *store.Store, email string) int64 {
	t.Helper()
	ctx := context.Background()
	if err := st.UpsertUser(ctx, store.User{Email: email, Name: email, PasswordHash: "unused", Role: "student"}); err != nil {
		t.Fatal(err)
	}
	u, err := st.UserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestAC20_DB_LinkGoogleLinksOnceAndOneAccountToOneUser(t *testing.T) {
	st := store.New(storetest.Open(t))
	ctx := context.Background()
	sam := addUser(t, st, "sam@labwiz.test")
	ana := addUser(t, st, "ana@labwiz.test")

	if _, err := st.UserByGoogleID(ctx, "g-sam"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("before linking: err = %v, want ErrNotFound", err)
	}
	if err := st.LinkGoogle(ctx, sam, "g-sam"); err != nil {
		t.Fatal(err)
	}
	u, err := st.UserByGoogleID(ctx, "g-sam")
	if err != nil || u.ID != sam || u.GoogleID != "g-sam" || u.Email != "sam@labwiz.test" {
		t.Fatalf("UserByGoogleID = %+v, %v", u, err)
	}
	// A linked user keeps the account it has: no second link.
	if err := st.LinkGoogle(ctx, sam, "g-other"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("relinking Sam: err = %v, want ErrNotFound", err)
	}
	// One Google account links to one user only: the unique constraint decides.
	if err := st.LinkGoogle(ctx, ana, "g-sam"); !errors.Is(err, store.ErrDuplicate) {
		t.Errorf("linking g-sam to Ana: err = %v, want ErrDuplicate", err)
	}
	if err := st.LinkGoogle(ctx, 9999, "g-x"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown user: err = %v, want ErrNotFound", err)
	}
	// Re-seeding an account keeps its link.
	if err := st.UpsertUser(ctx, store.User{Email: "sam@labwiz.test", Name: "Sam", PasswordHash: "new", Role: "student"}); err != nil {
		t.Fatal(err)
	}
	if u, _ := st.UserByEmail(ctx, "sam@labwiz.test"); u.GoogleID != "g-sam" {
		t.Errorf("after re-seeding, google id = %q", u.GoogleID)
	}
	if u, _ := st.UserByEmail(ctx, "ana@labwiz.test"); u.GoogleID != "" {
		t.Errorf("Ana's google id = %q, want none", u.GoogleID)
	}
}

func TestAC53_DB_MigrationAddsGoogleIDsAndKeepsUsers(t *testing.T) {
	pool := storetest.Open(t)
	url := os.Getenv("TEST_DATABASE_URL")
	t.Cleanup(func() {
		if err := migrations.Up(url); err != nil {
			t.Errorf("migrate back up: %v", err)
		}
	})
	st := store.New(pool)
	ctx := context.Background()
	sam := addUser(t, st, "sam@labwiz.test")
	if err := st.LinkGoogle(ctx, sam, "g-sam"); err != nil {
		t.Fatal(err)
	}
	if err := migrations.To(url, 4); err != nil {
		t.Fatalf("migrate down to 4: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = 'sam@labwiz.test'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("after down: %d rows, %v", n, err)
	}
	if err := migrations.Up(url); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	u, err := st.UserByEmail(ctx, "sam@labwiz.test")
	if err != nil || u.ID != sam || u.GoogleID != "" {
		t.Fatalf("after down and up: %+v, %v; want Sam, unlinked", u, err)
	}
}
