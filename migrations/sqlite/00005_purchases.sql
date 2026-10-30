-- +goose Up
-- Supplier catalogs and purchase orders. Requires 00003_identity.sql and
-- 00004_products.sql.

CREATE TABLE supplier_catalogs (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id       INTEGER,
    provider_id      INTEGER,
    supplier_sku     TEXT,
    last_quoted_cost INTEGER NOT NULL,
    lead_time        INTEGER,
    CONSTRAINT ck_supplier_catalogs_supplier_sku_len
        CHECK (supplier_sku IS NULL OR length(supplier_sku) <= 100),
    CONSTRAINT ck_supplier_catalogs_last_quoted_cost_range
        CHECK (last_quoted_cost BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT supplier_catalogs_product_id_fkey
        FOREIGN KEY (product_id) REFERENCES products(id),
    CONSTRAINT supplier_catalogs_provider_id_fkey
        FOREIGN KEY (provider_id) REFERENCES providers(id)
) STRICT;

CREATE INDEX ix_supplier_catalogs_product_id
    ON supplier_catalogs (product_id);
CREATE INDEX ix_supplier_catalogs_provider_id
    ON supplier_catalogs (provider_id);

CREATE TABLE purchase_orders (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id    INTEGER,
    status         TEXT NOT NULL DEFAULT 'PENDING',
    order_date     TEXT NOT NULL,
    total_amount   INTEGER NOT NULL,
    created_by_id  INTEGER,
    payment_status TEXT NOT NULL DEFAULT 'PENDING',
    CONSTRAINT ck_purchase_orders_status
        CHECK (status IN ('PENDING', 'SENT', 'RECEIVED', 'CANCELLED')),
    CONSTRAINT ck_purchase_orders_payment_status
        CHECK (payment_status IN ('PENDING', 'PAID')),
    CONSTRAINT ck_purchase_orders_order_date_format
        CHECK (length(order_date) = 30),
    CONSTRAINT ck_purchase_orders_total_amount_nonnegative
        CHECK (total_amount >= 0),
    CONSTRAINT ck_purchase_orders_total_amount_range
        CHECK (total_amount <= 999999999999),
    CONSTRAINT purchase_orders_provider_id_fkey
        FOREIGN KEY (provider_id) REFERENCES providers(id),
    CONSTRAINT purchase_orders_created_by_id_fkey
        FOREIGN KEY (created_by_id) REFERENCES users(id)
) STRICT;

CREATE INDEX ix_purchase_orders_provider_id
    ON purchase_orders (provider_id);
CREATE INDEX ix_purchase_orders_status ON purchase_orders (status);
CREATE INDEX ix_purchase_orders_order_date ON purchase_orders (order_date);
CREATE INDEX ix_purchase_orders_payment_status
    ON purchase_orders (payment_status);

CREATE TABLE order_items (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id     INTEGER NOT NULL,
    product_id   INTEGER NOT NULL,
    quantity     INTEGER NOT NULL,
    unit_cost    INTEGER NOT NULL,
    subtotal     INTEGER NOT NULL,
    supplier_sku TEXT,
    CONSTRAINT ck_order_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT ck_order_items_unit_cost_range
        CHECK (unit_cost BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT ck_order_items_subtotal_range
        CHECK (subtotal BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT ck_order_items_supplier_sku_len
        CHECK (supplier_sku IS NULL OR length(supplier_sku) <= 100),
    CONSTRAINT order_items_order_id_fkey
        FOREIGN KEY (order_id) REFERENCES purchase_orders(id),
    CONSTRAINT order_items_product_id_fkey
        FOREIGN KEY (product_id) REFERENCES products(id)
) STRICT;

CREATE INDEX ix_order_items_order_id ON order_items (order_id);
CREATE INDEX ix_order_items_product_id ON order_items (product_id);
