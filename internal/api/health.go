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

type errorBody struct {
	Error string `json:"error"`
}

// healthHandler answers GET /api/health. It returns 503 when the database
// does not respond. It is the only endpoint open to other origins: TV apps
// call it from their packaged (file://) page to find the server.
func healthHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body := healthBody{Name: "mediaserver", Status: "ok", Version: d.Version, DB: "ok"}
		status := http.StatusOK

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if d.DB == nil || d.DB.PingContext(ctx) != nil {
			body.Status, body.DB = "degraded", "error"
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		writeJSON(w, status, body)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// The status line is already sent; a write error here means the client left.
	_ = json.MarshalWrite(w, v)
}
