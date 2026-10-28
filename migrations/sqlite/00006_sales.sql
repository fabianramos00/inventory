-- +goose Up
-- Sales and sale items. Requires 00003_identity.sql and 00004_products.sql.

CREATE TABLE sales (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT,
    total_amount    INTEGER NOT NULL,
    amount_paid     INTEGER NOT NULL,
    payment_status  TEXT NOT NULL DEFAULT 'PENDING',
    payment_method  TEXT,
    created_by_id   INTEGER,
    delivery_status TEXT NOT NULL DEFAULT 'PENDING',
    client_id       INTEGER,
    CONSTRAINT ck_sales_created_at_format CHECK (
        created_at GLOB
        '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]Z'
    ),
    CONSTRAINT ck_sales_updated_at_format CHECK (
        updated_at IS NULL OR updated_at GLOB
        '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]Z'
    ),
    CONSTRAINT ck_sales_total_amount_nonnegative CHECK (total_amount >= 0),
    CONSTRAINT ck_sales_total_amount_range
        CHECK (total_amount <= 999999999999),
    CONSTRAINT ck_sales_amount_paid_range
        CHECK (amount_paid BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT ck_sales_payment_status
        CHECK (payment_status IN ('PAID', 'PENDING', 'PARTIAL')),
    CONSTRAINT ck_sales_payment_method
        CHECK (payment_method IS NULL
               OR payment_method IN ('CASH', 'CREDIT', 'DEBIT')),
    CONSTRAINT ck_sales_delivery_status
        CHECK (delivery_status IN ('PENDING', 'PARTIAL', 'DELIVERED')),
    CONSTRAINT fk_sales_client_id
        FOREIGN KEY (client_id) REFERENCES clients(id),
    CONSTRAINT sales_created_by_id_fkey
        FOREIGN KEY (created_by_id) REFERENCES users(id)
) STRICT;

CREATE INDEX ix_sales_created_at ON sales (created_at);
CREATE INDEX ix_sales_payment_status ON sales (payment_status);
CREATE INDEX ix_sales_delivery_status ON sales (delivery_status);
CREATE INDEX ix_sales_client_id ON sales (client_id);
CREATE INDEX ix_sales_created_by_id ON sales (created_by_id);
CREATE INDEX ix_sales_payment_method ON sales (payment_method);

CREATE TABLE sale_items (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    sale_id            INTEGER NOT NULL,
    product_id         INTEGER NOT NULL,
    quantity           INTEGER NOT NULL,
    unit_price         INTEGER NOT NULL,
    subtotal           INTEGER NOT NULL,
    delivered_quantity INTEGER NOT NULL,
    CONSTRAINT ck_sale_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT ck_sale_items_unit_price_range
        CHECK (unit_price BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT ck_sale_items_subtotal_range
        CHECK (subtotal BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT ck_sale_items_delivered_valid
        CHECK (delivered_quantity >= 0 AND delivered_quantity <= quantity),
    CONSTRAINT sale_items_sale_id_fkey
        FOREIGN KEY (sale_id) REFERENCES sales(id),
    CONSTRAINT sale_items_product_id_fkey
        FOREIGN KEY (product_id) REFERENCES products(id)
) STRICT;

CREATE INDEX ix_sale_items_sale_id ON sale_items (sale_id);
CREATE INDEX ix_sale_items_product_id ON sale_items (product_id);
