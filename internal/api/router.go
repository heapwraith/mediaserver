// Package api holds the HTTP routes, handlers and middlewares.
package api

import (
	"database/sql"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"mediaserver/internal/library"
	"mediaserver/internal/scanner"
	"mediaserver/internal/stream"
)

// Deps are the dependencies of the HTTP layer.
type Deps struct {
	Logger  *slog.Logger
	DB      *sql.DB
	Version string
	// Web is the built frontend (index.html at its root). It may be nil.
	Web fs.FS
	// Scanner powers the library endpoints. They answer 503 when it is nil.
	Scanner *scanner.Scanner
	// StartScan starts a background scan bound to the server lifetime (not to
	// the request). It reports whether a new scan started now.
	StartScan func() bool
	// Library, Stream and Images power the read and playback endpoints.
	Library library.Store
	Stream  *stream.Server
	Images  *stream.Images
}

// NewRouter builds the HTTP handler of the server.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", healthHandler(d))
		r.Group(func(r chi.Router) {
			r.Use(d.requireScanner)
			r.Get("/library/scan", d.scanStatus)
			r.Post("/library/scan", d.startScan)
			r.Get("/review", d.reviewQueue)
			r.Put("/review/{fileId}", d.correctFile)
			r.Post("/series/merge", d.mergeSeries)
			r.Post("/series/split", d.splitSeries)

			r.Get("/movies", d.listMovies)
			r.Get("/series", d.listSeries("series"))
			r.Get("/animes", d.listSeries("anime"))
			r.Get("/series/{id}", byID(d, func(d Deps, r *http.Request, id string) (library.SeriesDetail, error) {
				return d.Library.SeriesByID(r.Context(), id)
			}))
			r.Get("/movies/{id}", byID(d, func(d Deps, r *http.Request, id string) (library.Movie, error) {
				return d.Library.MovieByID(r.Context(), id)
			}))
			r.Get("/movies/{id}/extras", byID(d, func(d Deps, r *http.Request, id string) ([]library.Extra, error) {
				return d.Library.MovieExtras(r.Context(), id)
			}))
			r.Get("/series/{id}/extras", byID(d, func(d Deps, r *http.Request, id string) ([]library.Extra, error) {
				return d.Library.SeriesExtras(r.Context(), id)
			}))
			r.Get("/media/{id}", byID(d, func(d Deps, r *http.Request, id string) (library.Media, error) {
				return d.Library.MediaByID(r.Context(), id)
			}))
			r.Get("/media/{id}/stream", d.streamMedia)
			r.Get("/images/{id}", d.image)
		})
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotFound, errorBody{Error: "not found"})
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{Error: "method not allowed"})
		})
	})

	r.Handle("/*", newSPAHandler(d.Web, d.Logger))
	return r
}

// requireScanner answers 503 when the library is not available.
func (d Deps) requireScanner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.Scanner == nil || d.StartScan == nil || d.Library.DB == nil || d.Stream == nil || d.Images == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "library not available"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger logs each request: API calls at info, static files at debug.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			level := slog.LevelDebug
			if strings.HasPrefix(r.URL.Path, "/api/") {
				level = slog.LevelInfo
			}
			logger.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("request_id", middleware.GetReqID(r.Context())),
			)
		})
	}
}
