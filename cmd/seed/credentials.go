package main

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
)

// fromEnv marks an account whose password lives in .env, so this file never copies it.
const fromEnv = "("

// loadPasswords reads the passwords of an earlier credentials file, by email, so they stay the same
// between seeds. A missing file gives an empty map.
func loadPasswords(path string) (map[string]string, error) {
	passwords := map[string]string{}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return passwords, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] == "role" || strings.HasPrefix(fields[2], fromEnv) {
			continue
		}
		passwords[fields[1]] = fields[2]
	}
	return passwords, sc.Err()
}

// withPasswords gives every account its earlier password, or a new random one.
func withPasswords(accounts []seed.Account, earlier map[string]string) []seed.Account {
	out := make([]seed.Account, len(accounts))
	for i, a := range accounts {
		a.Password = earlier[a.Email]
		if a.Password == "" {
			a.Password = rand.Text()
		}
		out[i] = a
	}
	return out
}

// writeCredentials writes every account's login. The two main accounts point at their .env variable
// instead of repeating its value. The file is readable by its owner only.
func writeCredentials(path string, demo []seed.Account) error {
	var b strings.Builder
	b.WriteString("# Labwiz demo accounts for trying the app by hand. Written by `go run ./cmd/seed`.\n")
	b.WriteString("# Git ignores this file: never commit it. Delete it to get new passwords on the next seed.\n\n")
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "role\temail\tpassword\tname")
	fmt.Fprintf(w, "staff\t%s\t(SEED_STAFF_PASSWORD in .env)\tStella Staff\n", seed.StaffEmail)
	fmt.Fprintf(w, "student\t%s\t(SEED_STUDENT_PASSWORD in .env)\tSam Student\n", seed.StudentEmail)
	for _, a := range demo {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Role, a.Email, a.Password, a.Name)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
