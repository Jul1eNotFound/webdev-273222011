// Package store is the data layer and the only place that writes SQL.
package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned when a lookup matches no row.
	ErrNotFound = errors.New("not found")
	// ErrDuplicate is returned when an insert breaks a unique constraint.
	ErrDuplicate = errors.New("duplicate")
)

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}
