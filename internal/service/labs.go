package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

type LabStore interface {
	LabsAvailableBetween(ctx context.Context, from, to time.Time) ([]store.Lab, error)
	CreateLab(ctx context.Context, l store.Lab) (store.Lab, error)
	LabByID(ctx context.Context, id int64) (store.Lab, error)
	LabBookers(ctx context.Context, labID int64) ([]store.Booker, error)
}

// DateTimeLayout is how a lab's start and end are typed: the value of an HTML datetime-local input.
const DateTimeLayout = "2006-01-02T15:04"

const maxTitleLen = 200

// NewLab is the add-lab form as typed. Add parses it, so HTTP and MCP get the same messages.
type NewLab struct {
	Title       string
	Description string
	Starts      string
	Ends        string
	PlaceLimit  string
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

// Location is the timezone lab times are typed and shown in.
func (l *Labs) Location() *time.Location { return l.loc }

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

// Add creates a lab. Staff only (AC-15); the place limit must be 1 or more (AC-13) and the title unused (AC-2).
func (l *Labs) Add(ctx context.Context, caller Caller, in NewLab) (store.Lab, error) {
	if err := RequireStaff(caller); err != nil {
		return store.Lab{}, err
	}
	lab, err := l.parse(in)
	if err != nil {
		return store.Lab{}, err
	}
	// The unique constraint decides, so two staff adding the same title at once cannot both succeed.
	lab, err = l.store.CreateLab(ctx, lab)
	if errors.Is(err, store.ErrDuplicate) {
		return store.Lab{}, ErrDuplicateTitle
	}
	if err != nil {
		return store.Lab{}, fmt.Errorf("create lab: %w", err)
	}
	return lab, nil
}

func (l *Labs) parse(in NewLab) (store.Lab, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return store.Lab{}, ErrTitleRequired
	}
	if utf8.RuneCountInString(title) > maxTitleLen {
		return store.Lab{}, ErrTitleTooLong
	}
	starts, errStarts := time.ParseInLocation(DateTimeLayout, strings.TrimSpace(in.Starts), l.loc)
	ends, errEnds := time.ParseInLocation(DateTimeLayout, strings.TrimSpace(in.Ends), l.loc)
	if errStarts != nil || errEnds != nil {
		return store.Lab{}, ErrInvalidPeriod
	}
	if !ends.After(starts) {
		return store.Lab{}, ErrPeriodOrder
	}
	// 32 bits is what the place_limit column holds.
	limit, err := strconv.ParseInt(strings.TrimSpace(in.PlaceLimit), 10, 32)
	if err != nil || limit < 1 {
		return store.Lab{}, ErrInvalidPlaceLimit
	}
	return store.Lab{
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		StartsAt:    starts,
		EndsAt:      ends,
		PlaceLimit:  int(limit),
	}, nil
}

// Bookings returns a lab and the students who booked it (AC-14). Staff only, checked before the
// lookup, so a student cannot even learn whether a lab id exists (AC-45).
func (l *Labs) Bookings(ctx context.Context, caller Caller, id int64) (store.Lab, []store.Booker, error) {
	if err := RequireStaff(caller); err != nil {
		return store.Lab{}, nil, err
	}
	lab, err := l.store.LabByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Lab{}, nil, ErrLabNotFound
	}
	if err != nil {
		return store.Lab{}, nil, fmt.Errorf("look up lab: %w", err)
	}
	bookers, err := l.store.LabBookers(ctx, id)
	if err != nil {
		return store.Lab{}, nil, fmt.Errorf("list bookers: %w", err)
	}
	return lab, bookers, nil
}
