// Package servicetest has an in-memory store for tests that do not need PostgreSQL.
package servicetest

import (
	"context"
	"slices"
	"strings"
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
	// LabSessions are the labs' scheduled sessions (Slice 3b).
	LabSessions []LabSession
	// Bookers lists, per session id, the students who booked it.
	Bookers map[int64][]store.Booker

	ErrUserByEmail, ErrCreateSession, ErrSessionUser, ErrDeleteSession error
	// ErrLabs makes SessionsBetween fail: the query behind the lab list.
	ErrLabs, ErrCreateLab, ErrLabByID, ErrLabsOwnedBy                   error
	ErrSessionsOfLab, ErrSessionBookers, ErrAddSessions, ErrBookSession error
	ErrSessionByID, ErrCancelSession, ErrDeleteLab                      error
	ErrCancelBooking, ErrBookingsOf, ErrSessionsBookedBy                error

	// booked holds the (user id, session id) pairs booked through BookSession, like the unique
	// constraint on bookings.
	booked map[[2]int64]bool
}

func NewStore() *Store {
	return &Store{
		users: map[string]store.User{}, sessions: map[string]session{},
		Bookers: map[int64][]store.Booker{}, booked: map[[2]int64]bool{},
	}
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

// LabSession is a lab_sessions row in the fake store.
type LabSession struct {
	ID          int64
	LabID       int64
	StartsAt    time.Time
	EndsAt      time.Time
	CancelledAt *time.Time
}

// LabCount returns how many labs are stored, like counting rows in labs.
func (s *Store) LabCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Labs)
}

// SessionCount returns how many sessions a lab has, like counting its rows in lab_sessions.
func (s *Store) SessionCount(labID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, x := range s.LabSessions {
		if x.LabID == labID {
			n++
		}
	}
	return n
}

// lab returns the lab with its owner's name, as the real join does. The caller holds the mutex.
func (s *Store) lab(id int64) (store.Lab, bool) {
	for _, l := range s.Labs {
		if l.ID == id {
			for _, u := range s.users {
				if u.ID == l.OwnerID {
					l.OwnerName = u.Name
				}
			}
			return l, true
		}
	}
	return store.Lab{}, false
}

// view joins a session with its lab and counts its bookings, as sessionColumns does. The caller holds the mutex.
func (s *Store) view(x LabSession) store.SessionView {
	l, _ := s.lab(x.LabID)
	return store.SessionView{
		SessionID: x.ID, LabID: l.ID, Title: l.Title, Description: l.Description, PlaceLimit: l.PlaceLimit,
		OwnerID: l.OwnerID, OwnerName: l.OwnerName, StartsAt: x.StartsAt, EndsAt: x.EndsAt,
		CancelledAt: x.CancelledAt, Booked: len(s.Bookers[x.ID]),
	}
}

// sorted returns the sessions in time order, as the real queries do. The caller holds the mutex.
func (s *Store) sorted() []LabSession {
	out := slices.Clone(s.LabSessions)
	slices.SortStableFunc(out, func(a, b LabSession) int { return a.StartsAt.Compare(b.StartsAt) })
	return out
}

func (s *Store) SessionsBetween(_ context.Context, from, to time.Time) ([]store.SessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrLabs != nil {
		return nil, s.ErrLabs
	}
	var out []store.SessionView
	for _, x := range s.sorted() {
		if x.StartsAt.Before(to) && x.EndsAt.After(from) {
			out = append(out, s.view(x))
		}
	}
	return out, nil
}

func (s *Store) SessionsOfLab(_ context.Context, labID int64) ([]store.SessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrSessionsOfLab != nil {
		return nil, s.ErrSessionsOfLab
	}
	var out []store.SessionView
	for _, x := range s.sorted() {
		if x.LabID == labID {
			out = append(out, s.view(x))
		}
	}
	return out, nil
}

func (s *Store) SessionBookers(_ context.Context, labID int64) ([]store.SessionBooker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrSessionBookers != nil {
		return nil, s.ErrSessionBookers
	}
	var out []store.SessionBooker
	for _, x := range s.sorted() {
		if x.LabID == labID {
			for _, b := range s.Bookers[x.ID] {
				out = append(out, store.SessionBooker{SessionID: x.ID, Booker: b})
			}
		}
	}
	return out, nil
}

func (s *Store) LabsOwnedBy(_ context.Context, ownerID int64, now time.Time) ([]store.OwnedLab, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrLabsOwnedBy != nil {
		return nil, s.ErrLabsOwnedBy
	}
	var out []store.OwnedLab
	for _, l := range s.Labs {
		if l.OwnerID != ownerID {
			continue
		}
		o := store.OwnedLab{}
		o.Lab, _ = s.lab(l.ID)
		for _, x := range s.sorted() {
			if x.LabID != l.ID {
				continue
			}
			o.Sessions++
			if o.Next == nil && x.EndsAt.After(now) && x.CancelledAt == nil {
				o.Next = &store.Period{StartsAt: x.StartsAt, EndsAt: x.EndsAt}
			}
		}
		out = append(out, o)
	}
	// Labs with a next session first, soonest first, then by title, as the real query orders them.
	slices.SortStableFunc(out, func(a, b store.OwnedLab) int {
		switch {
		case a.Next != nil && b.Next != nil && !a.Next.StartsAt.Equal(b.Next.StartsAt):
			return a.Next.StartsAt.Compare(b.Next.StartsAt)
		case a.Next != nil && b.Next == nil:
			return -1
		case a.Next == nil && b.Next != nil:
			return 1
		}
		return strings.Compare(a.Title, b.Title)
	})
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
	if l, ok := s.lab(id); ok {
		return l, nil
	}
	return store.Lab{}, store.ErrNotFound
}

// AddSessions refuses an overlap with a live session of the lab, or within the batch, and then adds
// nothing, like the exclusion constraint inside the real store's transaction.
func (s *Store) AddSessions(_ context.Context, labID int64, periods []store.Period) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrAddSessions != nil {
		return nil, s.ErrAddSessions
	}
	if _, ok := s.lab(labID); !ok {
		return nil, store.ErrNotFound
	}
	var maxID int64
	taken := []store.Period{}
	for _, x := range s.LabSessions {
		maxID = max(maxID, x.ID)
		if x.LabID == labID && x.CancelledAt == nil {
			taken = append(taken, store.Period{StartsAt: x.StartsAt, EndsAt: x.EndsAt})
		}
	}
	for _, p := range periods {
		for _, t := range taken {
			if p.StartsAt.Before(t.EndsAt) && t.StartsAt.Before(p.EndsAt) {
				return nil, store.ErrOverlap
			}
		}
		taken = append(taken, p)
	}
	ids := make([]int64, len(periods))
	for i, p := range periods {
		maxID++
		ids[i] = maxID
		s.LabSessions = append(s.LabSessions, LabSession{ID: maxID, LabID: labID, StartsAt: p.StartsAt, EndsAt: p.EndsAt})
	}
	return ids, nil
}

// BookSession holds the mutex throughout, so it is as atomic as the real store's locked transaction.
func (s *Store) BookSession(_ context.Context, userID, sessionID int64, check store.BookingCheck) (store.SessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrBookSession != nil {
		return store.SessionView{}, s.ErrBookSession
	}
	i := slices.IndexFunc(s.LabSessions, func(x LabSession) bool { return x.ID == sessionID })
	if i < 0 {
		return store.SessionView{}, store.ErrNotFound
	}
	v := s.view(s.LabSessions[i])
	key := [2]int64{userID, sessionID}
	if err := check(v, s.booked[key]); err != nil {
		return store.SessionView{}, err
	}
	if s.booked[key] {
		return store.SessionView{}, store.ErrDuplicate
	}
	s.booked[key] = true
	var b store.Booker
	for _, u := range s.users {
		if u.ID == userID {
			b = store.Booker{Name: u.Name, Email: u.Email}
		}
	}
	s.Bookers[sessionID] = append(s.Bookers[sessionID], b)
	v.Booked++
	return v, nil
}

func (s *Store) SessionByID(_ context.Context, id int64) (store.SessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrSessionByID != nil {
		return store.SessionView{}, s.ErrSessionByID
	}
	for _, x := range s.LabSessions {
		if x.ID == id {
			return s.view(x), nil
		}
	}
	return store.SessionView{}, store.ErrNotFound
}

// CancelSession holds the mutex throughout, so it is as atomic as the real store's locked transaction.
func (s *Store) CancelSession(_ context.Context, sessionID int64, now time.Time, check store.CancelCheck) (store.SessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrCancelSession != nil {
		return store.SessionView{}, s.ErrCancelSession
	}
	i := slices.IndexFunc(s.LabSessions, func(x LabSession) bool { return x.ID == sessionID })
	if i < 0 {
		return store.SessionView{}, store.ErrNotFound
	}
	if err := check(s.view(s.LabSessions[i])); err != nil {
		return store.SessionView{}, err
	}
	s.LabSessions[i].CancelledAt = &now
	return s.view(s.LabSessions[i]), nil
}

// DeleteLab refuses a lab whose sessions have any booker, like the RESTRICT key on bookings, and otherwise
// removes the lab and its sessions, like ON DELETE CASCADE.
func (s *Store) DeleteLab(_ context.Context, labID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrDeleteLab != nil {
		return s.ErrDeleteLab
	}
	if _, ok := s.lab(labID); !ok {
		return store.ErrNotFound
	}
	for _, x := range s.LabSessions {
		if x.LabID == labID && len(s.Bookers[x.ID]) > 0 {
			return store.ErrInUse
		}
	}
	s.Labs = slices.DeleteFunc(s.Labs, func(l store.Lab) bool { return l.ID == labID })
	s.LabSessions = slices.DeleteFunc(s.LabSessions, func(x LabSession) bool { return x.LabID == labID })
	return nil
}

// CancelBooking holds the mutex throughout, so it is as atomic as the real store's locked transaction.
// Only a booking made through BookSession can be cancelled, keyed on the user, like the real DELETE.
func (s *Store) CancelBooking(_ context.Context, userID, sessionID int64, check store.CancelBookingCheck) (store.SessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrCancelBooking != nil {
		return store.SessionView{}, s.ErrCancelBooking
	}
	i := slices.IndexFunc(s.LabSessions, func(x LabSession) bool { return x.ID == sessionID })
	if i < 0 {
		return store.SessionView{}, store.ErrNotFound
	}
	key := [2]int64{userID, sessionID}
	if !s.booked[key] {
		return store.SessionView{}, store.ErrNoBooking
	}
	if err := check(s.view(s.LabSessions[i])); err != nil {
		return store.SessionView{}, err
	}
	delete(s.booked, key)
	// Remove the Booker that BookSession added for this user.
	var me store.Booker
	for _, u := range s.users {
		if u.ID == userID {
			me = store.Booker{Name: u.Name, Email: u.Email}
		}
	}
	if j := slices.Index(s.Bookers[sessionID], me); j >= 0 {
		s.Bookers[sessionID] = slices.Delete(s.Bookers[sessionID], j, j+1)
	}
	return s.view(s.LabSessions[i]), nil
}

// BookingsOf returns the sessions userID booked through BookSession, in time order.
func (s *Store) BookingsOf(_ context.Context, userID int64) ([]store.SessionView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrBookingsOf != nil {
		return nil, s.ErrBookingsOf
	}
	var out []store.SessionView
	for _, x := range s.sorted() {
		if s.booked[[2]int64{userID, x.ID}] {
			out = append(out, s.view(x))
		}
	}
	return out, nil
}

func (s *Store) SessionsBookedBy(_ context.Context, userID int64, sessionIDs []int64) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ErrSessionsBookedBy != nil {
		return nil, s.ErrSessionsBookedBy
	}
	var out []int64
	for _, id := range sessionIDs {
		if s.booked[[2]int64{userID, id}] {
			out = append(out, id)
		}
	}
	return out, nil
}
