// Package config reads every environment variable the app uses, in one place.
package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Port          string
	DatabaseURL   string
	PublicBaseURL string

	// Only cmd/seed needs these. They are never stored as plain text.
	SeedStudentPassword string
	SeedStaffPassword   string
}

func Load() (Config, error) {
	c := Config{
		Port:                getenv("PORT", "3000"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		PublicBaseURL:       getenv("PUBLIC_BASE_URL", "http://localhost:3000"),
		SeedStudentPassword: os.Getenv("SEED_STUDENT_PASSWORD"),
		SeedStaffPassword:   os.Getenv("SEED_STAFF_PASSWORD"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is not set")
	}
	return c, nil
}

// SecureCookies is true when the app is served over https.
func (c Config) SecureCookies() bool {
	return strings.HasPrefix(c.PublicBaseURL, "https://")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
