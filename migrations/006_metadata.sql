-- 006_metadata: genres and collections from TMDB, optional and off by default
-- (docs/adr/012-metadata-tmdb.md). The key and the switch are in app_state.
--
-- movies is rebuilt on every materialize (its ids are stable), so nothing here
-- has a foreign key to movies or series: a cascade would erase it all. Rows of
-- titles that are gone are removed after each sync.

-- What is known of each movie or series (owner_id).
CREATE TABLE title_meta (
    owner_id   TEXT    PRIMARY KEY,
    owner_type TEXT    NOT NULL CHECK (owner_type IN ('movie', 'series')),
    status     TEXT    NOT NULL CHECK (status IN ('matched', 'review', 'skipped')),
    tmdb_id    INTEGER,
    lookup_key TEXT    NOT NULL,    -- normalized title + year: a new one is looked up again
    candidates TEXT,                -- JSON, while the user has to choose (Revisar)
    updated_at INTEGER NOT NULL
) STRICT;

CREATE INDEX title_meta_status_idx ON title_meta (status);

CREATE TABLE genres (
    id   INTEGER PRIMARY KEY,       -- TMDB id
    name TEXT    NOT NULL
) STRICT;

CREATE TABLE title_genres (
    owner_id TEXT    NOT NULL,
    genre_id INTEGER NOT NULL REFERENCES genres (id) ON DELETE CASCADE,
    PRIMARY KEY (owner_id, genre_id)
) STRICT;

CREATE INDEX title_genres_genre_idx ON title_genres (genre_id, owner_id);

CREATE TABLE collections (
    id   INTEGER PRIMARY KEY,       -- TMDB id
    name TEXT    NOT NULL
) STRICT;

CREATE TABLE collection_movies (
    collection_id INTEGER NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
    movie_id      TEXT    NOT NULL,
    PRIMARY KEY (collection_id, movie_id)
) STRICT;

CREATE INDEX collection_movies_movie_idx ON collection_movies (movie_id);
