// Package auth holds password hashing and the session cookie.
package auth

import "golang.org/x/crypto/bcrypt"

// dummyHash is compared against when the email is unknown, so both paths take as long (AC-17).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("labwiz-dummy-password"), bcrypt.DefaultCost)

// Hash returns the bcrypt hash of a password. Only the hash is ever stored (AC-18).
func Hash(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(h), err
}

// Bcrypt implements service.PasswordChecker.
type Bcrypt struct{}

func (Bcrypt) Compare(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func (Bcrypt) CompareDummy(plain string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(plain))
}
