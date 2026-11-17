package service_test

import (
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
)

// day is the fixed clock's day. The clock (auth_test.go) says 10:00 on it.
var day = time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)

// at is the given hour of the fixed clock's day; 24 and more are on later days, negative ones earlier.
func at(h int) time.Time { return day.Add(time.Duration(h) * time.Hour) }

// sess is a session of a lab from one hour of the fixed day to another.
func sess(id, labID int64, from, to int) servicetest.LabSession {
	return servicetest.LabSession{ID: id, LabID: labID, StartsAt: at(from), EndsAt: at(to)}
}

var (
	staff   = service.Caller{ID: 2, Name: "Stella", Role: service.RoleStaff}
	student = service.Caller{ID: 7, Name: "Sam", Role: service.RoleStudent}
	// otherStaff is a second staff member, who owns none of staff's labs.
	otherStaff = service.Caller{ID: 3, Name: "Omar", Role: service.RoleStaff}
)
