-- +goose Up
-- Sale payment ledger. The opening payment backfills the legacy cumulative
-- sales.amount_paid; sales is rebuilt to drop payment_method and its table
-- CHECK and index.

-- +goose NO TRANSACTION
PRAGMA foreign_keys=OFF;
BEGIN;

CREATE TABLE sale_payments (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    sale_id        INTEGER NOT NULL,
    amount         INTEGER NOT NULL,
    payment_method TEXT,
    created_at     TEXT NOT NULL,
    created_by_id  INTEGER,
    CONSTRAINT ck_sale_payments_amount_positive CHECK (amount > 0),
    CONSTRAINT ck_sale_payments_amount_range
        CHECK (amount <= 999999999999),
    CONSTRAINT ck_sale_payments_method
        CHECK (payment_method IS NULL
               OR payment_method IN ('CASH', 'CREDIT', 'DEBIT')),
    CONSTRAINT ck_sale_payments_created_at_format CHECK (length(created_at) = 30),
    CONSTRAINT sale_payments_sale_id_fkey
        FOREIGN KEY (sale_id) REFERENCES sales(id) ON DELETE CASCADE,
    CONSTRAINT sale_payments_created_by_id_fkey
        FOREIGN KEY (created_by_id) REFERENCES users(id)
) STRICT;

INSERT INTO sale_payments (sale_id, amount, payment_method, created_at, created_by_id)
SELECT id, amount_paid, payment_method, created_at, created_by_id
FROM sales
WHERE amount_paid > 0;

CREATE TABLE sales_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT,
    total_amount    INTEGER NOT NULL,
    amount_paid     INTEGER NOT NULL,
    payment_status  TEXT NOT NULL DEFAULT 'PENDING',
    created_by_id   INTEGER,
    delivery_status TEXT NOT NULL DEFAULT 'PENDING',
    client_id       INTEGER,
    CONSTRAINT ck_sales_created_at_format CHECK (length(created_at) = 30),
    CONSTRAINT ck_sales_updated_at_format
        CHECK (updated_at IS NULL OR length(updated_at) = 30),
    CONSTRAINT ck_sales_total_amount_nonnegative CHECK (total_amount >= 0),
    CONSTRAINT ck_sales_total_amount_range
        CHECK (total_amount <= 999999999999),
    CONSTRAINT ck_sales_amount_paid_range
        CHECK (amount_paid BETWEEN -999999999999 AND 999999999999),
    CONSTRAINT ck_sales_payment_status
        CHECK (payment_status IN ('PAID', 'PENDING', 'PARTIAL')),
    CONSTRAINT ck_sales_delivery_status
        CHECK (delivery_status IN ('PENDING', 'PARTIAL', 'DELIVERED')),
    CONSTRAINT fk_sales_client_id
        FOREIGN KEY (client_id) REFERENCES clients(id),
    CONSTRAINT sales_created_by_id_fkey
        FOREIGN KEY (created_by_id) REFERENCES users(id)
) STRICT;

INSERT INTO sales_new (id, created_at, updated_at, total_amount, amount_paid, payment_status,
                       created_by_id, delivery_status, client_id)
SELECT id, created_at, updated_at, total_amount, amount_paid, payment_status,
       created_by_id, delivery_status, client_id
FROM sales;

DROP TABLE sales;
ALTER TABLE sales_new RENAME TO sales;

CREATE INDEX ix_sales_created_at ON sales (created_at);
CREATE INDEX ix_sales_payment_status ON sales (payment_status);
CREATE INDEX ix_sales_delivery_status ON sales (delivery_status);
CREATE INDEX ix_sales_client_id ON sales (client_id);
CREATE INDEX ix_sales_created_by_id ON sales (created_by_id);
CREATE INDEX ix_sale_payments_sale_id_created_at_id
    ON sale_payments (sale_id, created_at, id);

COMMIT;
PRAGMA foreign_keys=ON;
PRAGMA foreign_key_check;
