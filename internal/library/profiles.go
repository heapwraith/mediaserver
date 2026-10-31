package library

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

// ErrConflict is returned when a change clashes with existing data (e.g. a
// profile name already in use).
var ErrConflict = errors.New("conflict")

// Profile limits.
const (
	MaxProfiles    = 12
	maxProfileName = 20
	pinIterations  = 100_000
	pinKeyLen      = 32
)

var (
	colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	pinRe   = regexp.MustCompile(`^[0-9]{4}$`)
)

// Profile is a local profile: each person of the house has their own
// progress. The PIN only keeps the others out of it (no strong auth).
type Profile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"` // avatar color, '#rrggbb'
	HasPIN    bool   `json:"has_pin"`
	CreatedAt int64  `json:"created_at"`
}

// ProfileInput creates a profile or changes one. On update, a nil PIN keeps
// the current one, "" removes it and 4 digits set a new one.
type ProfileInput struct {
	Name  string  `json:"name"`
	Color string  `json:"color"`
	PIN   *string `json:"pin,omitempty"`
}

func (in *ProfileInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > maxProfileName {
		return fmt.Errorf("%w: name must have 1-%d characters", ErrInvalid, maxProfileName)
	}
	if !colorRe.MatchString(in.Color) {
		return fmt.Errorf("%w: color must look like #rrggbb", ErrInvalid)
	}
	in.Color = strings.ToLower(in.Color)
	if in.PIN != nil && *in.PIN != "" && !pinRe.MatchString(*in.PIN) {
		return fmt.Errorf("%w: pin must have 4 digits", ErrInvalid)
	}
	return nil
}

// HashPIN derives the stored form of a PIN: pbkdf2-sha256$iter$salt$key.
func HashPIN(pin string) (string, error) {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt) // never fails (crypto/rand)
	key, err := pbkdf2.Key(sha256.New, pin, salt, pinIterations, pinKeyLen)
	if err != nil {
		return "", fmt.Errorf("hash pin: %w", err)
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pinIterations, hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

// VerifyPIN reports whether pin matches a HashPIN result.
func VerifyPIN(stored, pin string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	salt, err1 := hex.DecodeString(parts[2])
	want, err2 := hex.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pin, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

const profileCols = `id, name, color, pin_hash IS NOT NULL, created_at`

func scanProfile(row interface{ Scan(...any) error }, p *Profile) error {
	return row.Scan(&p.ID, &p.Name, &p.Color, &p.HasPIN, &p.CreatedAt)
}

// Profiles lists the profiles in creation order.
func (s Store) Profiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+profileCols+` FROM profiles ORDER BY created_at, rowid`)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	out := []Profile{}
	err = scanAll(rows, func(r *sql.Rows) error {
		var p Profile
		err := scanProfile(r, &p)
		out = append(out, p)
		return err
	})
	return out, err
}

// ProfileAuth returns a profile and its stored PIN hash ("" without PIN).
func (s Store) ProfileAuth(ctx context.Context, id string) (Profile, string, error) {
	var p Profile
	var hash sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT id, name, color, created_at, pin_hash FROM profiles WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Color, &p.CreatedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return p, "", fmt.Errorf("profile %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return p, "", fmt.Errorf("profile %s: %w", id, err)
	}
	p.HasPIN = hash.Valid
	return p, hash.String, nil
}

// nameTaken reports whether another profile (not except) uses name, ignoring case.
func nameTaken(ctx context.Context, tx *sql.Tx, name, except string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM profiles WHERE name = ? COLLATE NOCASE AND id != ?`, name, except).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("check profile name: %w", err)
	default:
		return fmt.Errorf("%w: a profile named %q already exists", ErrConflict, name)
	}
}

func pinValue(pin *string) (any, error) {
	if pin == nil || *pin == "" {
		return nil, nil
	}
	return HashPIN(*pin)
}

// CreateProfile adds a profile.
func (s Store) CreateProfile(ctx context.Context, in ProfileInput) (Profile, error) {
	if err := in.validate(); err != nil {
		return Profile{}, err
	}
	hash, err := pinValue(in.PIN)
	if err != nil {
		return Profile{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, fmt.Errorf("create profile: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profiles`).Scan(&n); err != nil {
		return Profile{}, fmt.Errorf("count profiles: %w", err)
	}
	if n >= MaxProfiles {
		return Profile{}, fmt.Errorf("%w: at most %d profiles", ErrConflict, MaxProfiles)
	}
	if err := nameTaken(ctx, tx, in.Name, ""); err != nil {
		return Profile{}, err
	}
	p := Profile{ID: uuid.NewV7().String(), Name: in.Name, Color: in.Color, HasPIN: hash != nil, CreatedAt: time.Now().UnixMilli()}
	if _, err := tx.ExecContext(ctx, `INSERT INTO profiles (id, name, color, pin_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Color, hash, p.CreatedAt); err != nil {
		return Profile{}, fmt.Errorf("insert profile: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, fmt.Errorf("create profile: %w", err)
	}
	return p, nil
}

// UpdateProfile changes the name, color and (when in.PIN is set) the PIN.
func (s Store) UpdateProfile(ctx context.Context, id string, in ProfileInput) (Profile, error) {
	if err := in.validate(); err != nil {
		return Profile{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Profile{}, fmt.Errorf("update profile: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := nameTaken(ctx, tx, in.Name, id); err != nil {
		return Profile{}, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE profiles SET name = ?, color = ? WHERE id = ?`, in.Name, in.Color, id)
	if err != nil {
		return Profile{}, fmt.Errorf("update profile %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Profile{}, fmt.Errorf("profile %s: %w", id, ErrNotFound)
	}
	if in.PIN != nil {
		hash, err := pinValue(in.PIN)
		if err != nil {
			return Profile{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE profiles SET pin_hash = ? WHERE id = ?`, hash, id); err != nil {
			return Profile{}, fmt.Errorf("update pin %s: %w", id, err)
		}
	}
	var p Profile
	if err := scanProfile(tx.QueryRowContext(ctx, `SELECT `+profileCols+` FROM profiles WHERE id = ?`, id), &p); err != nil {
		return Profile{}, fmt.Errorf("read profile %s: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, fmt.Errorf("update profile: %w", err)
	}
	return p, nil
}

// DeleteProfile removes a profile and its progress.
func (s Store) DeleteProfile(ctx context.Context, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete profile %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("profile %s: %w", id, ErrNotFound)
	}
	return nil
}

// Secret returns the server secret that signs profile sessions, creating it
// on first use.
func (s Store) Secret(ctx context.Context) ([]byte, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails (crypto/rand)
	if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO app_state (key, value) VALUES ('session_secret', ?)`,
		hex.EncodeToString(b)); err != nil {
		return nil, fmt.Errorf("create secret: %w", err)
	}
	var v string
	if err := s.DB.QueryRowContext(ctx, `SELECT value FROM app_state WHERE key = 'session_secret'`).Scan(&v); err != nil {
		return nil, fmt.Errorf("read secret: %w", err)
	}
	secret, err := hex.DecodeString(v)
	if err != nil || len(secret) < 16 {
		return nil, errors.New("read secret: invalid value")
	}
	return secret, nil
}
