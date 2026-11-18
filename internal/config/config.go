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
	// SessionSecret signs the CSRF tokens (Slice 3e). Only the server needs it; it checks the length.
	SessionSecret string

	// Only cmd/seed needs these. They are never stored as plain text.
	SeedStudentPassword string
	SeedStaffPassword   string

	// Sign in with Google (Slice 6). Without both, the button is hidden.
	GoogleClientID     string
	GoogleClientSecret string
}

func Load() (Config, error) {
	c := Config{
		Port:                getenv("PORT", "3000"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		PublicBaseURL:       getenv("PUBLIC_BASE_URL", "http://localhost:3000"),
		SessionSecret:       os.Getenv("SESSION_SECRET"),
		SeedStudentPassword: os.Getenv("SEED_STUDENT_PASSWORD"),
		SeedStaffPassword:   os.Getenv("SEED_STAFF_PASSWORD"),
		GoogleClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is not set")
	}
	return c, nil
}

// GoogleEnabled is true when an OAuth client is configured for Sign in with Google.
func (c Config) GoogleEnabled() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

// GoogleRedirectURL is the callback URL to register with Google.
func (c Config) GoogleRedirectURL() string {
	return strings.TrimRight(c.PublicBaseURL, "/") + "/auth/google/callback"
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
