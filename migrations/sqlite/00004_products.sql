-- +goose Up
-- Products and price history. Requires 00002_catalog.sql.

CREATE TABLE products (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    sku                 TEXT NOT NULL,
    name                TEXT NOT NULL,
    size_value          TEXT,
    measurement_unit_id INTEGER,
    price               INTEGER NOT NULL,
    cost                INTEGER NOT NULL,
    stock_quantity      INTEGER NOT NULL,
    low_stock_threshold INTEGER NOT NULL,
    brand_id            INTEGER,
    category_id         INTEGER,
    material_id         INTEGER,
    CONSTRAINT ck_products_sku_len CHECK (length(sku) <= 100),
    CONSTRAINT ck_products_name_len CHECK (length(name) <= 150),
    CONSTRAINT ck_products_size_value_len
        CHECK (size_value IS NULL OR length(size_value) <= 20),
    CONSTRAINT ck_products_price_positive CHECK (price > 0),
    CONSTRAINT ck_products_price_range CHECK (price <= 999999999999),
    CONSTRAINT ck_products_cost_positive CHECK (cost > 0),
    CONSTRAINT ck_products_cost_range CHECK (cost <= 999999999999),
    CONSTRAINT ck_products_stock_nonnegative CHECK (stock_quantity >= 0),
    CONSTRAINT ck_products_low_stock_threshold_nonnegative
        CHECK (low_stock_threshold >= 0),
    CONSTRAINT products_measurement_unit_id_fkey
        FOREIGN KEY (measurement_unit_id) REFERENCES measurement_units(id),
    CONSTRAINT products_brand_id_fkey
        FOREIGN KEY (brand_id) REFERENCES brands(id),
    CONSTRAINT products_category_id_fkey
        FOREIGN KEY (category_id) REFERENCES categories(id),
    CONSTRAINT products_material_id_fkey
        FOREIGN KEY (material_id) REFERENCES materials(id)
) STRICT;

CREATE UNIQUE INDEX ix_products_sku ON products (sku);
CREATE INDEX ix_products_brand_id ON products (brand_id);
CREATE INDEX ix_products_category_id ON products (category_id);
CREATE INDEX ix_products_material_id ON products (material_id);
CREATE INDEX ix_products_stock_quantity ON products (stock_quantity);

CREATE TABLE price_history (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id INTEGER,
    old_price  INTEGER NOT NULL,
    new_price  INTEGER NOT NULL,
    changed_at TEXT NOT NULL,
    CONSTRAINT ck_price_history_old_price_range
        CHECK (old_price BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT ck_price_history_new_price_range
        CHECK (new_price BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT ck_price_history_changed_at_format CHECK (
        changed_at GLOB
        '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]Z'
    ),
    CONSTRAINT price_history_product_id_fkey
        FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) STRICT;

CREATE INDEX ix_price_history_product_id ON price_history (product_id);
