package session

import (
	"errors"
	"net/http"
	"time"
)

const CookieName = "file_share_access"

func SetCookie(w http.ResponseWriter, rawToken string, lifetime time.Duration, secure bool) error {
	if rawToken == "" {
		return errors.New("session token must not be empty")
	}
	if lifetime < time.Second {
		return errors.New("cookie lifetime must be at least one second")
	}
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(lifetime / time.Second),
		Expires:  time.Now().UTC().Add(lifetime),
	})
	return nil
}

func ClearCookie(w http.ResponseWriter, secure bool) {
	w.Header().Set("Cache-Control", "no-store")
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
	})
}
