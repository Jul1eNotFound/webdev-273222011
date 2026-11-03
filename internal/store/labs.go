package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type Lab struct {
	ID          int64
	Title       string
	Description string
	StartsAt    time.Time
	EndsAt      time.Time
	PlaceLimit  int
}

// LabsAvailableBetween returns labs whose time period overlaps [from, to).
func (s *Store) LabsAvailableBetween(ctx context.Context, from, to time.Time) ([]Lab, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, title, description, starts_at, ends_at, place_limit
		FROM labs
		WHERE starts_at < $2 AND ends_at > $1
		ORDER BY starts_at, title`, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Lab, error) {
		var l Lab
		err := row.Scan(&l.ID, &l.Title, &l.Description, &l.StartsAt, &l.EndsAt, &l.PlaceLimit)
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
