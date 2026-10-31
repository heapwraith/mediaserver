package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"
	"uuid"

	"mediaserver/internal/library"
)

// ProfileHeader carries the session token of the current profile.
const ProfileHeader = "X-Profile"

// sessionToken signs a profile id: "<id>.<hmac>". The PIN hash is part of
// the signed data, so changing or removing the PIN ends the old sessions.
func sessionToken(secret []byte, profileID, pinHash string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(profileID + "|" + pinHash))
	return profileID + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type profileKey struct{}

// profileFrom returns the profile of a request that went through requireProfile.
func profileFrom(ctx context.Context) library.Profile {
	p, _ := ctx.Value(profileKey{}).(library.Profile)
	return p
}

// requireProfiles answers 503 when profiles are not available.
func (d Deps) requireProfiles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Library.DB == nil || len(d.Secret) == 0 {
			writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "profiles not available", Code: "unavailable"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireProfile checks the session token of the X-Profile header and puts
// its profile in the request context; 401 without a valid one.
func (d Deps) requireProfile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(ProfileHeader)
		id, _, _ := strings.Cut(token, ".")
		if _, err := uuid.Parse(id); err != nil {
			writeJSON(w, http.StatusUnauthorized, errorBody{Error: "choose a profile", Code: "no_profile"})
			return
		}
		p, hash, err := d.Library.ProfileAuth(r.Context(), id)
		if err != nil || !hmac.Equal([]byte(token), []byte(sessionToken(d.Secret, id, hash))) {
			if err != nil && !isNotFound(err) {
				d.writeError(w, r, err)
				return
			}
			writeJSON(w, http.StatusUnauthorized, errorBody{Error: "choose a profile", Code: "no_profile"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), profileKey{}, p)))
	})
}

// PIN attempts: after pinMaxFails wrong PINs a profile is locked for pinLock.
const (
	pinMaxFails = 5
	pinLock     = 30 * time.Second
)

// pinLimiter slows down guessing a 4-digit PIN. It lives in memory: a
// restart clears it, which is fine for a home server.
type pinLimiter struct {
	mu    sync.Mutex
	now   func() time.Time
	fails map[string]pinFails
}

type pinFails struct {
	count int
	until time.Time
}

func newPinLimiter() *pinLimiter {
	return &pinLimiter{now: time.Now, fails: map[string]pinFails{}}
}

// wait is how long a profile is still locked (0 = it may try).
func (l *pinLimiter) wait(id string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.fails[id]
	if left := f.until.Sub(l.now()); left > 0 {
		return left
	}
	return 0
}

// fail counts a wrong PIN and reports whether the profile got locked.
func (l *pinLimiter) fail(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.fails[id]
	f.count++
	if f.count >= pinMaxFails {
		l.fails[id] = pinFails{until: l.now().Add(pinLock)}
		return true
	}
	l.fails[id] = f
	return false
}

func (l *pinLimiter) reset(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, id)
}
