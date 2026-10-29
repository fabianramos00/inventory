-- +goose Up
-- Auth-only schema for offline SQLite mode.

CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    full_name TEXT,
    email TEXT NOT NULL UNIQUE,
    hashed_password TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    is_superuser INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE refresh_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    revoked INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX ix_refresh_tokens_user_id ON refresh_tokens (user_id);

-- +goose Down
DROP TABLE refresh_tokens;
DROP TABLE users;
