package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrOverlap is returned when a session would overlap another live session of the same lab (AC-69).
var ErrOverlap = errors.New("overlapping session")

// Period is a start and an end.
type Period struct {
	StartsAt time.Time
	EndsAt   time.Time
}

// SessionView is one session of a lab, with what the pages show about its lab. It is flat, so templates
// can say .Title and .StartsAt without reaching into nested structs.
type SessionView struct {
	SessionID   int64
	LabID       int64
	Title       string
	Description string
	PlaceLimit  int
	OwnerID     int64
	OwnerName   string
	StartsAt    time.Time
	EndsAt      time.Time
	// CancelledAt is set once the owner cancels the session (Slice 3c).
	CancelledAt *time.Time
	// Booked is the session's number of bookings rows.
	Booked int
}

// SessionBooker is a student who booked one session of a lab.
type SessionBooker struct {
	SessionID int64
	Booker
}

// sessionColumns is what every query that shows sessions selects, in the order scanSession reads it.
const sessionColumns = `s.id, l.id, l.title, l.description, l.place_limit, l.owner_id, o.name,
	s.starts_at, s.ends_at, s.cancelled_at,
	(SELECT count(*) FROM bookings b WHERE b.session_id = s.id)`

// sessionsFrom is the FROM clause that goes with sessionColumns.
const sessionsFrom = `lab_sessions s JOIN labs l ON l.id = s.lab_id JOIN users o ON o.id = l.owner_id`

func scanSession(row pgx.Row) (SessionView, error) {
	var v SessionView
	err := row.Scan(&v.SessionID, &v.LabID, &v.Title, &v.Description, &v.PlaceLimit, &v.OwnerID, &v.OwnerName,
		&v.StartsAt, &v.EndsAt, &v.CancelledAt, &v.Booked)
	return v, err
}

func collectSessions(rows pgx.Rows, err error) ([]SessionView, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SessionView, error) { return scanSession(row) })
}

// SessionsBetween returns the sessions whose time period overlaps [from, to), cancelled ones included,
// each with its lab, owner and number of bookings.
func (s *Store) SessionsBetween(ctx context.Context, from, to time.Time) ([]SessionView, error) {
	return collectSessions(s.db.Query(ctx, `
		SELECT `+sessionColumns+`
		FROM `+sessionsFrom+`
		WHERE s.starts_at < $2 AND s.ends_at > $1
		ORDER BY s.starts_at, l.title, s.id`, from, to))
}

// SessionsOfLab returns all of a lab's sessions, past and cancelled ones included, in time order.
func (s *Store) SessionsOfLab(ctx context.Context, labID int64) ([]SessionView, error) {
	return collectSessions(s.db.Query(ctx, `
		SELECT `+sessionColumns+`
		FROM `+sessionsFrom+`
		WHERE s.lab_id = $1
		ORDER BY s.starts_at, s.id`, labID))
}

// SessionBookers returns who booked each session of a lab, by session and then by name (AC-14).
func (s *Store) SessionBookers(ctx context.Context, labID int64) ([]SessionBooker, error) {
	rows, err := s.db.Query(ctx, `
		SELECT b.session_id, u.name, u.email
		FROM bookings b
		JOIN lab_sessions s ON s.id = b.session_id
		JOIN users u ON u.id = b.user_id
		WHERE s.lab_id = $1
		ORDER BY b.session_id, u.name, u.email`, labID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SessionBooker, error) {
		var b SessionBooker
		err := row.Scan(&b.SessionID, &b.Name, &b.Email)
		return b, err
	})
}

// AddSessions adds sessions to a lab, all or none (AC-70). One that overlaps a live session of the lab,
// or another one in the same call, gives ErrOverlap and nothing is added (AC-69). A missing lab gives
// ErrNotFound.
func (s *Store) AddSessions(ctx context.Context, labID int64, periods []Period) ([]int64, error) {
	ids := make([]int64, 0, len(periods))
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		for _, p := range periods {
			var id int64
			err := tx.QueryRow(ctx, `
				INSERT INTO lab_sessions (lab_id, starts_at, ends_at) VALUES ($1, $2, $3) RETURNING id`,
				labID, p.StartsAt, p.EndsAt).Scan(&id)
			if err != nil {
				return sessionInsertError(err)
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func sessionInsertError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgExclusionViolation:
			return ErrOverlap
		case pgForeignKeyViolation:
			return ErrNotFound
		}
	}
	return err
}

// EnsureSession returns the id of the lab's session with exactly this period, cancelled or not, adding it
// if there is none. The seed uses it, so running the seed again adds nothing: the overlap constraint alone
// would not do, since it ignores a cancelled session and would let a live copy of it in. A different
// session in the way gives ErrOverlap.
func (s *Store) EnsureSession(ctx context.Context, labID int64, p Period) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `
		SELECT id FROM lab_sessions
		WHERE lab_id = $1 AND starts_at = $2 AND ends_at = $3
		ORDER BY cancelled_at NULLS FIRST, id
		LIMIT 1`, labID, p.StartsAt, p.EndsAt).Scan(&id)
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO lab_sessions (lab_id, starts_at, ends_at) VALUES ($1, $2, $3) RETURNING id`,
		labID, p.StartsAt, p.EndsAt).Scan(&id)
	return id, sessionInsertError(err)
}
