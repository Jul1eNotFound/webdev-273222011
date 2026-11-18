package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

type LabStore interface {
	CreateLab(ctx context.Context, l store.Lab) (store.Lab, error)
	LabByID(ctx context.Context, id int64) (store.Lab, error)
	LabsOwnedBy(ctx context.Context, ownerID int64, now time.Time) ([]store.OwnedLab, error)
	SessionsBetween(ctx context.Context, from, to time.Time) ([]store.SessionView, error)
	SessionsOfLab(ctx context.Context, labID int64) ([]store.SessionView, error)
	SessionBookers(ctx context.Context, labID int64) ([]store.SessionBooker, error)
	AddSessions(ctx context.Context, labID int64, periods []store.Period) ([]int64, error)
	BookSession(ctx context.Context, userID, sessionID int64, check store.BookingCheck) (store.SessionView, error)
	SessionByID(ctx context.Context, id int64) (store.SessionView, error)
	CancelSession(ctx context.Context, sessionID int64, now time.Time, check store.CancelCheck) (store.SessionView, error)
	DeleteLab(ctx context.Context, labID int64) error
	CancelBooking(ctx context.Context, userID, sessionID int64, check store.CancelBookingCheck) (store.SessionView, error)
	BookingsOf(ctx context.Context, userID int64) ([]store.SessionView, error)
	SessionsBookedBy(ctx context.Context, userID int64, sessionIDs []int64) ([]int64, error)
}

// MsgBooked and MsgBookingCancelled confirm a booking and its cancellation. The web pages, the booking
// script and the MCP tools all show these, so they cannot drift apart (AC-37).
const (
	MsgBooked           = "Booking confirmed."
	MsgBookingCancelled = "Booking cancelled."
)

// DateTimeLayout is how a session's start and end are typed: the value of an HTML datetime-local input.
const DateTimeLayout = "2006-01-02T15:04"

// DateLayout is how a date is typed: the value of an HTML date input.
const DateLayout = "2006-01-02"

const maxTitleLen = 200

// NewLab is the add-lab form as typed. Add parses it, so HTTP and MCP get the same messages.
// A lab has no time period of its own: its sessions do (Slice 3b).
type NewLab struct {
	Title       string
	Description string
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

// Location is the timezone session times are typed and shown in.
func (l *Labs) Location() *time.Location { return l.loc }

// SessionCard is one session as the pages list it: with its lab, its free places, and whether booking
// it is still open.
type SessionCard struct {
	store.SessionView
	FreePlaces int
	// Started is true once the session has started: booking is closed (AC-83).
	Started bool
	// Cancelled is true once the owner cancelled it (Slice 3c).
	Cancelled bool
	// Mine is true when the calling student has booked it (AC-89).
	Mine bool
}

// LabPage is a lab with its sessions that have not ended, for /labs/{id}.
type LabPage struct {
	Lab      store.Lab
	Sessions []SessionCard
}

// card adds what the pages need to know about a session at this moment.
func (l *Labs) card(v store.SessionView) SessionCard {
	return SessionCard{
		SessionView: v,
		FreePlaces:  freePlaces(v),
		Started:     !l.now().Before(v.StartsAt),
		Cancelled:   v.CancelledAt != nil,
	}
}

func (l *Labs) cards(views []store.SessionView) []SessionCard {
	out := make([]SessionCard, len(views))
	for i, v := range views {
		out[i] = l.card(v)
	}
	return out
}

// freePlaces is the lab's place limit minus the session's bookings, never below zero.
func freePlaces(v store.SessionView) int {
	return max(v.PlaceLimit-v.Booked, 0)
}

// midnight is the start of t's calendar day in the server's timezone ("today" in SPEC's decisions).
func (l *Labs) midnight(t time.Time) time.Time {
	t = t.In(l.loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, l.loc)
}

// DayList is the lab list for one day (AC-80).
type DayList struct {
	// Day is the shown day's midnight in the server's timezone.
	Day   time.Time
	Today bool
	// Date, Prev and Next are the shown, previous and next day as YYYY-MM-DD, for links and the date field.
	Date, Prev, Next string
	Sessions         []SessionCard
}

// ListDay returns the sessions whose time period includes part of one day, today when date is empty,
// each with its lab and free places (AC-10, AC-80). Any signed-in user may call it. A date that is not
// a real YYYY-MM-DD day gives ErrInvalidDate (AC-81).
func (l *Labs) ListDay(ctx context.Context, caller Caller, date string) (DayList, error) {
	if caller.ID == 0 {
		return DayList{}, ErrUnauthenticated
	}
	today := l.midnight(l.now())
	day := today
	if date != "" {
		d, err := time.ParseInLocation(DateLayout, date, l.loc)
		if err != nil {
			return DayList{}, ErrInvalidDate
		}
		day = d
	}
	// AddDate, not 24 hours: a day with a daylight saving change is 23 or 25 hours long.
	next := day.AddDate(0, 0, 1)
	views, err := l.store.SessionsBetween(ctx, day, next)
	if err != nil {
		return DayList{}, fmt.Errorf("list sessions: %w", err)
	}
	cards, err := l.markMine(ctx, caller, l.cards(views))
	if err != nil {
		return DayList{}, err
	}
	return DayList{
		Day:      day,
		Today:    day.Equal(today),
		Date:     day.Format(DateLayout),
		Prev:     day.AddDate(0, 0, -1).Format(DateLayout),
		Next:     next.Format(DateLayout),
		Sessions: cards,
	}, nil
}

// markMine sets Mine on the sessions a student caller has booked, with one query, so the page can show
// "Booked" (AC-89). Staff book nothing, so they need no query.
func (l *Labs) markMine(ctx context.Context, caller Caller, cards []SessionCard) ([]SessionCard, error) {
	if caller.Role != RoleStudent || len(cards) == 0 {
		return cards, nil
	}
	ids := make([]int64, len(cards))
	for i, c := range cards {
		ids[i] = c.SessionID
	}
	mine, err := l.store.SessionsBookedBy(ctx, caller.ID, ids)
	if err != nil {
		return nil, fmt.Errorf("find own bookings: %w", err)
	}
	for i := range cards {
		cards[i].Mine = slices.Contains(mine, cards[i].SessionID)
	}
	return cards, nil
}

// ListToday returns today's sessions: ListDay without a date.
func (l *Labs) ListToday(ctx context.Context, caller Caller) ([]SessionCard, error) {
	list, err := l.ListDay(ctx, caller, "")
	return list.Sessions, err
}

// lab looks a lab up and turns a missing one into ErrLabNotFound (AC-47).
func (l *Labs) lab(ctx context.Context, id int64) (store.Lab, error) {
	lab, err := l.store.LabByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return store.Lab{}, ErrLabNotFound
	}
	if err != nil {
		return store.Lab{}, fmt.Errorf("look up lab: %w", err)
	}
	return lab, nil
}

// Get returns a lab and its sessions that have not ended. Any signed-in user may call it.
func (l *Labs) Get(ctx context.Context, caller Caller, id int64) (LabPage, error) {
	if caller.ID == 0 {
		return LabPage{}, ErrUnauthenticated
	}
	lab, err := l.lab(ctx, id)
	if err != nil {
		return LabPage{}, err
	}
	views, err := l.store.SessionsOfLab(ctx, id)
	if err != nil {
		return LabPage{}, fmt.Errorf("list sessions: %w", err)
	}
	page := LabPage{Lab: lab}
	for _, v := range views {
		if v.EndsAt.After(l.now()) {
			page.Sessions = append(page.Sessions, l.card(v))
		}
	}
	if page.Sessions, err = l.markMine(ctx, caller, page.Sessions); err != nil {
		return LabPage{}, err
	}
	return page, nil
}

// NewLabForm allows the empty add-lab form to staff only (AC-15). The handler asks here instead of
// checking the role itself, so every role check stays in the service.
func (l *Labs) NewLabForm(caller Caller) error {
	return RequireStaff(caller)
}

// Add creates a lab owned by the caller (AC-60). Staff only (AC-15); the place limit must be 1 or more
// (AC-13) and the title unused (AC-2). The lab has no sessions yet: its owner adds them (AC-67).
func (l *Labs) Add(ctx context.Context, caller Caller, in NewLab) (store.Lab, error) {
	if err := RequireStaff(caller); err != nil {
		return store.Lab{}, err
	}
	lab, err := parseLab(in)
	if err != nil {
		return store.Lab{}, err
	}
	lab.OwnerID = caller.ID
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

func parseLab(in NewLab) (store.Lab, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return store.Lab{}, ErrTitleRequired
	}
	if utf8.RuneCountInString(title) > maxTitleLen {
		return store.Lab{}, ErrTitleTooLong
	}
	// 32 bits is what the place_limit column holds.
	limit, err := strconv.ParseInt(strings.TrimSpace(in.PlaceLimit), 10, 32)
	if err != nil || limit < 1 {
		return store.Lab{}, ErrInvalidPlaceLimit
	}
	return store.Lab{
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		PlaceLimit:  int(limit),
	}, nil
}

// MyLabs returns the labs the calling staff member owns, each with its session count and next session
// (AC-62).
func (l *Labs) MyLabs(ctx context.Context, caller Caller) ([]store.OwnedLab, error) {
	if err := RequireStaff(caller); err != nil {
		return nil, err
	}
	labs, err := l.store.LabsOwnedBy(ctx, caller.ID, l.now())
	if err != nil {
		return nil, fmt.Errorf("list own labs: %w", err)
	}
	return labs, nil
}
