package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID           int64
	Email        string
	Name         string
	PasswordHash string
	Role         string
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.db.QueryRow(ctx,
		`SELECT id, email, name, password, role FROM users WHERE email = $1`, email))
}

// UpsertUser creates or refreshes a seeded account.
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
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}
