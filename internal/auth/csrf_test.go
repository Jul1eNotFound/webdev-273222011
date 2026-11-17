package auth

import (
	"strings"
	"testing"
)

const testSecret = "test-secret-that-is-at-least-32-characters-long"

func TestCSRFTokenIsPerSession_AC86(t *testing.T) {
	c, err := NewCSRF(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	a, again, b := c.Token("session-a"), c.Token("session-a"), c.Token("session-b")
	if a != again || a == b || a == "" {
		t.Fatalf("tokens: a=%q again=%q b=%q; want a stable token per session", a, again, b)
	}
	other, err := NewCSRF(strings.Repeat("x", 40))
	if err != nil {
		t.Fatal(err)
	}
	if other.Token("session-a") == a {
		t.Fatal("a different secret gives the same token")
	}
}

func TestCSRFValidAcceptsOnlyTheSessionsOwnToken_AC85_AC86(t *testing.T) {
	c, _ := NewCSRF(testSecret)
	good := c.Token("session-a")
	if !c.Valid("session-a", good) {
		t.Fatal("the session's own token is refused")
	}
	for name, got := range map[string]string{
		"empty": "", "changed": good[:len(good)-1] + "x", "another session's": c.Token("session-b"), "the session token itself": "session-a",
	} {
		if c.Valid("session-a", got) {
			t.Errorf("%s token is accepted", name)
		}
	}
	if c.Valid("", c.Token("")) {
		t.Error("a request without a session is accepted")
	}
	if (CSRF{}).Valid("session-a", good) {
		t.Error("a CSRF without a secret accepts tokens")
	}
}

func TestNewCSRFRefusesAShortSecret(t *testing.T) {
	for _, s := range []string{"", "change-me", strings.Repeat("x", 31)} {
		if _, err := NewCSRF(s); err == nil || !strings.Contains(err.Error(), "SESSION_SECRET") {
			t.Errorf("%d characters: err = %v, want a refusal naming SESSION_SECRET", len(s), err)
		}
	}
	if _, err := NewCSRF(strings.Repeat("x", 32)); err != nil {
		t.Errorf("32 characters: %v", err)
	}
}
