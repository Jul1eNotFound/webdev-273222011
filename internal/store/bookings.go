package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// BookingCheck decides whether a booking may go ahead. It runs inside the booking transaction while
// the lab's row is locked, so lab.Booked cannot change under it. mine reports whether the user has
// already booked the lab.
type BookingCheck func(lab Lab, mine bool) error

// BookLab adds a booking for userID when check allows it, and returns the lab with its new booking count.
// Every booking locks its lab's row first, so concurrent bookings of one lab run one at a time and the
// last place cannot be taken twice (AC-6). A missing lab gives ErrNotFound, a second booking by the same
// user ErrDuplicate, and an error from check is returned unchanged.
func (s *Store) BookLab(ctx context.Context, userID, labID int64, check BookingCheck) (Lab, error) {
	var lab Lab
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT id, title, description, starts_at, ends_at, place_limit
			FROM labs
			WHERE id = $1
			FOR UPDATE`, labID).Scan(&lab.ID, &lab.Title, &lab.Description, &lab.StartsAt, &lab.EndsAt, &lab.PlaceLimit)
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
			WHERE lab_id = $1`, labID, userID).Scan(&lab.Booked, &mine); err != nil {
			return err
		}
		if err := check(lab, mine); err != nil {
			return err
		}
		// The unique (user_id, lab_id) constraint backs up the check for a second booking.
		_, err = tx.Exec(ctx, `INSERT INTO bookings (user_id, lab_id) VALUES ($1, $2)`, userID, labID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrDuplicate
		}
		if err != nil {
			return err
		}
		lab.Booked++
		return nil
	})
	if err != nil {
		return Lab{}, err
	}
	return lab, nil
}
