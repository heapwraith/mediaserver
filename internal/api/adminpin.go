// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mediaserver/internal/library"
)

// AdminHeader carries the admin token (see POST /api/admin/unlock).
const AdminHeader = "X-Admin"

// adminTTL is how long an unlocked admin session lasts.
const adminTTL = 30 * time.Minute

const adminPinKey = "admin_pin"

// adminPinHash is the stored hash of the admin PIN ("" = no admin PIN, the
// administration stays open as before).
func (d Deps) adminPinHash(ctx context.Context) (string, error) {
	if d.DB == nil {
		return "", nil
	}
	var v string
	err := d.DB.QueryRowContext(ctx, `SELECT value FROM app_state WHERE key = ?`, adminPinKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read admin pin: %w", err)
	}
	return v, nil
}

func adminMAC(secret []byte, pinHash, unix string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("admin|" + pinHash + "|" + unix))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// adminToken is "<unix>.<hmac>": signed with the PIN hash, so changing or
// removing the admin PIN ends every admin session.
func adminToken(secret []byte, pinHash string, exp time.Time) string {
	unix := strconv.FormatInt(exp.Unix(), 10)
	return unix + "." + adminMAC(secret, pinHash, unix)
}

func validAdminToken(secret []byte, token, pinHash string, now time.Time) bool {
	if len(secret) == 0 {
		return false
	}
	unix, mac, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(unix, 10, 64)
	if err != nil || now.Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(mac), []byte(adminMAC(secret, pinHash, unix)))
}

// requireAdmin lets the request through when no admin PIN is set (the
// default, as before) or when it carries a valid admin token.
func (d Deps) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hash, err := d.adminPinHash(r.Context())
		if err != nil {
			d.writeError(w, r, err)
			return
		}
		if hash != "" && !validAdminToken(d.secret(), r.Header.Get(AdminHeader), hash, time.Now()) {
			writeJSON(w, http.StatusForbidden, errorBody{Error: "administrator PIN required", Code: "admin_required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type adminAccess struct {
	Enabled  bool `json:"enabled"`
	Unlocked bool `json:"unlocked"`
}

// adminAccess tells the interface whether it must ask for the admin PIN.
func (d Deps) adminAccess(w http.ResponseWriter, r *http.Request) {
	hash, err := d.adminPinHash(r.Context())
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, adminAccess{
		Enabled:  hash != "",
		Unlocked: hash == "" || validAdminToken(d.secret(), r.Header.Get(AdminHeader), hash, time.Now()),
	})
}

type adminTokenBody struct {
	Token string `json:"token"`
}

// adminUnlock checks the admin PIN and returns an admin token.
func (d Deps) adminUnlock(w http.ResponseWriter, r *http.Request) {
	var b pinBody
	if err := decodeJSON(r, &b); err != nil {
		d.writeError(w, r, err)
		return
	}
	hash, err := d.adminPinHash(r.Context())
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	if hash == "" {
		writeJSON(w, http.StatusOK, adminTokenBody{})
		return
	}
	key := pinKey("admin", r)
	if left := d.pins.wait(key); left > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(left.Seconds()))))
		writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "too many wrong PINs, wait a moment", Code: "pin_locked"})
		return
	}
	if !library.VerifyPIN(hash, b.PIN) {
		if d.pins.fail(key) {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(d.pins.lockFor(key).Seconds()))))
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "too many wrong PINs, wait a moment", Code: "pin_locked"})
			return
		}
		writeJSON(w, http.StatusForbidden, errorBody{Error: "wrong PIN", Code: "wrong_pin"})
		return
	}
	d.pins.reset(key)
	writeJSON(w, http.StatusOK, adminTokenBody{Token: adminToken(d.secret(), hash, time.Now().Add(adminTTL))})
}

// putAdminPin sets, changes or (empty PIN) removes the admin PIN. When one is
// set, requireAdmin already checked the old one.
func (d Deps) putAdminPin(w http.ResponseWriter, r *http.Request) {
	var b pinBody
	if err := decodeJSON(r, &b); err != nil {
		d.writeError(w, r, err)
		return
	}
	if b.PIN == "" {
		if _, err := d.DB.ExecContext(r.Context(), `DELETE FROM app_state WHERE key = ?`, adminPinKey); err != nil {
			d.writeError(w, r, fmt.Errorf("remove admin pin: %w", err))
			return
		}
		writeJSON(w, http.StatusOK, adminTokenBody{})
		return
	}
	if len(b.PIN) != 4 || strings.Trim(b.PIN, "0123456789") != "" {
		d.writeError(w, r, fmt.Errorf("%w: pin must have 4 digits", library.ErrInvalid))
		return
	}
	hash, err := library.HashPIN(b.PIN)
	if err != nil {
		d.writeError(w, r, err)
		return
	}
	if _, err := d.DB.ExecContext(r.Context(),
		`INSERT INTO app_state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		adminPinKey, hash); err != nil {
		d.writeError(w, r, fmt.Errorf("save admin pin: %w", err))
		return
	}
	// The old admin sessions ended with the old hash; this one starts fresh.
	writeJSON(w, http.StatusOK, adminTokenBody{Token: adminToken(d.secret(), hash, time.Now().Add(adminTTL))})
}
