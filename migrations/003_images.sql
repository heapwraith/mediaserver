-- 003_images: generated images (frames, WebP variants) cached in DATA_DIR.

-- One row per movie, series, season, episode or media file whose images were
-- generated. source_key changes when the source (local art or video) changes.
CREATE TABLE image_cache (
    owner_id   TEXT    PRIMARY KEY,
    source_key TEXT    NOT NULL,
    color      TEXT,                -- dominant color of the card, '#rrggbb'
    updated_at INTEGER NOT NULL
) STRICT;
