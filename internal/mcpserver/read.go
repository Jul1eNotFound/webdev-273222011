package mcpserver

import (
	"context"
	"errors"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
)

type listLabsIn struct {
	Date string `json:"date,omitempty" jsonschema:"the day to list, as YYYY-MM-DD; today when left out"`
}

type listLabsOut struct {
	Date     string    `json:"date"`
	Sessions []Session `json:"sessions"`
}

type labIDIn struct {
	LabID int64 `json:"lab_id" jsonschema:"the lab's id, as list_labs returns it"`
}

type getLabOut struct {
	Lab      Lab       `json:"lab"`
	Sessions []Session `json:"sessions" jsonschema:"the lab's sessions that have not ended, in time order"`
}

type myBookingsOut struct {
	Upcoming []Session `json:"upcoming" jsonschema:"sessions that have not ended, in time order"`
	Past     []Session `json:"past" jsonschema:"sessions that have ended, most recent first"`
}

type labBookingsOut struct {
	Lab      Lab       `json:"lab"`
	Upcoming []Session `json:"upcoming" jsonschema:"sessions that have not ended, in time order, each with its bookers"`
	Past     []Session `json:"past" jsonschema:"sessions that have ended, most recent first, each with its bookers"`
}

func (s *server) addReadTools() {
	addTool(s, "list_labs",
		"Lists the lab sessions on one day (today by default): lab title, description, owner, time period, place limit, free places, and whether it is cancelled or already booked by you. Any signed-in user.",
		func(ctx context.Context, c service.Caller, in listLabsIn) (listLabsOut, error) {
			day, err := s.labs.ListDay(ctx, c, in.Date)
			if err != nil {
				return listLabsOut{}, err
			}
			return listLabsOut{Date: day.Date, Sessions: s.sessions(day.Sessions)}, nil
		})

	addTool(s, "get_lab",
		"Returns one lab by id with its owner, place limit and the sessions that have not ended, with their free places. The lab's owner also gets who booked each session. Any signed-in user.",
		s.getLab)

	addTool(s, "my_bookings",
		"Lists the calling student's own bookings, upcoming and past, with lab title, session time and whether staff cancelled the session. Students only.",
		func(ctx context.Context, c service.Caller, _ struct{}) (myBookingsOut, error) {
			page, err := s.labs.MyBookings(ctx, c)
			if err != nil {
				return myBookingsOut{}, err
			}
			return myBookingsOut{Upcoming: s.sessions(page.Upcoming), Past: s.sessions(page.Past)}, nil
		})

	addTool(s, "list_lab_bookings",
		"Lists the students who booked each session of one lab, upcoming and past. Only the staff member who owns the lab may call it.",
		func(ctx context.Context, c service.Caller, in labIDIn) (labBookingsOut, error) {
			page, err := s.labs.Bookings(ctx, c, in.LabID)
			if err != nil {
				return labBookingsOut{}, err
			}
			return labBookingsOut{Lab: lab(page.Lab), Upcoming: s.withBookers(page.Upcoming), Past: s.withBookers(page.Past)}, nil
		})
}

// getLab is the lab page's data. The booking lists come from Labs.Bookings, which only the owner may
// call: its ErrForbidden means "not the owner", and the lab is returned without them (AC-36, AC-91).
// The service decides who owns the lab; this only leaves out what it refused.
func (s *server) getLab(ctx context.Context, c service.Caller, in labIDIn) (getLabOut, error) {
	page, err := s.labs.Get(ctx, c, in.LabID)
	if err != nil {
		return getLabOut{}, err
	}
	out := getLabOut{Lab: lab(page.Lab), Sessions: s.sessions(page.Sessions)}
	owner, err := s.labs.Bookings(ctx, c, in.LabID)
	switch {
	case errors.Is(err, service.ErrForbidden):
		return out, nil
	case err != nil:
		return getLabOut{}, err
	}
	who := map[int64]*[]Booker{}
	for _, x := range owner.Upcoming {
		who[x.SessionID] = bookers(x.Bookers)
	}
	for i := range out.Sessions {
		if b, ok := who[out.Sessions[i].SessionID]; ok {
			out.Sessions[i].Bookers = b
		} else {
			out.Sessions[i].Bookers = bookers(nil)
		}
	}
	return out, nil
}
