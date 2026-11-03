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

var fixedNow = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

// plainChecker treats the hash as "hashed:" + password, and counts dummy compares.
type plainChecker struct{ dummyCalls int }

func (p *plainChecker) Compare(hash, plain string) bool { return hash == "hashed:"+plain }
func (p *plainChecker) CompareDummy(string)             { p.dummyCalls++ }

var errDB = errors.New("connection refused")

func newAuth() (*service.Auth, *servicetest.Store, *plainChecker) {
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: 7, Email: "student@labwiz.test", Name: "Sam", PasswordHash: "hashed:right", Role: "student"})
	pw := &plainChecker{}
	return service.NewAuth(st, pw, clock), st, pw
}

func TestLoginSuccessStoresOnlyTheTokenHash_AC3(t *testing.T) {
	a, st, _ := newAuth()
	s, err := a.Login(context.Background(), "  Student@Labwiz.test ", "right")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if s.Token == "" || !s.ExpiresAt.Equal(fixedNow.Add(service.SessionTTL)) {
		t.Fatalf("session = %+v", s)
	}
	keys := st.Sessions()
	if len(keys) != 1 || keys[0] == s.Token {
		t.Fatalf("stored sessions = %q, want one hash that is not the token", keys)
	}
	c, err := a.CurrentUser(context.Background(), s.Token)
	if err != nil || c.ID != 7 || c.Role != service.RoleStudent || c.Name != "Sam" {
		t.Fatalf("CurrentUser = %+v, %v", c, err)
	}
}

func TestLoginUnknownEmailAndWrongPasswordGiveTheSameError_AC17(t *testing.T) {
	a, _, pw := newAuth()
	_, errWrong := a.Login(context.Background(), "student@labwiz.test", "wrong")
	_, errUnknown := a.Login(context.Background(), "nobody@labwiz.test", "right")
	if errWrong != service.ErrInvalidCredentials || errUnknown != service.ErrInvalidCredentials {
		t.Fatalf("errors = %v / %v, want both ErrInvalidCredentials", errWrong, errUnknown)
	}
	if errWrong.Error() != "Invalid email or password." {
		t.Fatalf("message = %q", errWrong.Error())
	}
	if pw.dummyCalls != 1 {
		t.Fatalf("dummy compares = %d, want 1 for the unknown email", pw.dummyCalls)
	}
}

func TestLoginStoreErrors(t *testing.T) {
	a, st, _ := newAuth()
	st.ErrUserByEmail = errDB
	if _, err := a.Login(context.Background(), "student@labwiz.test", "right"); !errors.Is(err, errDB) {
		t.Fatalf("lookup error = %v", err)
	}
	st.ErrUserByEmail = nil
	st.ErrCreateSession = errDB
	if _, err := a.Login(context.Background(), "student@labwiz.test", "right"); !errors.Is(err, errDB) {
		t.Fatalf("create session error = %v", err)
	}
}

func TestCurrentUserWithoutValidSessionIsUnauthenticated_AC9(t *testing.T) {
	a, st, _ := newAuth()
	for _, token := range []string{"", "not-a-session"} {
		if _, err := a.CurrentUser(context.Background(), token); err != service.ErrUnauthenticated {
			t.Fatalf("CurrentUser(%q) err = %v", token, err)
		}
	}
	st.ErrSessionUser = errDB
	if _, err := a.CurrentUser(context.Background(), "x"); !errors.Is(err, errDB) {
		t.Fatalf("store error = %v", err)
	}
}

func TestCurrentUserExpiredSessionIsUnauthenticated_AC9(t *testing.T) {
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: 7, Email: "student@labwiz.test", PasswordHash: "hashed:right", Role: "student"})
	now := fixedNow
	a := service.NewAuth(st, &plainChecker{}, func() time.Time { return now })
	s, err := a.Login(context.Background(), "student@labwiz.test", "right")
	if err != nil {
		t.Fatal(err)
	}
	now = s.ExpiresAt
	if _, err := a.CurrentUser(context.Background(), s.Token); err != service.ErrUnauthenticated {
		t.Fatalf("expired session err = %v", err)
	}
}

func TestLogoutEndsTheSession_AC21(t *testing.T) {
	a, st, _ := newAuth()
	s, err := a.Login(context.Background(), "student@labwiz.test", "right")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Logout(context.Background(), s.Token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := a.CurrentUser(context.Background(), s.Token); err != service.ErrUnauthenticated {
		t.Fatalf("after logout err = %v", err)
	}
	if err := a.Logout(context.Background(), ""); err != nil {
		t.Fatalf("Logout(\"\") = %v", err)
	}
	st.ErrDeleteSession = errDB
	if err := a.Logout(context.Background(), "x"); !errors.Is(err, errDB) {
		t.Fatalf("store error = %v", err)
	}
}
