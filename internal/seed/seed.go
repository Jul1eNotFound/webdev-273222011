// Package seed creates the demo accounts and labs. Running it again refreshes them to the current day.
package seed

import (
	"context"
	"errors"
	"fmt"
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

// Run seeds one student, one staff member, and four labs that the staff member owns, each with one
// session: three today and one tomorrow. Running it again on another day adds that day's sessions.
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
	owner, err := st.UserByEmail(ctx, StaffEmail)
	if err != nil {
		return err
	}

	t := now.In(loc)
	today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	at := func(day, hour int) time.Time { return today.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour) }
	labs := []struct {
		lab     store.Lab
		session store.Period
	}{
		{store.Lab{Title: "Chemistry basics", Description: "Titration and safe handling of reagents.", PlaceLimit: 10}, store.Period{StartsAt: at(0, 9), EndsAt: at(0, 12)}},
		{store.Lab{Title: "Circuits workshop", Description: "Breadboards, resistors and a multimeter.", PlaceLimit: 2}, store.Period{StartsAt: at(0, 13), EndsAt: at(0, 17)}},
		{store.Lab{Title: "Open microscopy", Description: "Drop in and use the light microscopes.", PlaceLimit: 5}, store.Period{StartsAt: at(0, 8), EndsAt: at(0, 20)}},
		{store.Lab{Title: "Robotics intro", Description: "Program a line-following robot.", PlaceLimit: 8}, store.Period{StartsAt: at(1, 9), EndsAt: at(1, 12)}},
	}
	for _, l := range labs {
		l.lab.OwnerID = owner.ID
		id, err := st.UpsertLab(ctx, l.lab)
		if err != nil {
			return err
		}
		if _, err := st.EnsureSession(ctx, id, l.session); err != nil {
			return fmt.Errorf("session of %q: %w", l.lab.Title, err)
		}
	}
	return nil
}
