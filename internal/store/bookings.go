package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// BookingCheck decides whether a booking may go ahead. It runs inside the booking transaction while
// the session's row is locked, so s.Booked cannot change under it. mine reports whether the user has
// already booked the session.
type BookingCheck func(s SessionView, mine bool) error

// BookSession adds a booking for userID when check allows it, and returns the session with its new
// booking count. Every booking locks its session's row first, so concurrent bookings of one session run
// one at a time and the last place cannot be taken twice (AC-6). A missing session gives ErrNotFound, a
// second booking by the same user ErrDuplicate, and an error from check is returned unchanged.
func (s *Store) BookSession(ctx context.Context, userID, sessionID int64, check BookingCheck) (SessionView, error) {
	var v SessionView
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT s.id, l.id, l.title, l.description, l.place_limit, l.owner_id, o.name,
			       s.starts_at, s.ends_at, s.cancelled_at
			FROM `+sessionsFrom+`
			WHERE s.id = $1
			FOR UPDATE OF s`, sessionID).Scan(&v.SessionID, &v.LabID, &v.Title, &v.Description, &v.PlaceLimit,
			&v.OwnerID, &v.OwnerName, &v.StartsAt, &v.EndsAt, &v.CancelledAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// Counted after the lock, in a new statement, so it sees every booking committed before it.
		var mine bool
		if err := tx.QueryRow(ctx, `
			SELECT count(*), coalesce(bool_or(user_id = $2), false)
			FROM bookings
			WHERE session_id = $1`, sessionID, userID).Scan(&v.Booked, &mine); err != nil {
			return err
		}
		if err := check(v, mine); err != nil {
			return err
		}
		// The unique (user_id, session_id) constraint backs up the check for a second booking.
		_, err = tx.Exec(ctx, `INSERT INTO bookings (user_id, session_id) VALUES ($1, $2)`, userID, sessionID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrDuplicate
		}
		if err != nil {
			return err
		}
		v.Booked++
		return nil
	})
	if err != nil {
		return SessionView{}, err
	}
	return v, nil
}
