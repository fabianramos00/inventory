-- +goose Up
-- Catalog tables. Requires 00001_auth.sql for the baseline goose row.

CREATE TABLE categories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    description TEXT,
    code        TEXT,
    CONSTRAINT categories_name_key UNIQUE (name),
    CONSTRAINT ck_categories_name_len CHECK (length(name) <= 150),
    CONSTRAINT ck_categories_description_len
        CHECK (description IS NULL OR length(description) <= 500),
    CONSTRAINT ck_categories_code_len
        CHECK (code IS NULL OR length(code) <= 10)
) STRICT;

CREATE UNIQUE INDEX ix_categories_code ON categories (code);

CREATE TABLE materials (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    CONSTRAINT materials_name_key UNIQUE (name),
    CONSTRAINT ck_materials_name_len CHECK (length(name) <= 150)
) STRICT;

CREATE TABLE brands (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT NOT NULL,
    logo_url TEXT,
    CONSTRAINT brands_name_key UNIQUE (name),
    CONSTRAINT ck_brands_name_len CHECK (length(name) <= 150),
    CONSTRAINT ck_brands_logo_url_len
        CHECK (logo_url IS NULL OR length(logo_url) <= 255)
) STRICT;

CREATE TABLE measurement_units (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL,
    abbreviation TEXT NOT NULL,
    CONSTRAINT measurement_units_name_key UNIQUE (name),
    CONSTRAINT measurement_units_abbreviation_key UNIQUE (abbreviation),
    CONSTRAINT ck_measurement_units_name_len CHECK (length(name) <= 150),
    CONSTRAINT ck_measurement_units_abbreviation_len
        CHECK (length(abbreviation) <= 5)
) STRICT;
