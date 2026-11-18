package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

const SessionTTL = 7 * 24 * time.Hour

type Role string

const (
	RoleStudent Role = "student"
	RoleStaff   Role = "staff"
)

// Caller is the signed-in user a request acts for.
type Caller struct {
	ID    int64
	Email string
	Name  string
	Role  Role
}

func (c Caller) IsStaff() bool { return c.Role == RoleStaff }

// RequireStaff allows signed-in staff only. Every staff-only action starts with it.
func RequireStaff(c Caller) error {
	if c.ID == 0 {
		return ErrUnauthenticated
	}
	if !c.IsStaff() {
		return ErrForbidden
	}
	return nil
}

// RequireOwner allows the staff member who owns the lab only (AC-64). Every action that manages one
// lab (its booking list, and from Slice 3b its sessions) runs it after looking the lab up.
func RequireOwner(c Caller, lab store.Lab) error {
	if err := RequireStaff(c); err != nil {
		return err
	}
	if lab.OwnerID != c.ID {
		return ErrForbidden
	}
	return nil
}

// RequireStudent allows signed-in students only, so staff cannot book (AC-46).
func RequireStudent(c Caller) error {
	if c.ID == 0 {
		return ErrUnauthenticated
	}
	if c.Role != RoleStudent {
		return ErrForbidden
	}
	return nil
}

type AuthStore interface {
	UserByEmail(ctx context.Context, email string) (store.User, error)
	CreateSession(ctx context.Context, idHash []byte, userID int64, expiresAt time.Time) error
	SessionUser(ctx context.Context, idHash []byte, now time.Time) (store.User, error)
	DeleteSession(ctx context.Context, idHash []byte) error
	// Google sign-in (Slice 6).
	UserByGoogleID(ctx context.Context, googleID string) (store.User, error)
	LinkGoogle(ctx context.Context, userID int64, googleID string) error
}

type PasswordChecker interface {
	Compare(hash, plain string) bool
	// CompareDummy spends the same time as Compare, so an unknown email is not faster.
	CompareDummy(plain string)
}

type Session struct {
	Token     string
	ExpiresAt time.Time
}

type Auth struct {
	store     AuthStore
	passwords PasswordChecker
	now       Clock
}

func NewAuth(s AuthStore, p PasswordChecker, now Clock) *Auth {
	return &Auth{store: s, passwords: p, now: now}
}

// Login checks an email and password and starts a session.
func (a *Auth) Login(ctx context.Context, email, password string) (Session, error) {
	u, err := a.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, store.ErrNotFound) {
		a.passwords.CompareDummy(password)
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, fmt.Errorf("look up user: %w", err)
	}
	if !a.passwords.Compare(u.PasswordHash, password) {
		return Session{}, ErrInvalidCredentials
	}
	return a.startSession(ctx, u.ID)
}

// startSession makes a new session for a user, the same for every way of signing in.
func (a *Auth) startSession(ctx context.Context, userID int64) (Session, error) {
	s := Session{Token: rand.Text(), ExpiresAt: a.now().Add(SessionTTL)}
	if err := a.store.CreateSession(ctx, hashToken(s.Token), userID, s.ExpiresAt); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

// GoogleIdentity is what Google says about the person signing in: its account id ("sub"), which never
// changes, and an email address that may change and may not be verified.
type GoogleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
}

// GoogleSignIn signs in with a Google account (Slice 6). A linked account is matched on Google's id
// alone, so a changed email still finds the same user (AC-53). Otherwise the account with the same email
// is linked, but only when Google has verified that email (AC-20, AC-52). Accounts are seeded: an
// address that matches none is refused, not signed up.
func (a *Auth) GoogleSignIn(ctx context.Context, id GoogleIdentity) (Session, error) {
	if id.Subject == "" {
		return Session{}, errors.New("google identity without a subject")
	}
	for attempt := 0; ; attempt++ {
		u, err := a.store.UserByGoogleID(ctx, id.Subject)
		if err == nil {
			return a.startSession(ctx, u.ID)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return Session{}, fmt.Errorf("look up google account: %w", err)
		}
		u, err = a.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(id.Email)))
		switch {
		case errors.Is(err, store.ErrNotFound):
			return Session{}, ErrGoogleNoAccount
		case err != nil:
			return Session{}, fmt.Errorf("look up user: %w", err)
		case !id.EmailVerified:
			return Session{}, ErrGoogleUnverified
		case u.GoogleID != "":
			return Session{}, ErrGoogleOtherAccount
		}
		// The database decides a race: if another sign-in linked first, look again, once.
		err = a.store.LinkGoogle(ctx, u.ID, id.Subject)
		if err == nil {
			return a.startSession(ctx, u.ID)
		}
		if attempt > 0 || !(errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrDuplicate)) {
			return Session{}, fmt.Errorf("link google account: %w", err)
		}
	}
}

// Logout ends the session. An empty token is not an error.
func (a *Auth) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := a.store.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// CurrentUser returns the caller for a session token, or ErrUnauthenticated.
func (a *Auth) CurrentUser(ctx context.Context, token string) (Caller, error) {
	if token == "" {
		return Caller{}, ErrUnauthenticated
	}
	u, err := a.store.SessionUser(ctx, hashToken(token), a.now())
	if errors.Is(err, store.ErrNotFound) {
		return Caller{}, ErrUnauthenticated
	}
	if err != nil {
		return Caller{}, fmt.Errorf("look up session: %w", err)
	}
	return Caller{ID: u.ID, Email: u.Email, Name: u.Name, Role: Role(u.Role)}, nil
}

// hashToken is what the sessions table stores, so a database leak does not leak live cookies.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
