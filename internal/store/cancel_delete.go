package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrInUse is returned when a lab cannot be deleted because one of its sessions has bookings (AC-77).
var ErrInUse = errors.New("in use")

// CancelCheck decides whether a session may be cancelled. It runs inside the cancel transaction while the
// session's row is locked.
type CancelCheck func(s SessionView) error

// SessionByID returns one session with its lab, owner and number of bookings.
func (s *Store) SessionByID(ctx context.Context, id int64) (SessionView, error) {
	v, err := scanSession(s.db.QueryRow(ctx, `SELECT `+sessionColumns+` FROM `+sessionsFrom+` WHERE s.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionView{}, ErrNotFound
	}
	return v, err
}

// CancelSession marks a session cancelled at now when check allows it, and returns it. It locks the
// session's row first, the same lock a booking takes, so a booking that waited for the cancel sees the
// session cancelled (AC-74). Its bookings are kept (AC-73). A missing session gives ErrNotFound, and an
// error from check is returned unchanged.
func (s *Store) CancelSession(ctx context.Context, sessionID int64, now time.Time, check CancelCheck) (SessionView, error) {
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
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE session_id = $1`, sessionID).Scan(&v.Booked); err != nil {
			return err
		}
		if err := check(v); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE lab_sessions SET cancelled_at = $2 WHERE id = $1`, sessionID, now); err != nil {
			return err
		}
		v.CancelledAt = &now
		return nil
	})
	if err != nil {
		return SessionView{}, err
	}
	return v, nil
}

// DeleteLab deletes a lab and, through ON DELETE CASCADE, its sessions. If any of its sessions has a
// booking, past and cancelled ones included, the RESTRICT foreign key on bookings refuses it and nothing is
// deleted: ErrInUse (AC-77). The database decides, not a prior lookup, so a booking made while the delete
// runs is never deleted with the lab (AC-78). A missing lab gives ErrNotFound.
func (s *Store) DeleteLab(ctx context.Context, labID int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM labs WHERE id = $1`, labID)
	// RESTRICT reports 23001; NO ACTION, if the key is ever changed to it, reports 23503.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == pgRestrictViolation || pgErr.Code == pgForeignKeyViolation) {
		return ErrInUse
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
