// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// guard blocks what a web page opened in a browser of the house could do to
// the server, and sets the security headers.
//
//   - Host: DNS rebinding makes a hostile domain point to the server's LAN
//     address; the browser then sends it as the Host. Only IPs, localhost,
//     names without dots, *.local, *.lan, *.home.arpa and Deps.AllowedHosts pass.
//   - Origin: a request that changes something and comes from another origin
//     (CSRF) is refused. The apps load the interface from the server itself, so
//     their requests are same-origin.
func (d Deps) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("Referrer-Policy", "no-referrer")

		if !d.hostAllowed(r.Host) {
			writeJSON(w, http.StatusForbidden, errorBody{Error: "host not allowed", Code: "bad_host"})
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if o := r.Header.Get("Origin"); o != "" && !sameOrigin(o, r.Host) {
				writeJSON(w, http.StatusForbidden, errorBody{Error: "cross-origin request refused", Code: "bad_origin"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// hostAllowed reports whether the Host header names this server in a way a
// hostile domain cannot.
func (d Deps) hostAllowed(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	switch {
	case host == "":
		return true // HTTP/1.0 without Host
	case net.ParseIP(host) != nil, host == "localhost", !strings.Contains(host, "."):
		return true
	}
	for _, suffix := range []string{".local", ".lan", ".home.arpa", ".localhost"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	for _, a := range d.AllowedHosts {
		if strings.EqualFold(strings.TrimSpace(a), host) {
			return true
		}
	}
	return false
}

// sameOrigin compares the host:port of an Origin header with the Host header.
func sameOrigin(origin, host string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, host)
}

// isJSON reports whether the request declares a JSON body. A page cannot send
// application/json to another origin without a CORS preflight.
func isJSON(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

// webOrigin tells a web page (http/https origin) from the TV apps, whose
// pages come from file:// and send "null" or no Origin.
func webOrigin(origin string) bool {
	return strings.HasPrefix(origin, "http://") || strings.HasPrefix(origin, "https://")
}
