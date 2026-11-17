package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// maxWeeklySessions is how many sessions one "Repeat weekly until" may add (AC-70): half a year.
const maxWeeklySessions = 26

// NewSession is the add-session form as typed. AddSessions parses it, so HTTP and MCP get the same messages.
type NewSession struct {
	Starts string
	Ends   string
	// RepeatUntil is an optional date: the session is repeated every week up to and including it.
	RepeatUntil string
}

// SessionBookers is one session of a lab with the students who booked it, for the owner's page.
type SessionBookers struct {
	SessionCard
	Bookers []store.Booker
	// Cancellable is true while the owner may still cancel it: not cancelled and not started (AC-75).
	Cancellable bool
}

// StaffLabPage is what the owner sees on /staff/labs/{id}: the lab, its sessions that have not ended in
// time order, then its past sessions, most recent first, each with who booked it (AC-14).
type StaffLabPage struct {
	Lab      store.Lab
	Upcoming []SessionBookers
	Past     []SessionBookers
	// Deletable is true while no session of the lab has a booking, past and cancelled ones included (AC-77).
	Deletable bool
}

// ownLab looks a lab up for the caller, who must be its owner (AC-64, AC-72). The role is checked
// before the lookup, so a student cannot even learn whether a lab id exists (AC-45).
func (l *Labs) ownLab(ctx context.Context, caller Caller, id int64) (store.Lab, error) {
	if err := RequireStaff(caller); err != nil {
		return store.Lab{}, err
	}
	lab, err := l.lab(ctx, id)
	if err != nil {
		return store.Lab{}, err
	}
	if err := RequireOwner(caller, lab); err != nil {
		return store.Lab{}, err
	}
	return lab, nil
}

// Bookings returns a lab's sessions and who booked each one, to the lab's owner only (AC-14, AC-64).
func (l *Labs) Bookings(ctx context.Context, caller Caller, id int64) (StaffLabPage, error) {
	lab, err := l.ownLab(ctx, caller, id)
	if err != nil {
		return StaffLabPage{}, err
	}
	views, err := l.store.SessionsOfLab(ctx, id)
	if err != nil {
		return StaffLabPage{}, fmt.Errorf("list sessions: %w", err)
	}
	bookers, err := l.store.SessionBookers(ctx, id)
	if err != nil {
		return StaffLabPage{}, fmt.Errorf("list bookers: %w", err)
	}
	bySession := map[int64][]store.Booker{}
	for _, b := range bookers {
		bySession[b.SessionID] = append(bySession[b.SessionID], b.Booker)
	}
	page := StaffLabPage{Lab: lab, Deletable: len(bookers) == 0}
	for _, v := range views {
		c := l.card(v)
		s := SessionBookers{SessionCard: c, Bookers: bySession[v.SessionID], Cancellable: !c.Cancelled && !c.Started}
		if v.EndsAt.After(l.now()) {
			page.Upcoming = append(page.Upcoming, s)
		} else {
			page.Past = append(page.Past, s)
		}
	}
	slices.Reverse(page.Past)
	return page, nil
}

// AddSessions schedules a session of a lab, or one every week up to a date, for the lab's owner only
// (AC-67, AC-72). They are added all or none, and none may overlap another session of the lab (AC-69,
// AC-70). It returns how many sessions were added.
func (l *Labs) AddSessions(ctx context.Context, caller Caller, labID int64, in NewSession) (int, error) {
	if _, err := l.ownLab(ctx, caller, labID); err != nil {
		return 0, err
	}
	periods, err := l.parseSessions(in)
	if err != nil {
		return 0, err
	}
	// The exclusion constraint decides an overlap, so two owners adding sessions at once cannot both succeed.
	ids, err := l.store.AddSessions(ctx, labID, periods)
	switch {
	case errors.Is(err, store.ErrOverlap):
		return 0, ErrSessionOverlap
	case errors.Is(err, store.ErrNotFound):
		return 0, ErrLabNotFound // deleted since the lookup
	case err != nil:
		return 0, fmt.Errorf("add sessions: %w", err)
	}
	return len(ids), nil
}

// parseSessions checks the form (AC-68) and returns one period, or one per week up to the repeat date.
func (l *Labs) parseSessions(in NewSession) ([]store.Period, error) {
	starts, errStarts := time.ParseInLocation(DateTimeLayout, strings.TrimSpace(in.Starts), l.loc)
	ends, errEnds := time.ParseInLocation(DateTimeLayout, strings.TrimSpace(in.Ends), l.loc)
	if errStarts != nil || errEnds != nil {
		return nil, ErrInvalidPeriod
	}
	if !ends.After(starts) {
		return nil, ErrPeriodOrder
	}
	if !starts.After(l.now()) {
		return nil, ErrSessionInPast
	}
	first := store.Period{StartsAt: starts, EndsAt: ends}
	repeat := strings.TrimSpace(in.RepeatUntil)
	if repeat == "" {
		return []store.Period{first}, nil
	}
	until, err := time.ParseInLocation(DateLayout, repeat, l.loc)
	if err != nil {
		return nil, ErrInvalidRepeat
	}
	startDay := time.Date(starts.Year(), starts.Month(), starts.Day(), 0, 0, 0, 0, l.loc)
	if until.Before(startDay) {
		return nil, ErrRepeatBeforeStart
	}
	// Sessions start on the repeat date at the latest. AddDate keeps the wall-clock time across a
	// daylight saving change.
	var periods []store.Period
	for week := 0; ; week++ {
		p := store.Period{StartsAt: starts.AddDate(0, 0, 7*week), EndsAt: ends.AddDate(0, 0, 7*week)}
		if !p.StartsAt.Before(until.AddDate(0, 0, 1)) {
			break
		}
		if len(periods) == maxWeeklySessions {
			return nil, ErrRepeatTooLong
		}
		periods = append(periods, p)
	}
	return periods, nil
}

// Book books a place in a session for the calling student and returns the session with its new free
// places. Students only (AC-46). Any session can be booked until it starts (AC-83, AC-84). The rules run
// inside the store's transaction while the session is locked, so two students cannot both take the last
// place (AC-6).
func (l *Labs) Book(ctx context.Context, caller Caller, sessionID int64) (SessionCard, error) {
	if err := RequireStudent(caller); err != nil {
		return SessionCard{}, err
	}
	now := l.now()
	v, err := l.store.BookSession(ctx, caller.ID, sessionID, func(s store.SessionView, mine bool) error {
		switch {
		case s.CancelledAt != nil:
			return ErrSessionCancelled // AC-74
		case mine:
			return ErrAlreadyBooked // AC-5
		case !now.Before(s.StartsAt):
			return ErrSessionStarted // AC-83
		case s.Booked >= s.PlaceLimit:
			return ErrLabFull // AC-1
		}
		return nil
	})
	var se *Error
	switch {
	case err == nil:
		return l.card(v), nil
	case errors.As(err, &se):
		return SessionCard{}, err
	case errors.Is(err, store.ErrNotFound):
		return SessionCard{}, ErrSessionNotFound // AC-47
	case errors.Is(err, store.ErrDuplicate):
		return SessionCard{}, ErrAlreadyBooked
	}
	return SessionCard{}, fmt.Errorf("book session: %w", err)
}

// cancelCheck refuses cancelling a session that is already cancelled or has started (AC-75), and lets only
// its lab's owner do it (AC-79).
func (l *Labs) cancelCheck(caller Caller, now time.Time) store.CancelCheck {
	return func(v store.SessionView) error {
		if err := RequireOwner(caller, store.Lab{ID: v.LabID, OwnerID: v.OwnerID}); err != nil {
			return err
		}
		switch {
		case v.CancelledAt != nil:
			return ErrAlreadyCancelled
		case !now.Before(v.StartsAt):
			return ErrCancelStarted
		}
		return nil
	}
}

// CancelPage returns the session the owner is about to cancel, for the confirmation page (AC-76). It
// refuses what CancelSession would refuse, so the page never offers an impossible action.
func (l *Labs) CancelPage(ctx context.Context, caller Caller, sessionID int64) (SessionCard, error) {
	if err := RequireStaff(caller); err != nil {
		return SessionCard{}, err
	}
	v, err := l.store.SessionByID(ctx, sessionID)
	if errors.Is(err, store.ErrNotFound) {
		return SessionCard{}, ErrSessionNotFound
	}
	if err != nil {
		return SessionCard{}, fmt.Errorf("look up session: %w", err)
	}
	if err := l.cancelCheck(caller, l.now())(v); err != nil {
		return SessionCard{}, err
	}
	return l.card(v), nil
}

// CancelSession cancels an upcoming session for its lab's owner (AC-73, AC-79). Its bookings are kept, so
// the students see the cancellation. The check runs while the session is locked, the same lock a booking
// takes, so no booking gets in after the cancel (AC-74).
func (l *Labs) CancelSession(ctx context.Context, caller Caller, sessionID int64) (SessionCard, error) {
	if err := RequireStaff(caller); err != nil {
		return SessionCard{}, err
	}
	now := l.now()
	v, err := l.store.CancelSession(ctx, sessionID, now, l.cancelCheck(caller, now))
	var se *Error
	switch {
	case err == nil:
		return l.card(v), nil
	case errors.As(err, &se):
		return SessionCard{}, err
	case errors.Is(err, store.ErrNotFound):
		return SessionCard{}, ErrSessionNotFound
	}
	return SessionCard{}, fmt.Errorf("cancel session: %w", err)
}

// DeletePage returns the lab the owner is about to delete with its sessions, for the confirmation page
// (AC-76). It refuses a lab that has any booking, as DeleteLab would (AC-77).
func (l *Labs) DeletePage(ctx context.Context, caller Caller, labID int64) (LabPage, error) {
	lab, err := l.ownLab(ctx, caller, labID)
	if err != nil {
		return LabPage{}, err
	}
	views, err := l.store.SessionsOfLab(ctx, labID)
	if err != nil {
		return LabPage{}, fmt.Errorf("list sessions: %w", err)
	}
	page := LabPage{Lab: lab, Sessions: l.cards(views)}
	for _, s := range page.Sessions {
		if s.Booked > 0 {
			return LabPage{}, ErrLabHasBookings
		}
	}
	return page, nil
}

// DeleteLab deletes a lab nobody has booked, with its sessions, for its owner (AC-77, AC-79). The database
// decides whether it has bookings, so a booking made while the delete runs is never lost (AC-78).
func (l *Labs) DeleteLab(ctx context.Context, caller Caller, labID int64) (store.Lab, error) {
	lab, err := l.ownLab(ctx, caller, labID)
	if err != nil {
		return store.Lab{}, err
	}
	err = l.store.DeleteLab(ctx, labID)
	switch {
	case errors.Is(err, store.ErrInUse):
		return store.Lab{}, ErrLabHasBookings
	case errors.Is(err, store.ErrNotFound):
		return store.Lab{}, ErrLabNotFound // deleted since the lookup
	case err != nil:
		return store.Lab{}, fmt.Errorf("delete lab: %w", err)
	}
	return lab, nil
}
