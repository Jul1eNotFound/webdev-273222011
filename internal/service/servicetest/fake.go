// Package servicetest has an in-memory store for tests that do not need PostgreSQL.
package servicetest

import (
	"context"
	"slices"
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
	// Bookers lists, per lab id, the students who booked it.
	Bookers map[int64][]store.Booker

	ErrUserByEmail, ErrCreateSession, ErrSessionUser, ErrDeleteSession, ErrLabs error
	ErrCreateLab, ErrLabByID, ErrLabBookers, ErrBookLab                         error

	// booked holds the (user id, lab id) pairs booked through BookLab, like the unique constraint on bookings.
	booked map[[2]int64]bool
}

func NewStore() *Store {
	return &Store{
		users: map[string]store.User{}, sessions: map[string]session{},
		Bookers: map[int64][]store.Booker{}, booked: map[[2]int64]bool{},
	}
}

// LabCount returns how many labs are stored, like counting rows in labs.
func (s *Store) LabCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Labs)
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
			l.Booked = len(s.Bookers[l.ID])
			out = append(out, l)
		}
	}
	return out, nil
}

// CreateLab gives a taken title ErrDuplicate, like the unique constraint on labs.title.
func (s *Store) CreateLab(_ context.Context, l store.Lab) (store.Lab, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrCreateLab != nil {
		return store.Lab{}, s.ErrCreateLab
	}
	var maxID int64
	for _, existing := range s.Labs {
		if existing.Title == l.Title {
			return store.Lab{}, store.ErrDuplicate
		}
		maxID = max(maxID, existing.ID)
	}
	l.ID = maxID + 1
	s.Labs = append(s.Labs, l)
	return l, nil
}

func (s *Store) LabByID(_ context.Context, id int64) (store.Lab, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrLabByID != nil {
		return store.Lab{}, s.ErrLabByID
	}
	for _, l := range s.Labs {
		if l.ID == id {
			l.Booked = len(s.Bookers[id])
			return l, nil
		}
	}
	return store.Lab{}, store.ErrNotFound
}

// BookLab holds the mutex throughout, so it is as atomic as the real store's locked transaction.
func (s *Store) BookLab(_ context.Context, userID, labID int64, check store.BookingCheck) (store.Lab, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrBookLab != nil {
		return store.Lab{}, s.ErrBookLab
	}
	i := slices.IndexFunc(s.Labs, func(l store.Lab) bool { return l.ID == labID })
	if i < 0 {
		return store.Lab{}, store.ErrNotFound
	}
	lab := s.Labs[i]
	lab.Booked = len(s.Bookers[labID])
	key := [2]int64{userID, labID}
	if err := check(lab, s.booked[key]); err != nil {
		return store.Lab{}, err
	}
	if s.booked[key] {
		return store.Lab{}, store.ErrDuplicate
	}
	s.booked[key] = true
	var b store.Booker
	for _, u := range s.users {
		if u.ID == userID {
			b = store.Booker{Name: u.Name, Email: u.Email}
		}
	}
	s.Bookers[labID] = append(s.Bookers[labID], b)
	lab.Booked++
	return lab, nil
}

func (s *Store) LabBookers(_ context.Context, labID int64) ([]store.Booker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrLabBookers != nil {
		return nil, s.ErrLabBookers
	}
	return s.Bookers[labID], nil
}
