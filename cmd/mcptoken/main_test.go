package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Jul1eNotFound/webdev-273222011/internal/auth"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

func newAuth(t *testing.T) *service.Auth {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("right-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	st := servicetest.NewStore()
	st.AddUser(store.User{ID: 1, Email: "sam@labwiz.test", Name: "Sam", PasswordHash: string(hash), Role: "student"})
	now := time.Date(2026, 10, 22, 8, 30, 0, 0, time.UTC)
	return service.NewAuth(st, auth.Bcrypt{}, func() time.Time { return now })
}

func TestIssuePrintsOnlyATokenThatSignsInAsTheAccount(t *testing.T) {
	a := newAuth(t)
	var out, info bytes.Buffer
	if err := issue(context.Background(), a, "sam@labwiz.test", "right-password", &out, &info); err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(out.String())
	if token == "" || strings.ContainsAny(token, " \n") {
		t.Fatalf("stdout = %q, want the token alone", out.String())
	}
	c, err := a.CurrentUser(context.Background(), token)
	if err != nil || c.ID != 1 {
		t.Fatalf("the token signs in as %+v, err %v", c, err)
	}
	if want := "Signed in as Sam (student). The token works until 29 Oct 2026 08:30."; strings.TrimSpace(info.String()) != want {
		t.Errorf("info = %q, want %q", info.String(), want)
	}
}

func TestIssueWithAWrongPasswordPrintsNoToken(t *testing.T) {
	var out, info bytes.Buffer
	err := issue(context.Background(), newAuth(t), "sam@labwiz.test", "wrong", &out, &info)
	if err != service.ErrInvalidCredentials || err.Error() != "Invalid email or password." {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", out.String())
	}
}

func TestFirstLineIsThePipedPassword(t *testing.T) {
	for in, want := range map[string]string{"secret\r\nignored": "secret", "secret\n": "secret", "secret": "secret", "": ""} {
		if got, err := firstLine(strings.NewReader(in)); err != nil || got != want {
			t.Errorf("firstLine(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
