// Package servicetest has an in-memory store for tests that do not need PostgreSQL.
package servicetest

import (
	"context"
	"sync"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

type session struct {
	userID    int64
	expiresAt time.Time
}

// Store implements service.AuthStore and service.LabStore. Set an Err field to make that method fail.
type Store struct {
	mu       sync.Mutex
	users    map[string]store.User
	sessions map[string]session
	Labs     []store.Lab

	ErrUserByEmail, ErrCreateSession, ErrSessionUser, ErrDeleteSession, ErrLabs error
}

func NewStore() *Store {
	return &Store{users: map[string]store.User{}, sessions: map[string]session{}}
}

func (s *Store) AddUser(u store.User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.Email] = u
}

// Sessions returns the stored session keys, which are token hashes.
func (s *Store) Sessions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.sessions {
		keys = append(keys, k)
	}
	return keys
}

func (s *Store) UserByEmail(_ context.Context, email string) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrUserByEmail != nil {
		return store.User{}, s.ErrUserByEmail
	}
	u, ok := s.users[email]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (s *Store) CreateSession(_ context.Context, idHash []byte, userID int64, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrCreateSession != nil {
		return s.ErrCreateSession
	}
	s.sessions[string(idHash)] = session{userID, expiresAt}
	return nil
}

func (s *Store) SessionUser(_ context.Context, idHash []byte, now time.Time) (store.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrSessionUser != nil {
		return store.User{}, s.ErrSessionUser
	}
	sess, ok := s.sessions[string(idHash)]
	if !ok || !sess.expiresAt.After(now) {
		return store.User{}, store.ErrNotFound
	}
	for _, u := range s.users {
		if u.ID == sess.userID {
			return u, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (s *Store) DeleteSession(_ context.Context, idHash []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrDeleteSession != nil {
		return s.ErrDeleteSession
	}
	delete(s.sessions, string(idHash))
	return nil
}

func (s *Store) LabsAvailableBetween(_ context.Context, from, to time.Time) ([]store.Lab, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrLabs != nil {
		return nil, s.ErrLabs
	}
	var out []store.Lab
	for _, l := range s.Labs {
		if l.StartsAt.Before(to) && l.EndsAt.After(from) {
			out = append(out, l)
		}
	}
	return out, nil
}
