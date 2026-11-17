package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes the store turns into its own errors.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgRestrictViolation   = "23001"
	pgExclusionViolation  = "23P01"
)

// Lab is what a lab is apart from its schedule: its sessions carry the time periods (Slice 3b).
type Lab struct {
	ID          int64
	Title       string
	Description string
	PlaceLimit  int
	// OwnerID is the staff member who added the lab (AC-60).
	OwnerID int64
	// OwnerName is the owner's name, filled in by the queries that show labs.
	OwnerName string
}

// Booker is a student who booked a session.
type Booker struct {
	Name  string
	Email string
}

// OwnedLab is a lab on its owner's "My labs" page.
type OwnedLab struct {
	Lab
	// Sessions counts all of the lab's sessions, past and cancelled ones included.
	Sessions int
	// Next is the lab's next live session that has not ended, or nil.
	Next *Period
}

// labColumns is what every query that shows a lab selects, in the order scanLab reads it.
const labColumns = `l.id, l.title, l.description, l.place_limit, l.owner_id, o.name`

// labsWithOwner is the FROM clause that goes with labColumns.
const labsWithOwner = `labs l JOIN users o ON o.id = l.owner_id`

func scanLab(row pgx.Row) (Lab, error) {
	var l Lab
	err := row.Scan(&l.ID, &l.Title, &l.Description, &l.PlaceLimit, &l.OwnerID, &l.OwnerName)
	return l, err
}

// CreateLab inserts a lab and returns it with its id. A title that is already taken gives ErrDuplicate.
func (s *Store) CreateLab(ctx context.Context, l Lab) (Lab, error) {
	err := s.db.QueryRow(ctx, `
		INSERT INTO labs (title, description, place_limit, owner_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		l.Title, l.Description, l.PlaceLimit, l.OwnerID).Scan(&l.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return Lab{}, ErrDuplicate
	}
	if err != nil {
		return Lab{}, err
	}
	return l, nil
}

// LabByID returns one lab with its owner's name.
func (s *Store) LabByID(ctx context.Context, id int64) (Lab, error) {
	l, err := scanLab(s.db.QueryRow(ctx, `SELECT `+labColumns+` FROM `+labsWithOwner+` WHERE l.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lab{}, ErrNotFound
	}
	return l, err
}

// LabsOwnedBy returns the labs a staff member owns, each with its number of sessions and its next live
// session that has not ended by now (AC-62). Labs with a next session come first, soonest first.
func (s *Store) LabsOwnedBy(ctx context.Context, ownerID int64, now time.Time) ([]OwnedLab, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+labColumns+`,
		       (SELECT count(*) FROM lab_sessions s WHERE s.lab_id = l.id),
		       n.starts_at, n.ends_at
		FROM `+labsWithOwner+`
		LEFT JOIN LATERAL (
			SELECT s.starts_at, s.ends_at FROM lab_sessions s
			WHERE s.lab_id = l.id AND s.ends_at > $2 AND s.cancelled_at IS NULL
			ORDER BY s.starts_at
			LIMIT 1
		) n ON true
		WHERE l.owner_id = $1
		ORDER BY n.starts_at NULLS LAST, l.title`, ownerID, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (OwnedLab, error) {
		var o OwnedLab
		var from, to *time.Time
		err := row.Scan(&o.ID, &o.Title, &o.Description, &o.PlaceLimit, &o.OwnerID, &o.OwnerName, &o.Sessions, &from, &to)
		if from != nil && to != nil {
			o.Next = &Period{StartsAt: *from, EndsAt: *to}
		}
		return o, err
	})
}

// UpsertLab creates or refreshes a seeded lab, matched on title, and returns its id.
func (s *Store) UpsertLab(ctx context.Context, l Lab) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO labs (title, description, place_limit, owner_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (title) DO UPDATE
		SET description = EXCLUDED.description, place_limit = EXCLUDED.place_limit, owner_id = EXCLUDED.owner_id
		RETURNING id`,
		l.Title, l.Description, l.PlaceLimit, l.OwnerID).Scan(&id)
	return id, err
}
