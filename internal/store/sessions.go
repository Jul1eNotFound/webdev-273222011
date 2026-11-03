package store

import (
	"context"
	"time"
)

func (s *Store) CreateSession(ctx context.Context, idHash []byte, userID int64, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO sessions (id_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		idHash, userID, expiresAt)
	return err
}

// SessionUser returns the user of a session that has not expired at now.
func (s *Store) SessionUser(ctx context.Context, idHash []byte, now time.Time) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `
		SELECT u.id, u.email, u.name, u.password, u.role
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id_hash = $1 AND s.expires_at > $2`, idHash, now))
}

func (s *Store) DeleteSession(ctx context.Context, idHash []byte) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE id_hash = $1`, idHash)
	return err
}
