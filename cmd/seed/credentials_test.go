package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/seed"
)

func TestCredentialsKeepPasswordsBetweenRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed-credentials.txt")
	earlier, err := loadPasswords(path)
	if err != nil || len(earlier) != 0 {
		t.Fatalf("missing file: %v, %v; want an empty map", earlier, err)
	}
	first := withPasswords(seed.DemoAccounts(), earlier)
	seen := map[string]bool{}
	for _, a := range first {
		if len(a.Password) < 16 || seen[a.Password] {
			t.Fatalf("%s: weak or repeated password %q", a.Email, a.Password)
		}
		seen[a.Password] = true
	}
	if err := writeCredentials(path, first); err != nil {
		t.Fatal(err)
	}

	again, err := loadPasswords(path)
	if err != nil {
		t.Fatal(err)
	}
	second := withPasswords(seed.DemoAccounts(), again)
	for i := range first {
		if second[i].Password != first[i].Password {
			t.Errorf("%s: password changed between runs", first[i].Email)
		}
	}
	if _, ok := again[seed.StudentEmail]; ok {
		t.Error("the main student's .env password was read back as a demo password")
	}
}

func TestCredentialsFileNeverHoldsTheEnvPasswords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed-credentials.txt")
	if err := writeCredentials(path, withPasswords(seed.DemoAccounts(), nil)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{seed.StaffEmail, "(SEED_STAFF_PASSWORD in .env)", seed.StudentEmail, "(SEED_STUDENT_PASSWORD in .env)", "dana@labwiz.test"} {
		if !strings.Contains(text, want) {
			t.Errorf("credentials file lacks %q", want)
		}
	}
	for _, v := range []string{os.Getenv("SEED_STUDENT_PASSWORD"), os.Getenv("SEED_STAFF_PASSWORD")} {
		if v != "" && strings.Contains(text, v) {
			t.Error("credentials file contains a password from .env")
		}
	}
}
