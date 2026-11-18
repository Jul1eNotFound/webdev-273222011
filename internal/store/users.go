package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type User struct {
	ID           int64
	Email        string
	Name         string
	PasswordHash string
	Role         string
	// GoogleID is the linked Google account's id ("sub"), or "" when none is linked (Slice 6).
	GoogleID string
}

// userColumns is what scanUser reads, in its order.
const userColumns = `u.id, u.email, u.name, u.password, u.role, COALESCE(u.google_id, '')`

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users u WHERE u.email = $1`, email))
}

// UserByGoogleID returns the user a Google account is linked to, or ErrNotFound.
func (s *Store) UserByGoogleID(ctx context.Context, googleID string) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users u WHERE u.google_id = $1`, googleID))
}

// LinkGoogle attaches a Google account to a user who has none yet. ErrNotFound means the user has one
// already (or does not exist); ErrDuplicate means that Google account is linked to another user. The
// database decides both, so two sign-ins racing cannot both link.
func (s *Store) LinkGoogle(ctx context.Context, userID int64, googleID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE users SET google_id = $2 WHERE id = $1 AND google_id IS NULL`, userID, googleID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertUser creates or refreshes a seeded account. A linked Google account is kept.
func (s *Store) UpsertUser(ctx context.Context, u User) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO users (email, name, password, role) VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE
		SET name = EXCLUDED.name, password = EXCLUDED.password, role = EXCLUDED.role`,
		u.Email, u.Name, u.PasswordHash, u.Role)
	return err
}

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.GoogleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}
