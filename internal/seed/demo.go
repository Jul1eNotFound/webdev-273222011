package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// Account is a demo account. Password is the plain password, which only the caller knows: the
// database gets its bcrypt hash (AC-18).
type Account struct {
	Role     string
	Email    string
	Name     string
	Password string
}

// DemoAccounts are the extra accounts for trying the app by hand, without passwords. The caller
// fills in the passwords; nothing here is a secret.
func DemoAccounts() []Account {
	return []Account{
		{Role: "staff", Email: "dana@labwiz.test", Name: "Dana Novak"},
		{Role: "staff", Email: "omar@labwiz.test", Name: "Omar Haddad"},
		{Role: "student", Email: "ana@labwiz.test", Name: "Ana Ivanova"},
		{Role: "student", Email: "ben@labwiz.test", Name: "Ben Carter"},
		{Role: "student", Email: "chen@labwiz.test", Name: "Chen Li"},
		{Role: "student", Email: "diego@labwiz.test", Name: "Diego Ramos"},
		{Role: "student", Email: "eva@labwiz.test", Name: "Eva Petrova"},
		{Role: "student", Email: "farah@labwiz.test", Name: "Farah Khan"},
	}
}

type demoLab struct {
	lab   store.Lab
	owner string // email of the demo staff member who owns it
	// sessions are the lab's sessions; the first one is the one the demo students booked.
	sessions []store.Period
	bookers  []string // emails of the demo students who booked its first session
}

const (
	dana = "dana@labwiz.test"
	omar = "omar@labwiz.test"
)

// demoLabs covers the cases worth seeing by hand: free, nearly full and full sessions today, one that
// runs past midnight, a lab that runs weekly, labs on later days, and one that is already over.
func demoLabs(now time.Time, loc *time.Location) []demoLab {
	t := now.In(loc)
	today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	at := func(day, hour int) time.Time { return today.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour) }
	lab := func(owner, title, desc string, from, to time.Time, limit int, bookers ...string) demoLab {
		return demoLab{
			lab: store.Lab{Title: title, Description: desc, PlaceLimit: limit}, owner: owner,
			sessions: []store.Period{{StartsAt: from, EndsAt: to}}, bookers: bookers,
		}
	}
	weekly := func(d demoLab, weeks int) demoLab {
		first := d.sessions[0]
		for w := 1; w < weeks; w++ {
			d.sessions = append(d.sessions, store.Period{StartsAt: first.StartsAt.AddDate(0, 0, 7*w), EndsAt: first.EndsAt.AddDate(0, 0, 7*w)})
		}
		return d
	}
	return []demoLab{
		weekly(lab(dana, "Pendulum timing", "Measure g with a pendulum and a stopwatch.", at(0, 10), at(0, 12), 6,
			"ana@labwiz.test"), 3), // every week for three weeks
		lab(dana, "Biology dissection", "Plant and flower dissection under the stereo microscope.", at(0, 14), at(0, 16), 3,
			"ana@labwiz.test", "ben@labwiz.test", "chen@labwiz.test"), // full
		lab(omar, "3D printing basics", "Slice a model and print it in PLA.", at(0, 15), at(0, 18), 4,
			"diego@labwiz.test", "eva@labwiz.test", "farah@labwiz.test"), // one place left
		lab(dana, "Evening astronomy", "Telescope session on the roof, weather permitting.", at(0, 21), at(1, 1), 12,
			"ben@labwiz.test", "eva@labwiz.test"), // runs past midnight
		lab(omar, "Soldering practice", "Solder a small LED kit. Safety glasses provided.", at(1, 9), at(1, 11), 5,
			"chen@labwiz.test"),
		lab(dana, "Spectroscopy", "Identify gases from their emission lines.", at(1, 13), at(1, 15), 4),
		lab(dana, "Organic chemistry", "Synthesis of aspirin.", at(2, 10), at(2, 13), 8),
		lab(omar, "Machine learning lab", "Train a small image classifier on the lab GPUs.", at(7, 14), at(7, 17), 20),
		lab(dana, "Water quality testing", "Test river samples for pH and nitrates.", at(-1, 9), at(-1, 12), 6,
			"diego@labwiz.test"), // already over
	}
}

// errSkip tells BookLab to leave a demo booking out: it exists already, or the lab is full.
var errSkip = errors.New("skip")

// Demo adds the demo accounts, labs and bookings. Running it again refreshes the labs to the current
// day and the accounts to the given passwords, and adds no duplicate bookings.
func Demo(ctx context.Context, st *store.Store, now time.Time, loc *time.Location, accounts []Account) error {
	ids := map[string]int64{}
	for _, a := range accounts {
		if a.Password == "" {
			return fmt.Errorf("demo account %s has no password", a.Email)
		}
		hash, err := auth.Hash(a.Password)
		if err != nil {
			return err
		}
		if err := st.UpsertUser(ctx, store.User{Email: a.Email, Name: a.Name, PasswordHash: hash, Role: a.Role}); err != nil {
			return err
		}
		u, err := st.UserByEmail(ctx, a.Email)
		if err != nil {
			return err
		}
		ids[a.Email] = u.ID
	}
	skipTaken := func(s store.SessionView, mine bool) error {
		if mine || s.Booked >= s.PlaceLimit {
			return errSkip
		}
		return nil
	}
	for _, d := range demoLabs(now, loc) {
		ownerID, ok := ids[d.owner]
		if !ok {
			return fmt.Errorf("demo lab %q is owned by unknown account %s", d.lab.Title, d.owner)
		}
		d.lab.OwnerID = ownerID
		labID, err := st.UpsertLab(ctx, d.lab)
		if err != nil {
			return err
		}
		var booked int64
		for i, p := range d.sessions {
			id, err := st.EnsureSession(ctx, labID, p)
			if err != nil {
				return fmt.Errorf("session of %q: %w", d.lab.Title, err)
			}
			if i == 0 {
				booked = id
			}
		}
		for _, email := range d.bookers {
			userID, ok := ids[email]
			if !ok {
				return fmt.Errorf("demo lab %q is booked by unknown account %s", d.lab.Title, email)
			}
			if _, err := st.BookSession(ctx, userID, booked, skipTaken); err != nil && !errors.Is(err, errSkip) {
				return fmt.Errorf("book %q for %s: %w", d.lab.Title, email, err)
			}
		}
	}
	return nil
}
