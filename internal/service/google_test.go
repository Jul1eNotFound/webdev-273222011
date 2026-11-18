package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jul1eNotFound/webdev-273222011/internal/service"
	"github.com/Jul1eNotFound/webdev-273222011/internal/service/servicetest"
	"github.com/Jul1eNotFound/webdev-273222011/internal/store"
)

// Sam (id 7, student@labwiz.test) has a local account with no Google account linked.

func verifiedSam() service.GoogleIdentity {
	return service.GoogleIdentity{Subject: "g-sam", Email: "Student@Labwiz.test", EmailVerified: true}
}

// signedInAs is the user a Google sign-in's session belongs to.
func signedInAs(t *testing.T, a *service.Auth, s service.Session) service.Caller {
	t.Helper()
	c, err := a.CurrentUser(context.Background(), s.Token)
	if err != nil {
		t.Fatalf("the session does not sign in: %v", err)
	}
	return c
}

func TestGoogleSignInLinksAVerifiedEmailToTheAccount_AC20(t *testing.T) {
	a, st, _ := newAuth()
	s, err := a.GoogleSignIn(context.Background(), verifiedSam())
	if err != nil {
		t.Fatal(err)
	}
	if c := signedInAs(t, a, s); c.ID != 7 {
		t.Fatalf("signed in as %+v, want Sam", c)
	}
	if got := st.User("student@labwiz.test").GoogleID; got != "g-sam" {
		t.Fatalf("google id = %q, want g-sam", got)
	}
}

func TestGoogleSignInWithAnUnverifiedEmailIsRefused_AC52(t *testing.T) {
	a, st, _ := newAuth()
	id := verifiedSam()
	id.EmailVerified = false
	if _, err := a.GoogleSignIn(context.Background(), id); err != service.ErrGoogleUnverified {
		t.Fatalf("err = %v, want ErrGoogleUnverified", err)
	}
	if st.User("student@labwiz.test").GoogleID != "" || len(st.Sessions()) != 0 {
		t.Fatal("an unverified address was linked or signed in")
	}
	if got := service.ErrGoogleUnverified.Error(); got != "Google has not verified this email address, so it cannot be used to sign in." {
		t.Errorf("message = %q", got)
	}
}

func TestGoogleSignInOfALinkedAccountWithAChangedEmail_AC53(t *testing.T) {
	a, st, _ := newAuth()
	st.SetGoogleID("student@labwiz.test", "g-sam")
	// Google now reports a different address, and not even a verified one: the id alone decides.
	s, err := a.GoogleSignIn(context.Background(), service.GoogleIdentity{Subject: "g-sam", Email: "sam.new@gmail.com"})
	if err != nil {
		t.Fatal(err)
	}
	if c := signedInAs(t, a, s); c.ID != 7 || c.Email != "student@labwiz.test" {
		t.Fatalf("signed in as %+v, want Sam with his email unchanged", c)
	}
}

func TestGoogleSignInWithoutAnAccountIsRefused(t *testing.T) {
	a, st, _ := newAuth()
	_, err := a.GoogleSignIn(context.Background(), service.GoogleIdentity{Subject: "g-x", Email: "nobody@gmail.com", EmailVerified: true})
	if err != service.ErrGoogleNoAccount || len(st.Sessions()) != 0 {
		t.Fatalf("err = %v, sessions %d; want ErrGoogleNoAccount and none", err, len(st.Sessions()))
	}
	if _, err := a.GoogleSignIn(context.Background(), service.GoogleIdentity{Email: "student@labwiz.test"}); err == nil {
		t.Error("an identity without a subject signed in")
	}
}

func TestGoogleSignInToAnAccountLinkedElsewhereIsRefused(t *testing.T) {
	a, st, _ := newAuth()
	st.SetGoogleID("student@labwiz.test", "g-other")
	if _, err := a.GoogleSignIn(context.Background(), verifiedSam()); err != service.ErrGoogleOtherAccount {
		t.Fatalf("err = %v, want ErrGoogleOtherAccount", err)
	}
	if st.User("student@labwiz.test").GoogleID != "g-other" {
		t.Fatal("the link changed")
	}
}

func TestGoogleSignInThatLosesALinkRaceIsDecidedAgain(t *testing.T) {
	// Another sign-in with the same Google account links Sam first: this one then finds the link.
	a, st, _ := newAuth()
	st.BeforeLinkGoogle = func() { st.SetGoogleID("student@labwiz.test", "g-sam") }
	s, err := a.GoogleSignIn(context.Background(), verifiedSam())
	if err != nil || signedInAs(t, a, s).ID != 7 {
		t.Fatalf("err = %v", err)
	}

	// Another Google account links Sam first: this one is refused.
	a, st, _ = newAuth()
	st.BeforeLinkGoogle = func() { st.SetGoogleID("student@labwiz.test", "g-other") }
	if _, err := a.GoogleSignIn(context.Background(), verifiedSam()); err != service.ErrGoogleOtherAccount {
		t.Fatalf("err = %v, want ErrGoogleOtherAccount", err)
	}

	// A link that keeps failing is not retried forever.
	a, st, _ = newAuth()
	st.ErrLinkGoogle = store.ErrNotFound
	if _, err := a.GoogleSignIn(context.Background(), verifiedSam()); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want the store's error after one retry", err)
	}
}

func TestGoogleSignInStoreErrors(t *testing.T) {
	for name, set := range map[string]func(*servicetest.Store){
		"by google id": func(s *servicetest.Store) { s.ErrUserByGoogleID = errDB },
		"by email":     func(s *servicetest.Store) { s.ErrUserByEmail = errDB },
		"link":         func(s *servicetest.Store) { s.ErrLinkGoogle = errDB },
		"session":      func(s *servicetest.Store) { s.ErrCreateSession = errDB },
	} {
		a, st, _ := newAuth()
		set(st)
		if _, err := a.GoogleSignIn(context.Background(), verifiedSam()); !errors.Is(err, errDB) {
			t.Errorf("%s: err = %v, want errDB", name, err)
		}
	}
}
