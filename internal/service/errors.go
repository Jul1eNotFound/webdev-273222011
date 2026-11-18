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

	// Google sign-in (Slice 6). Each is one sentence for the sign-in page.
	ErrGoogleNoAccount    = &Error{KindUnauthenticated, "No Labwiz account uses this Google address. Ask staff to add you."}
	ErrGoogleUnverified   = &Error{KindUnauthenticated, "Google has not verified this email address, so it cannot be used to sign in."}
	ErrGoogleOtherAccount = &Error{KindUnauthenticated, "This Labwiz account is linked to a different Google account."}

	ErrLabNotFound       = &Error{KindNotFound, "This lab does not exist."}
	ErrDuplicateTitle    = &Error{KindConflict, "A lab with this title already exists. Please choose a different title."}
	ErrTitleRequired     = &Error{KindValidation, "Please enter a title."}
	ErrTitleTooLong      = &Error{KindValidation, "The title must be 200 characters or fewer."}
	ErrInvalidPlaceLimit = &Error{KindValidation, "The place limit must be a whole number of 1 or more."}
	// ErrInvalidDate is a ?date= on /labs that is not a real YYYY-MM-DD day (AC-81).
	ErrInvalidDate = &Error{KindValidation, "Please choose a valid date."}

	// Booking a session. The sentences still say "lab": that is what a student books (AC-1, AC-5).
	ErrSessionNotFound = &Error{KindNotFound, "This session does not exist."}
	ErrLabFull         = &Error{KindConflict, "This lab is full."}
	ErrAlreadyBooked   = &Error{KindConflict, "You have already booked this lab."}
	ErrSessionStarted  = &Error{KindConflict, "Booking is closed: this session has already started."}
	// Cancelling one's own booking (AC-49, AC-88).
	ErrBookingNotFound      = &Error{KindNotFound, "You have no booking for this session."}
	ErrCancelBookingStarted = &Error{KindConflict, "This session has already started, so the booking cannot be cancelled."}
	// ErrSessionCancelled refuses booking a session its owner cancelled (AC-74).
	ErrSessionCancelled = &Error{KindConflict, "This session has been cancelled."}

	// Cancelling sessions and deleting labs (AC-75, AC-77).
	ErrCancelStarted    = &Error{KindConflict, "This session has already started."}
	ErrAlreadyCancelled = &Error{KindConflict, "This session is already cancelled."}
	ErrLabHasBookings   = &Error{KindConflict, "This lab has bookings. Cancel its sessions instead."}

	// Scheduling sessions (AC-68 to AC-70).
	ErrInvalidPeriod     = &Error{KindValidation, "Please enter a valid start and end time."}
	ErrPeriodOrder       = &Error{KindValidation, "The end time must be after the start time."}
	ErrSessionInPast     = &Error{KindValidation, "A session cannot start in the past."}
	ErrInvalidRepeat     = &Error{KindValidation, "Please enter a valid date to repeat the session until."}
	ErrRepeatBeforeStart = &Error{KindValidation, "The repeat date must be on or after the session's date."}
	ErrRepeatTooLong     = &Error{KindValidation, "A weekly repeat can add at most 26 sessions."}
	ErrSessionOverlap    = &Error{KindConflict, "This lab already has a session at that time."}
)
