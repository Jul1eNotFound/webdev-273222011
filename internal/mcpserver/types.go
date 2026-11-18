package mcpserver

import (
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// Lab is a lab as the tools return it.
type Lab struct {
	LabID       int64  `json:"lab_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Owner       string `json:"owner" jsonschema:"the name of the staff member who runs the lab"`
	PlaceLimit  int    `json:"place_limit" jsonschema:"how many students may book each session"`
}

// Booker is a student who booked a session.
type Booker struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Session is one scheduled run of a lab. Times are RFC 3339 in the server's timezone.
type Session struct {
	SessionID   int64  `json:"session_id"`
	LabID       int64  `json:"lab_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
	PlaceLimit  int    `json:"place_limit"`
	FreePlaces  int    `json:"free_places"`
	Started     bool   `json:"started" jsonschema:"true once the session has started: it can no longer be booked or cancelled"`
	Cancelled   bool   `json:"cancelled" jsonschema:"true when the lab's owner cancelled the session"`
	BookedByMe  bool   `json:"booked_by_me" jsonschema:"true when the calling student has booked this session"`
	// Bookers is present only where the caller may see who booked: for the lab's owner.
	Bookers *[]Booker `json:"bookers,omitempty" jsonschema:"the students who booked the session; present for the lab's owner only"`
}

func lab(l store.Lab) Lab {
	return Lab{LabID: l.ID, Title: l.Title, Description: l.Description, Owner: l.OwnerName, PlaceLimit: l.PlaceLimit}
}

func (s *server) session(c service.SessionCard) Session {
	loc := s.labs.Location()
	return Session{
		SessionID: c.SessionID, LabID: c.LabID, Title: c.Title, Description: c.Description, Owner: c.OwnerName,
		StartsAt: c.StartsAt.In(loc).Format(time.RFC3339), EndsAt: c.EndsAt.In(loc).Format(time.RFC3339),
		PlaceLimit: c.PlaceLimit, FreePlaces: c.FreePlaces, Started: c.Started, Cancelled: c.Cancelled, BookedByMe: c.Mine,
	}
}

// sessions never returns nil, so an empty list is [] in JSON, not null.
func (s *server) sessions(cards []service.SessionCard) []Session {
	out := make([]Session, 0, len(cards))
	for _, c := range cards {
		out = append(out, s.session(c))
	}
	return out
}

// withBookers is a session of the owner's lab with who booked it.
func (s *server) withBookers(list []service.SessionBookers) []Session {
	out := make([]Session, 0, len(list))
	for _, x := range list {
		out = append(out, s.session(x.SessionCard))
		out[len(out)-1].Bookers = bookers(x.Bookers)
	}
	return out
}

func bookers(bs []store.Booker) *[]Booker {
	out := make([]Booker, 0, len(bs))
	for _, b := range bs {
		out = append(out, Booker{Name: b.Name, Email: b.Email})
	}
	return &out
}
