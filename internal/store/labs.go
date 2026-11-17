package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// pgUniqueViolation is PostgreSQL's SQLSTATE for a broken unique constraint.
const pgUniqueViolation = "23505"

type Lab struct {
	ID          int64
	Title       string
	Description string
	StartsAt    time.Time
	EndsAt      time.Time
	PlaceLimit  int
	// Booked is the lab's number of bookings rows, filled in by the queries that say so.
	Booked int
}

// Booker is a student who booked a lab.
type Booker struct {
	Name  string
	Email string
}

// CreateLab inserts a lab and returns it with its id. A title that is already taken gives ErrDuplicate.
func (s *Store) CreateLab(ctx context.Context, l Lab) (Lab, error) {
	err := s.db.QueryRow(ctx, `
		INSERT INTO labs (title, description, starts_at, ends_at, place_limit)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		l.Title, l.Description, l.StartsAt, l.EndsAt, l.PlaceLimit).Scan(&l.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return Lab{}, ErrDuplicate
	}
	if err != nil {
		return Lab{}, err
	}
	return l, nil
}

// LabByID returns one lab with its number of bookings.
func (s *Store) LabByID(ctx context.Context, id int64) (Lab, error) {
	var l Lab
	err := s.db.QueryRow(ctx, `
		SELECT l.id, l.title, l.description, l.starts_at, l.ends_at, l.place_limit,
		       (SELECT count(*) FROM bookings b WHERE b.lab_id = l.id)
		FROM labs l
		WHERE l.id = $1`, id).Scan(&l.ID, &l.Title, &l.Description, &l.StartsAt, &l.EndsAt, &l.PlaceLimit, &l.Booked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Lab{}, ErrNotFound
	}
	return l, err
}

// LabBookers returns the students who booked a lab, ordered by name.
func (s *Store) LabBookers(ctx context.Context, labID int64) ([]Booker, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.name, u.email
		FROM bookings b
		JOIN users u ON u.id = b.user_id
		WHERE b.lab_id = $1
		ORDER BY u.name, u.email`, labID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Booker])
}

// LabsAvailableBetween returns labs whose time period overlaps [from, to), each with its number of bookings.
func (s *Store) LabsAvailableBetween(ctx context.Context, from, to time.Time) ([]Lab, error) {
	rows, err := s.db.Query(ctx, `
		SELECT l.id, l.title, l.description, l.starts_at, l.ends_at, l.place_limit,
		       (SELECT count(*) FROM bookings b WHERE b.lab_id = l.id)
		FROM labs l
		WHERE l.starts_at < $2 AND l.ends_at > $1
		ORDER BY l.starts_at, l.title`, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Lab, error) {
		var l Lab
		err := row.Scan(&l.ID, &l.Title, &l.Description, &l.StartsAt, &l.EndsAt, &l.PlaceLimit, &l.Booked)
		return l, err
	})
}

// UpsertLab creates or refreshes a seeded lab, matched on title.
func (s *Store) UpsertLab(ctx context.Context, l Lab) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO labs (title, description, starts_at, ends_at, place_limit)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (title) DO UPDATE
		SET description = EXCLUDED.description, starts_at = EXCLUDED.starts_at,
		    ends_at = EXCLUDED.ends_at, place_limit = EXCLUDED.place_limit`,
		l.Title, l.Description, l.StartsAt, l.EndsAt, l.PlaceLimit)
	return err
}
