package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

type LabStore interface {
	LabsAvailableBetween(ctx context.Context, from, to time.Time) ([]store.Lab, error)
}

type Labs struct {
	store LabStore
	now   Clock
	loc   *time.Location
}

// NewLabs takes the timezone that decides where "today" starts and ends.
func NewLabs(s LabStore, now Clock, loc *time.Location) *Labs {
	return &Labs{store: s, now: now, loc: loc}
}

// ListToday returns the labs whose time period includes part of today. Any signed-in user may call it.
func (l *Labs) ListToday(ctx context.Context, caller Caller) ([]store.Lab, error) {
	if caller.ID == 0 {
		return nil, ErrUnauthenticated
	}
	t := l.now().In(l.loc)
	from := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, l.loc)
	labs, err := l.store.LabsAvailableBetween(ctx, from, from.AddDate(0, 0, 1))
	if err != nil {
		return nil, fmt.Errorf("list labs: %w", err)
	}
	return labs, nil
}
