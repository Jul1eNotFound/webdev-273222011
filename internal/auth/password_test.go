package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashIsBcryptAndNotThePlainPassword_AC18(t *testing.T) {
	h, err := Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if h == "correct horse" {
		t.Fatal("hash equals the plain password")
	}
	if _, err := bcrypt.Cost([]byte(h)); err != nil {
		t.Fatalf("not a bcrypt hash: %v", err)
	}
	var b Bcrypt
	if !b.Compare(h, "correct horse") || b.Compare(h, "wrong") {
		t.Fatal("Compare gave the wrong answer")
	}
	b.CompareDummy("anything") // must not panic
}
