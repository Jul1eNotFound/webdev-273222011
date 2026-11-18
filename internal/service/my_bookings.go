package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// BookingsPage is a student's own bookings for /bookings (AC-90): sessions that have not ended in time
// order, then past ones, most recent first.
type BookingsPage struct {
	Upcoming []SessionCard
	Past     []SessionCard
}

// MyBookings returns the calling student's bookings, cancelled sessions included and marked (AC-90).
// Students only: staff book nothing.
func (l *Labs) MyBookings(ctx context.Context, caller Caller) (BookingsPage, error) {
	if err := RequireStudent(caller); err != nil {
		return BookingsPage{}, err
	}
	views, err := l.store.BookingsOf(ctx, caller.ID)
	if err != nil {
		return BookingsPage{}, fmt.Errorf("list own bookings: %w", err)
	}
	var page BookingsPage
	for _, v := range views {
		c := l.card(v)
		c.Mine = true
		if v.EndsAt.After(l.now()) {
			page.Upcoming = append(page.Upcoming, c)
		} else {
			page.Past = append(page.Past, c)
		}
	}
	slices.Reverse(page.Past)
	return page, nil
}

// CancelBooking cancels the calling student's own booking of a session before it starts, and frees the
// place (AC-7, AC-88). It runs while the session is locked, the same lock a booking takes, so the freed
// place goes to exactly one later booking (AC-50). Only the caller's own booking can be cancelled: a
// session they did not book is "not found" (AC-49).
func (l *Labs) CancelBooking(ctx context.Context, caller Caller, sessionID int64) (SessionCard, error) {
	if err := RequireStudent(caller); err != nil {
		return SessionCard{}, err
	}
	now := l.now()
	v, err := l.store.CancelBooking(ctx, caller.ID, sessionID, func(s store.SessionView) error {
		if !now.Before(s.StartsAt) {
			return ErrCancelBookingStarted
		}
		return nil
	})
	var se *Error
	switch {
	case err == nil:
		return l.card(v), nil
	case errors.As(err, &se):
		return SessionCard{}, err
	case errors.Is(err, store.ErrNoBooking):
		return SessionCard{}, ErrBookingNotFound
	case errors.Is(err, store.ErrNotFound):
		return SessionCard{}, ErrSessionNotFound
	}
	return SessionCard{}, fmt.Errorf("cancel booking: %w", err)
}
