-- 004_profiles: profile sessions.

-- Small key/value store of the server (e.g. the secret that signs profile
-- sessions). Kept in the database so a backup restores open sessions too.
CREATE TABLE app_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;
