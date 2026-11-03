// Package seed creates the demo accounts and labs. Running it again refreshes them to the current day.
package seed

import (
	"context"
	"errors"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

const (
	StudentEmail = "student@labwiz.test"
	StaffEmail   = "staff@labwiz.test"
)

type Passwords struct {
	Student string
	Staff   string
}

// Run seeds one student, one staff member, three labs available today and one that starts tomorrow.
func Run(ctx context.Context, st *store.Store, now time.Time, loc *time.Location, pw Passwords) error {
	if pw.Student == "" || pw.Staff == "" {
		return errors.New("both seed passwords must be set")
	}
	accounts := []struct{ email, name, password, role string }{
		{StudentEmail, "Sam Student", pw.Student, "student"},
		{StaffEmail, "Stella Staff", pw.Staff, "staff"},
	}
	for _, a := range accounts {
		hash, err := auth.Hash(a.password)
		if err != nil {
			return err
		}
		if err := st.UpsertUser(ctx, store.User{Email: a.email, Name: a.name, PasswordHash: hash, Role: a.role}); err != nil {
			return err
		}
	}

	t := now.In(loc)
	today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	at := func(day, hour int) time.Time { return today.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour) }
	labs := []store.Lab{
		{Title: "Chemistry basics", Description: "Titration and safe handling of reagents.", StartsAt: at(0, 9), EndsAt: at(0, 12), PlaceLimit: 10},
		{Title: "Circuits workshop", Description: "Breadboards, resistors and a multimeter.", StartsAt: at(0, 13), EndsAt: at(0, 17), PlaceLimit: 2},
		{Title: "Open microscopy", Description: "Drop in and use the light microscopes.", StartsAt: at(0, 8), EndsAt: at(0, 20), PlaceLimit: 5},
		{Title: "Robotics intro", Description: "Program a line-following robot.", StartsAt: at(1, 9), EndsAt: at(1, 12), PlaceLimit: 8},
	}
	for _, l := range labs {
		if err := st.UpsertLab(ctx, l); err != nil {
			return err
		}
	}
	return nil
}
