// Package metadata looks movies and series up on TMDB, only when the user
// turns it on with their own key: genres and, for movies, the collection
// (docs/adr/012-metadata-tmdb.md).
package metadata

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBase = "https://api.themoviedb.org/3"
	language    = "pt-BR"
	timeout     = 15 * time.Second
	maxResponse = 4 << 20
)

// Errors of the TMDB client. None of them carries the key.
var (
	ErrUnauthorized = errors.New("TMDB refused the key")
	ErrNotFound     = errors.New("not found on TMDB")
	ErrUnavailable  = errors.New("TMDB unavailable")
	ErrInvalid      = errors.New("invalid metadata request")
)

// Kinds of title on TMDB.
const (
	kindMovie = "movie"
	kindTV    = "tv"
)

// Candidate is a TMDB title the user can choose in Revisar.
type Candidate struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Original string `json:"original_title,omitzero"`
	Year     int    `json:"year,omitzero"`
}

// genre and collection are as TMDB names them.
type genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type collection struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// details is what the server keeps of a title.
type details struct {
	Genres     []genre     `json:"genres"`
	Collection *collection `json:"belongs_to_collection"`
}

// result is a movie (title…) or a series (name…) in a TMDB list.
type result struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	Name          string `json:"name"`
	OriginalName  string `json:"original_name"`
	FirstAirDate  string `json:"first_air_date"`
}

func candidates(rs []result) []Candidate {
	out := make([]Candidate, 0, len(rs))
	for _, r := range rs {
		c := Candidate{
			ID:       r.ID,
			Title:    cmp.Or(r.Title, r.Name),
			Original: cmp.Or(r.OriginalTitle, r.OriginalName),
		}
		if date := cmp.Or(r.ReleaseDate, r.FirstAirDate); len(date) >= 4 {
			c.Year, _ = strconv.Atoi(date[:4])
		}
		out = append(out, c)
	}
	return out
}

type client struct {
	base string
	key  string
	http *http.Client
}

// get answers path into out. A read token (v4) goes in a header; an API key
// (v3) can only go in the URL, so errors must never show the URL.
func (c client) get(ctx context.Context, path string, query url.Values, out any) error {
	if query == nil {
		query = url.Values{}
	}
	query.Set("language", language)
	token := strings.Contains(c.key, ".")
	if !token {
		query.Set("api_key", c.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+query.Encode(), http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: bad request for %s", ErrUnavailable, path)
	}
	req.Header.Set("Accept", "application/json")
	if token {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	res, err := c.http.Do(req) //nolint:gosec // the host is fixed (defaultBase); only tests change it
	if err != nil {
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err // its text has the URL, and the URL has the key
		}
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case res.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("%w: status %d", ErrUnavailable, res.StatusCode)
	}
	if err := json.UnmarshalRead(io.LimitReader(res.Body, maxResponse), out); err != nil {
		return fmt.Errorf("%w: bad answer for %s", ErrUnavailable, path)
	}
	return nil
}

// find looks up an IMDb id (tt1234567).
func (c client) find(ctx context.Context, imdb, kind string) ([]Candidate, error) {
	var out struct {
		Movies []result `json:"movie_results"`
		TV     []result `json:"tv_results"`
	}
	err := c.get(ctx, "/find/"+url.PathEscape(imdb), url.Values{"external_source": {"imdb_id"}}, &out)
	if err != nil {
		return nil, err
	}
	if kind == kindMovie {
		return candidates(out.Movies), nil
	}
	return candidates(out.TV), nil
}

// search looks a title up, the most popular first.
func (c client) search(ctx context.Context, kind, title string) ([]Candidate, error) {
	var out struct {
		Results []result `json:"results"`
	}
	if err := c.get(ctx, "/search/"+kind, url.Values{"query": {title}}, &out); err != nil {
		return nil, err
	}
	return candidates(out.Results), nil
}

// details reads the genres and the collection of a title.
func (c client) details(ctx context.Context, kind string, id int) (details, error) {
	var out details
	err := c.get(ctx, "/"+kind+"/"+strconv.Itoa(id), nil, &out)
	return out, err
}
