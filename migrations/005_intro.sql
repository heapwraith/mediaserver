-- 005_intro: openings and end credits found by audio fingerprints (ADR 010).

-- Chromaprint points (little-endian uint32) of the start and the end of each
-- episode. file_key (size:mtime) tells when the file changed.
CREATE TABLE fingerprints (
    media_file_id TEXT    PRIMARY KEY REFERENCES media_files (id) ON DELETE CASCADE,
    file_key      TEXT    NOT NULL,
    head          BLOB    NOT NULL,
    tail          BLOB    NOT NULL,
    tail_start_ms INTEGER NOT NULL
) STRICT;

-- Where the opening and the credits are (what the player skips).
CREATE TABLE media_markers (
    media_file_id TEXT    NOT NULL REFERENCES media_files (id) ON DELETE CASCADE,
    kind          TEXT    NOT NULL CHECK (kind IN ('intro', 'credits')),
    start_ms      INTEGER NOT NULL,
    end_ms        INTEGER NOT NULL,
    PRIMARY KEY (media_file_id, kind)
) STRICT;
