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
	ErrForbidden          = &Error{KindForbidden, "You are not allowed to do this."}

	ErrLabNotFound       = &Error{KindNotFound, "This lab does not exist."}
	ErrDuplicateTitle    = &Error{KindConflict, "A lab with this title already exists. Please choose a different title."}
	ErrTitleRequired     = &Error{KindValidation, "Please enter a title."}
	ErrTitleTooLong      = &Error{KindValidation, "The title must be 200 characters or fewer."}
	ErrInvalidPlaceLimit = &Error{KindValidation, "The place limit must be a whole number of 1 or more."}

	// Booking a session. The sentences still say "lab": that is what a student books (AC-1, AC-5).
	ErrSessionNotFound = &Error{KindNotFound, "This session does not exist."}
	ErrLabFull         = &Error{KindConflict, "This lab is full."}
	ErrAlreadyBooked   = &Error{KindConflict, "You have already booked this lab."}
	ErrSessionStarted  = &Error{KindConflict, "Booking is closed: this session has already started."}

	// Scheduling sessions (AC-68 to AC-70).
	ErrInvalidPeriod     = &Error{KindValidation, "Please enter a valid start and end time."}
	ErrPeriodOrder       = &Error{KindValidation, "The end time must be after the start time."}
	ErrSessionInPast     = &Error{KindValidation, "A session cannot start in the past."}
	ErrInvalidRepeat     = &Error{KindValidation, "Please enter a valid date to repeat the session until."}
	ErrRepeatBeforeStart = &Error{KindValidation, "The repeat date must be on or after the session's date."}
	ErrRepeatTooLong     = &Error{KindValidation, "A weekly repeat can add at most 26 sessions."}
	ErrSessionOverlap    = &Error{KindConflict, "This lab already has a session at that time."}
)
