// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGuard(t *testing.T) {
	d := Deps{AllowedHosts: []string{"telao.casa"}}
	h := d.guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))

	tests := []struct {
		name, method, host, origin string
		want                       int
	}{
		{"ip", "GET", "192.168.0.10:8080", "", 204},
		{"ipv6", "GET", "[::1]:8080", "", 204},
		{"localhost", "GET", "localhost:8080", "", 204},
		{"single label", "GET", "nas:8080", "", 204},
		{"mdns", "GET", "casa.local:8080", "", 204},
		{"allowed name", "GET", "telao.casa", "", 204},
		{"rebinding", "GET", "evil.example:8080", "", 403},
		{"rebinding POST", "POST", "evil.example", "http://evil.example", 403},
		{"same origin POST", "POST", "192.168.0.10:8080", "http://192.168.0.10:8080", 204},
		{"cross origin POST", "POST", "192.168.0.10:8080", "http://evil.example", 403},
		{"null origin DELETE", "DELETE", "192.168.0.10:8080", "null", 403},
		{"no origin POST", "POST", "192.168.0.10:8080", "", 204},
		{"cross origin GET", "GET", "192.168.0.10:8080", "http://evil.example", 204},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, "/api/x", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
			for k, v := range map[string]string{"X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer"} {
				if rec.Header().Get(k) != v {
					t.Errorf("%s = %q, want %q", k, rec.Header().Get(k), v)
				}
			}
			// Only the TV apps (file:// pages) and the server itself may frame it.
			if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'self' file:" {
				t.Errorf("Content-Security-Policy = %q", got)
			}
			if rec.Header().Get("X-Frame-Options") != "" {
				t.Error("X-Frame-Options cannot allow file: and would block the TV apps")
			}
		})
	}
}

func TestJSONContentTypeRequired(t *testing.T) {
	for ct, want := range map[string]bool{"application/json": true, "application/json; charset=utf-8": true, "text/plain": false, "": false} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		var v struct{}
		if got := decodeJSON(req, &v) == nil; got != want {
			t.Errorf("Content-Type %q accepted = %v, want %v", ct, got, want)
		}
	}
}

func TestHealthCORSOnlyForApps(t *testing.T) {
	h := healthHandler(Deps{Version: "9.9.9"})
	for origin, want := range map[string]string{"": "*", "null": "*", "file://": "*", "http://evil.example": "", "https://evil.example": ""} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/health", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("Origin %q: Allow-Origin = %q, want %q", origin, got, want)
		}
		if strings.Contains(rec.Body.String(), "9.9.9") {
			t.Error("the public health endpoint shows the version")
		}
	}
}

func TestSessionTokenExpires(t *testing.T) {
	secret := []byte("0123456789abcdef")
	now := time.Unix(1_000_000, 0)
	tok := sessionToken(secret, "p1", "hash", now.Add(time.Hour))
	if !validToken(secret, tok, "p1", "hash", now) {
		t.Fatal("a fresh token is refused")
	}
	if validToken(secret, tok, "p1", "hash", now.Add(2*time.Hour)) {
		t.Error("an expired token is accepted")
	}
	if validToken(secret, tok, "p1", "other-hash", now) || validToken(secret, tok, "p2", "hash", now) || validToken([]byte("another-secret!!"), tok, "p1", "hash", now) {
		t.Error("a token is accepted with a different PIN, profile or secret")
	}
	// Moving the expiry forward breaks the signature.
	parts := strings.Split(tok, ".")
	parts[1] = "9999999999"
	if validToken(secret, strings.Join(parts, "."), "p1", "hash", now) {
		t.Error("a token with a changed expiry is accepted")
	}
}

func TestPinLimiterPerAddressAndEscalates(t *testing.T) {
	now := time.Unix(0, 0)
	l := newPinLimiter()
	l.now = func() time.Time { return now }
	guest, owner := "p|192.168.0.50", "p|192.168.0.10"

	for range pinMaxFails {
		l.fail(guest)
	}
	if l.wait(guest) != pinLock {
		t.Fatalf("first lock = %v, want %v", l.wait(guest), pinLock)
	}
	if l.wait(owner) != 0 {
		t.Error("a guest locked the profile for its owner")
	}
	now = now.Add(pinLock)
	for range pinMaxFails {
		l.fail(guest)
	}
	if l.wait(guest) != 2*pinLock {
		t.Errorf("second lock = %v, want %v", l.wait(guest), 2*pinLock)
	}
	for range 40 {
		now = now.Add(pinLockMax)
		for range pinMaxFails {
			l.fail(guest)
		}
	}
	if l.wait(guest) > pinLockMax {
		t.Errorf("lock = %v, above the %v limit", l.wait(guest), pinLockMax)
	}
}

func TestContentSecurityPolicy(t *testing.T) {
	index := []byte("<html><body><script>\n\t{ boot(); }\n</script><script src=\"/x.js\"></script></body></html>")
	csp := contentSecurityPolicy(index)
	for _, want := range []string{
		"default-src 'self'", "script-src 'self' 'unsafe-eval' 'sha256-", "object-src 'none'",
		"frame-ancestors 'self' file:", "worker-src 'self' blob:",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP lacks %q: %s", want, csp)
		}
	}
	if strings.Count(csp, "'sha256-") != 1 {
		t.Errorf("only the inline script gets a hash: %s", csp)
	}
	for _, bad := range []string{"script-src 'unsafe-inline'", "script-src * ", "http:", "wasm-unsafe-eval"} {
		if strings.Contains(csp, bad) {
			t.Errorf("CSP has %q: %s", bad, csp)
		}
	}
}
