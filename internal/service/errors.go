// Package service holds the business rules. It knows nothing about HTTP or MCP.
package service

// Kind tells a handler which status code an Error maps to.
type Kind int

const (
	KindValidation Kind = iota + 1
	KindUnauthenticated
	KindForbidden
	KindNotFound
	KindConflict
)

// Error is a rule violation whose Message is one readable sentence, safe to show to the user.
type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string { return e.Message }

var (
	// ErrInvalidCredentials is the same for an unknown email and a wrong password (AC-17).
	ErrInvalidCredentials = &Error{KindUnauthenticated, "Invalid email or password."}
	ErrUnauthenticated    = &Error{KindUnauthenticated, "Please sign in to continue."}
)
