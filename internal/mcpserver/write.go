package mcpserver

import (
	"context"
	"strconv"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

type sessionIDIn struct {
	SessionID int64 `json:"session_id" jsonschema:"the session's id, as list_labs or get_lab returns it"`
}

// bookingOut has the fields and sentence of the HTTP booking reply, so both clients see the same (AC-37).
type bookingOut struct {
	Message    string `json:"message"`
	FreePlaces int    `json:"free_places"`
	PlaceLimit int    `json:"place_limit"`
}

type addLabIn struct {
	Title       string `json:"title" jsonschema:"the lab's title, unique among all labs, at most 200 characters"`
	Description string `json:"description,omitempty" jsonschema:"what the lab is about"`
	PlaceLimit  int    `json:"place_limit" jsonschema:"how many students may book each session, 1 or more"`
}

type scheduleIn struct {
	LabID             int64  `json:"lab_id" jsonschema:"the lab's id; you must own the lab"`
	Starts            string `json:"starts" jsonschema:"when the session starts, as YYYY-MM-DDTHH:MM in the server's timezone"`
	Ends              string `json:"ends" jsonschema:"when the session ends, as YYYY-MM-DDTHH:MM in the server's timezone; it may be on a later day"`
	RepeatWeeklyUntil string `json:"repeat_weekly_until,omitempty" jsonschema:"optional last date, as YYYY-MM-DD, to repeat the session every week up to (at most 26 sessions)"`
}

type scheduleOut struct {
	Added int `json:"added" jsonschema:"how many sessions were added"`
}

func (s *server) addWriteTools() {
	addTool(s, "book_lab",
		"Books one place in a lab session for the calling student. Refused if the session is full, cancelled or has started, or you already booked it. Students only.",
		func(ctx context.Context, c service.Caller, in sessionIDIn) (bookingOut, error) {
			card, err := s.labs.Book(ctx, c, in.SessionID)
			if err != nil {
				return bookingOut{}, err
			}
			return bookingOut{Message: service.MsgBooked, FreePlaces: card.FreePlaces, PlaceLimit: card.PlaceLimit}, nil
		})

	addTool(s, "cancel_booking",
		"Cancels the calling student's own booking of a session before it starts, which frees the place. Refused for a booking you do not have or a session that has started. Students only.",
		func(ctx context.Context, c service.Caller, in sessionIDIn) (bookingOut, error) {
			card, err := s.labs.CancelBooking(ctx, c, in.SessionID)
			if err != nil {
				return bookingOut{}, err
			}
			return bookingOut{Message: service.MsgBookingCancelled, FreePlaces: card.FreePlaces, PlaceLimit: card.PlaceLimit}, nil
		})

	addTool(s, "add_lab",
		"Creates a lab owned by the calling staff member, with a title, a description and a place limit of 1 or more. It has no sessions until you schedule them. Refused if the title already exists. Staff only.",
		func(ctx context.Context, c service.Caller, in addLabIn) (Lab, error) {
			created, err := s.labs.Add(ctx, c, service.NewLab{
				Title: in.Title, Description: in.Description, PlaceLimit: strconv.Itoa(in.PlaceLimit),
			})
			if err != nil {
				return Lab{}, err
			}
			created.OwnerName = c.Name
			return lab(created), nil
		})

	addTool(s, "schedule_session",
		"Adds a session to a lab you own: a start and an end, and optionally a date to repeat it weekly until. Refused if it starts in the past or overlaps another session of the lab. Staff who own the lab only.",
		func(ctx context.Context, c service.Caller, in scheduleIn) (scheduleOut, error) {
			n, err := s.labs.AddSessions(ctx, c, in.LabID, service.NewSession{
				Starts: in.Starts, Ends: in.Ends, RepeatUntil: in.RepeatWeeklyUntil,
			})
			if err != nil {
				return scheduleOut{}, err
			}
			return scheduleOut{Added: n}, nil
		})

	addTool(s, "cancel_session",
		"Cancels an upcoming session of a lab you own. Its bookings are kept and the students see it as cancelled. Refused once the session has started. Staff who own the lab only.",
		func(ctx context.Context, c service.Caller, in sessionIDIn) (Session, error) {
			card, err := s.labs.CancelSession(ctx, c, in.SessionID)
			if err != nil {
				return Session{}, err
			}
			return s.session(card), nil
		})
}
