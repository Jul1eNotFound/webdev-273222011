package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSetCookieUsesMaxAgeNotExpires(t *testing.T) {
	w := httptest.NewRecorder()
	SetCookie(w, "token", 7*24*time.Hour, true)
	got := w.Header().Get("Set-Cookie")
	for _, want := range []string{CookieName + "=token", "Max-Age=604800", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(got, want) {
			t.Errorf("Set-Cookie %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "Expires=") {
		t.Errorf("Set-Cookie %q has an absolute Expires", got)
	}
}
