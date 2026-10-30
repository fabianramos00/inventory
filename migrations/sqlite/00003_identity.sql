-- +goose Up
-- Clients and providers. Users and refresh_tokens live in 00001_auth.sql.

CREATE TABLE clients (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT,
    name          TEXT NOT NULL,
    identity_card TEXT,
    email         TEXT,
    phone         TEXT,
    CONSTRAINT clients_identity_card_key UNIQUE (identity_card),
    CONSTRAINT ck_clients_created_at_format CHECK (length(created_at) = 30),
    CONSTRAINT ck_clients_updated_at_format
        CHECK (updated_at IS NULL OR length(updated_at) = 30)
) STRICT;

CREATE TABLE providers (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    contact_info  TEXT,
    email         TEXT,
    phone         TEXT,
    created_by_id INTEGER,
    CONSTRAINT ck_providers_name_len CHECK (length(name) <= 255),
    CONSTRAINT ck_providers_contact_info_len
        CHECK (contact_info IS NULL OR length(contact_info) <= 500),
    CONSTRAINT ck_providers_email_len
        CHECK (email IS NULL OR length(email) <= 255),
    CONSTRAINT ck_providers_phone_len
        CHECK (phone IS NULL OR length(phone) <= 50),
    CONSTRAINT providers_created_by_id_fkey
        FOREIGN KEY (created_by_id) REFERENCES users(id)
) STRICT;
