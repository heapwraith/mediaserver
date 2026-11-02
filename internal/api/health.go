// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"time"
)

type healthBody struct {
	// Name lets TV apps tell this server apart while scanning the network.
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version"`
	DB      string `json:"db"`
}

// errorBody is every error answer. Code is stable (the frontend turns it into
// a message, web/src/lib/api/errors.ts); Error is for people reading logs.
type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// healthHandler answers GET /api/health. It returns 503 when the database
// does not respond. It is the only endpoint open to other origins: TV apps
// call it from their packaged (file://) page to find the server.
func healthHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := healthBody{Name: "mediaserver", Status: "ok", DB: "ok"} // no version: anyone on the network can read this
		status := http.StatusOK

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if d.DB == nil || d.DB.PingContext(ctx) != nil {
			body.Status, body.DB = "degraded", "error"
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Cache-Control", "no-store")
		// The TV apps (file:// pages) probe the network for servers; a web
		// page may not, so a site cannot map the house through this endpoint.
		if o := r.Header.Get("Origin"); !webOrigin(o) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		writeJSON(w, status, body)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// The status line is already sent; a write error here means the client left.
	_ = json.MarshalWrite(w, v)
}
