-- 002_scanner: manual corrections, extras and scanner bookkeeping.

-- What the scanner guessed (shown in the review queue) and probe results.
ALTER TABLE media_files ADD COLUMN guess_json      TEXT;
ALTER TABLE media_files ADD COLUMN review_reason   TEXT;
ALTER TABLE media_files ADD COLUMN probe_error     TEXT;
ALTER TABLE media_files ADD COLUMN audio_tracks    INTEGER;
ALTER TABLE media_files ADD COLUMN subtitle_tracks INTEGER;

-- Manual corrections. They win over the parser on every scan.
-- Key: relative path + size. partial_hash (first + last 64 KB) lets a
-- correction follow a file that was renamed or moved.
CREATE TABLE overrides (
    rel_path     TEXT    NOT NULL,
    size         INTEGER NOT NULL,
    partial_hash TEXT,
    kind         TEXT    CHECK (kind IN ('movie', 'series', 'anime', 'ignored')),
    title        TEXT,               -- movie title, or series/anime title
    year         INTEGER,
    season       INTEGER,
    episode      INTEGER,
    updated_at   INTEGER NOT NULL,
    PRIMARY KEY (rel_path, size)
) STRICT;

CREATE INDEX overrides_hash_idx ON overrides (size, partial_hash);

-- Trailers and extras, linked to the movie/series/season of the same folder.
CREATE TABLE extras (
    id            TEXT PRIMARY KEY,
    media_file_id TEXT NOT NULL UNIQUE REFERENCES media_files (id) ON DELETE CASCADE,
    owner_type    TEXT NOT NULL CHECK (owner_type IN ('movie', 'series', 'season')),
    owner_id      TEXT NOT NULL,
    extra_type    TEXT NOT NULL CHECK (extra_type IN
                  ('trailer', 'featurette', 'behind_the_scenes', 'deleted', 'interview', 'other')),
    title         TEXT NOT NULL
) STRICT;

CREATE INDEX extras_owner_idx ON extras (owner_type, owner_id);

-- Small key/value store for the scanner (e.g. fingerprint of sidecar files).
CREATE TABLE scan_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;
