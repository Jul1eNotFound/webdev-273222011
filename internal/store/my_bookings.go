package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrNoBooking is returned when a user cancels a booking they do not have (AC-49).
var ErrNoBooking = errors.New("no booking")

// CancelBookingCheck decides whether a booking may be cancelled. It runs inside the transaction while the
// session's row is locked.
type CancelBookingCheck func(s SessionView) error

// CancelBooking deletes userID's booking of a session when check allows it, and returns the session with
// its new booking count. It locks the session's row first, the same lock a booking takes, so a cancel and
// a booking of one session run one at a time and the freed place goes to exactly one booking (AC-50).
// A missing session gives ErrNotFound, a session the user has not booked ErrNoBooking, and an error from
// check is returned unchanged. The delete is keyed on userID, so nobody can cancel another's booking.
func (s *Store) CancelBooking(ctx context.Context, userID, sessionID int64, check CancelBookingCheck) (SessionView, error) {
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
		var mine bool
		if err := tx.QueryRow(ctx, `
			SELECT count(*), coalesce(bool_or(user_id = $2), false)
			FROM bookings
			WHERE session_id = $1`, sessionID, userID).Scan(&v.Booked, &mine); err != nil {
			return err
		}
		if !mine {
			return ErrNoBooking
		}
		if err := check(v); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM bookings WHERE user_id = $1 AND session_id = $2`, userID, sessionID); err != nil {
			return err
		}
		v.Booked--
		return nil
	})
	if err != nil {
		return SessionView{}, err
	}
	return v, nil
}

// BookingsOf returns the sessions a user booked, cancelled ones included, in time order (AC-90).
func (s *Store) BookingsOf(ctx context.Context, userID int64) ([]SessionView, error) {
	return collectSessions(s.db.Query(ctx, `
		SELECT `+sessionColumns+`
		FROM `+sessionsFrom+`
		WHERE EXISTS (SELECT 1 FROM bookings mb WHERE mb.session_id = s.id AND mb.user_id = $1)
		ORDER BY s.starts_at, l.title, s.id`, userID))
}

// SessionsBookedBy returns which of the given sessions a user booked, so a list can show "Booked" (AC-89).
func (s *Store) SessionsBookedBy(ctx context.Context, userID int64, sessionIDs []int64) ([]int64, error) {
	if len(sessionIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT session_id FROM bookings WHERE user_id = $1 AND session_id = ANY($2)`, userID, sessionIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}
