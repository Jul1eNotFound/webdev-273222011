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

type AuthStore interface {
	UserByEmail(ctx context.Context, email string) (store.User, error)
	CreateSession(ctx context.Context, idHash []byte, userID int64, expiresAt time.Time) error
	SessionUser(ctx context.Context, idHash []byte, now time.Time) (store.User, error)
	DeleteSession(ctx context.Context, idHash []byte) error
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
	s := Session{Token: rand.Text(), ExpiresAt: a.now().Add(SessionTTL)}
	if err := a.store.CreateSession(ctx, hashToken(s.Token), u.ID, s.ExpiresAt); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return s, nil
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
