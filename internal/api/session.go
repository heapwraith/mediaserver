// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"uuid"

	"mediaserver/internal/library"
)

// ProfileHeader carries the session token of the current profile.
const ProfileHeader = "X-Profile"

// sessionTTL is how long a session token is valid. After it, the profile is
// chosen again (and the PIN typed again, when it has one).
const sessionTTL = 90 * 24 * time.Hour

// sessionToken signs a profile id and an expiry: "<id>.<unix>.<hmac>". The
// PIN hash is part of the signed data, so changing or removing the PIN ends
// the old sessions.
func sessionToken(secret []byte, profileID, pinHash string, exp time.Time) string {
	unix := strconv.FormatInt(exp.Unix(), 10)
	return profileID + "." + unix + "." + sessionMAC(secret, profileID, pinHash, unix)
}

func sessionMAC(secret []byte, profileID, pinHash, unix string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(profileID + "|" + pinHash + "|" + unix))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// validToken checks the signature and the expiry of a token of profile id.
func validToken(secret []byte, token, id, pinHash string, now time.Time) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != id {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || now.Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(parts[2]), []byte(sessionMAC(secret, id, pinHash, parts[1])))
}

type profileKey struct{}

// profileFrom returns the profile of a request that went through requireProfile.
func profileFrom(ctx context.Context) library.Profile {
	p, _ := ctx.Value(profileKey{}).(library.Profile)
	return p
}

// secretBox holds the session secret; "sign out everywhere" replaces it.
type secretBox struct {
	mu sync.RWMutex
	v  []byte
}

func (b *secretBox) get() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.v
}

func (b *secretBox) set(v []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.v = v
}

// secret is the current session secret.
func (d Deps) secret() []byte { return d.secrets.get() }

// mediaTTL is how long a media token lasts. It is the credential of the URLs
// the browser loads by itself (<video>, <img>, subtitles, fonts), which cannot
// carry the X-Profile header. It is rounded so that a profile gets the same
// token for mediaStep: the URLs, and the browser cache with them, stay stable.
const (
	mediaTTL  = 36 * time.Hour
	mediaStep = 12 * time.Hour
)

func mediaMAC(secret []byte, profileID, pinHash, unix string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("media|" + profileID + "|" + pinHash + "|" + unix))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// mediaToken is "<id>.<unix>.<hmac>", good for GET/HEAD of media only.
func mediaToken(secret []byte, profileID, pinHash string, now time.Time) string {
	exp := now.Truncate(mediaStep).Add(mediaTTL)
	unix := strconv.FormatInt(exp.Unix(), 10)
	return profileID + "." + unix + "." + mediaMAC(secret, profileID, pinHash, unix)
}

func validMediaToken(secret []byte, token, id, pinHash string, now time.Time) bool {
	parts := strings.Split(token, ".")
	if len(secret) == 0 || len(parts) != 3 || parts[0] != id {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || now.Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(parts[2]), []byte(mediaMAC(secret, id, pinHash, parts[1])))
}

// requireProfiles answers 503 when profiles are not available.
func (d Deps) requireProfiles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Library.DB == nil || len(d.secret()) == 0 {
			writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "profiles not available", Code: "unavailable"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authenticate finds the profile of a request: by the X-Profile session token
// and, for GET/HEAD only, by the media token in the "m" query parameter.
func (d Deps) authenticate(r *http.Request, allowMedia bool) (library.Profile, bool, error) {
	token, media := r.Header.Get(ProfileHeader), false
	if token == "" && allowMedia && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		token, media = r.URL.Query().Get("m"), true
	}
	id, _, _ := strings.Cut(token, ".")
	if !isUUID(id) {
		return library.Profile{}, false, nil
	}
	p, hash, err := d.Library.ProfileAuth(r.Context(), id)
	if isNotFound(err) {
		return library.Profile{}, false, nil // a profile that was deleted: no session
	}
	if err != nil {
		return library.Profile{}, false, err
	}
	now := time.Now()
	if media {
		return p, validMediaToken(d.secret(), token, id, hash, now), nil
	}
	return p, validToken(d.secret(), token, id, hash, now), nil
}

func (d Deps) requireWith(allowMedia bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok, err := d.authenticate(r, allowMedia)
			if err != nil {
				d.writeError(w, r, err)
				return
			}
			if !ok {
				writeJSON(w, http.StatusUnauthorized, errorBody{Error: "choose a profile", Code: "no_profile"})
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), profileKey{}, p)))
		})
	}
}

// requireProfile checks the session token of the X-Profile header and puts
// its profile in the request context; 401 without a valid one.
func (d Deps) requireProfile(next http.Handler) http.Handler { return d.requireWith(false)(next) }

// requireAccess is requireProfile that also takes the media token of the
// "m" parameter on GET/HEAD, for what the browser loads from a URL.
func (d Deps) requireAccess(next http.Handler) http.Handler { return d.requireWith(true)(next) }

// PIN attempts: after pinMaxFails wrong PINs a (profile, address) pair is
// locked for pinLock, doubling at each new lock up to pinLockMax. The address
// is part of the key so that a guest cannot lock a profile for everybody.
const (
	pinMaxFails = 5
	pinLock     = 30 * time.Second
	pinLockMax  = 15 * time.Minute
)

// pinLimiter slows down guessing a 4-digit PIN. It lives in memory: a
// restart clears it, which is fine for a home server.
type pinLimiter struct {
	mu    sync.Mutex
	now   func() time.Time
	fails map[string]pinFails
}

type pinFails struct {
	count  int
	locks  int
	until  time.Time
	lastAt time.Time
}

func newPinLimiter() *pinLimiter {
	return &pinLimiter{now: time.Now, fails: map[string]pinFails{}}
}

// pinKey is the limiter key of a profile id tried from a client address.
func pinKey(id string, r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return id + "|" + ip
}

// wait is how long a key is still locked (0 = it may try).
func (l *pinLimiter) wait(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.fails[key]
	if left := f.until.Sub(l.now()); left > 0 {
		return left
	}
	return 0
}

// fail counts a wrong PIN and reports whether the key got locked.
func (l *pinLimiter) fail(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.fails) > 1000 { // forget old entries: the map must not grow without bound
		for k, f := range l.fails {
			if now.Sub(f.lastAt) > time.Hour {
				delete(l.fails, k)
			}
		}
	}
	f := l.fails[key]
	f.count++
	f.lastAt = now
	if f.count >= pinMaxFails {
		d := min(pinLock<<f.locks, pinLockMax)
		f.count, f.until = 0, now.Add(d)
		f.locks = min(f.locks+1, 10)
		l.fails[key] = f
		return true
	}
	l.fails[key] = f
	return false
}

// lockFor is the duration of the lock the last fail started.
func (l *pinLimiter) lockFor(key string) time.Duration { return l.wait(key) }

func (l *pinLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
