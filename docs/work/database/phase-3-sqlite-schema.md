# Phase 3 SQLite Business Schema — Decision-Ready Proposal

**Status:** Proposal only. No code, SQL migration, configuration, or test changed.
**Owner:** `database` (Markdown only).
**Target:** `migrations/sqlite/00002_business_schema.sql` plus the minimal
`gormstore` changes named below.
**Inputs:** `docs/MIGRATION.md`, `docs/DOMAIN.md`, `docs/SYNC.md`,
`docs/ARCHITECTURE.md`, `docs/BACKEND.md`, `docs/API-CONTRACT.md`,
`docs/TESTING.md`, `migrations/sqlite/00001_auth.sql`,
`migrations/postgres/00001_enums.sql` … `00006_sales.sql`,
`internal/adapters/outbound/persistence/gormstore/timestamp.go`,
`internal/db/sqlite.go`, and the legacy models/repositories/services.

---

## Verdict

SQLite can host the full business schema locally without sync metadata, but it
must not use `NUMERIC`/`REAL` for money, `DATETIME` for timestamps, or
PostgreSQL enum types.

**Table count discrepancy:** `docs/MIGRATION.md:80` calls the remaining schema
“15 business tables”. Inventory + Sales + Supply chain contain **13 business
tables**; the full schema is **15 tables including the 2 auth tables already in
`00001_auth.sql`**. This proposal creates the 13 missing tables in
`00002_business_schema.sql`. It does not recreate `users` or `refresh_tokens`.
The DDL below includes the two auth tables for dependency context only.

The blocking decisions are money representation, the GORM model mapping for
money, stock locking / unit-of-work before Sales, and the documented legacy
overpayment gap. Catalog can ship without a transaction port.

---

## 1. Storage decisions

| Concern | Decision | Rationale |
|---|---|---|
| Money | **Integer cents** in `INTEGER` columns, using the **same column names as PostgreSQL** (`price`, `cost`, `total_amount`, `amount_paid`, `unit_price`, `subtotal`, `unit_cost`, `old_price`, `new_price`, `last_quoted_cost`). No `REAL`, no `NUMERIC`, no `DATETIME` affinity. | Exact, additive in SQL, indexable, and 64-bit safe for the `NUMERIC(12,2)` range. Same names keep one GORM model across both backends. |
| Enums | **Uppercase `TEXT` + `CHECK (col IN (...))`** using the exact PostgreSQL labels. | Matches `DOMAIN.md:106-122`; one Go mapping for both backends. |
| Timestamps | **Fixed-width UTC `TEXT`** in `2006-01-02T15:04:05.000000000Z` form, written by `gormstore.Timestamp`. No `CURRENT_TIMESTAMP`, no `DATETIME`. | Lexicographic order is chronological; the existing `Timestamp` type already writes this format. |
| Booleans | **`INTEGER NOT NULL` 0/1** with explicit defaults. No business table has a boolean today; the auth tables already follow this. | SQLite has no boolean type; GORM converts. |
| Nullability/defaults | Mirror PostgreSQL, with the deliberate exceptions in §1.5. | Prevent offline rows that PostgreSQL rejects. |
| Table strictness | **`STRICT`** on every new table. | Rejects a `TEXT`/`REAL` value in a money `INTEGER` column and a numeric value in a timestamp `TEXT` column. The bundled driver is SQLite 3.53.4, above the 3.37 requirement. |
| Sync columns | **None.** No `sync_id`, `updated_at` on tables that do not already have it, `deleted_at`, `version`, tombstone, or outbox. | `docs/SYNC.md:50-62` says add these only with the sync protocol. |

### 1.1 Money: integer cents, same column names

- Store **cents** (`$ 1.234,56` → `123456`) in `INTEGER` columns.
- Columns keep the PostgreSQL names so one `gormstore` model serves both
  backends. **Do not use `_cents` suffixes**: they would force duplicate
  per-dialect persistence models and break the single-store contract.
- The adapter owns conversion:
  - write: validate the `decimal.Decimal` has at most 2 decimal places and is
    within `±9,999,999,999.99`; then `amount.Shift(2).IntPart()`.
  - read: `decimal.New(cents, -2)` from SQLite `int64`; parse the string /
    `[]byte` from PostgreSQL `numeric`.
  - never scan money into `float64`; reject a `float64` scan result.
- SQLite DDL stores `INTEGER`; `STRICT` makes a forgotten conversion fail
  loudly instead of silently storing `100x` or a `REAL`.
- The zero value of the adapter’s `Money` type should serialize in PostgreSQL
  form (decimal string). If a caller forgets to mark it as SQLite, the
  `STRICT` SQLite table rejects the write instead of corrupting the value.
- **Aggregates:** additive sums (`SUM(total_amount)`, `SUM(subtotal)`,
  `SUM(amount_paid)`) run in SQL; SQLite returns integer cents, PostgreSQL
  returns `numeric`. The adapter normalizes both to `decimal.Decimal`.
  Multiplication (`price * stock_quantity` for inventory value) is computed in
  Go with `decimal` to avoid `int64` overflow. Never divide by 100 in SQL.
- Range checks mirror `NUMERIC(12,2)`:
  `-999999999999 <= v <= 999999999999` cents.
- The five Python-side defaults (§1.5) are **not** database defaults.

### 1.2 Enums

Exact labels and `CHECK` sets:

| Column | Allowed labels | PostgreSQL declaration order |
|---|---|---|
| `sales.payment_status` | `PAID`, `PENDING`, `PARTIAL` | PAID, PENDING, PARTIAL |
| `sales.delivery_status` | `PENDING`, `PARTIAL`, `DELIVERED` | PENDING, PARTIAL, DELIVERED |
| `sales.payment_method` (nullable) | `CASH`, `CREDIT`, `DEBIT` | CASH, CREDIT, DEBIT |
| `purchase_orders.status` | `PENDING`, `SENT`, `RECEIVED`, `CANCELLED` | PENDING, SENT, RECEIVED, CANCELLED |
| `purchase_orders.payment_status` | `PENDING`, `PAID` | PENDING, PAID |

- API/JSON stays lowercase; the adapter maps lowercase API values to uppercase
  DB labels on writes/filters and back on reads.
- Server defaults exist in PostgreSQL for `payment_status`, `delivery_status`,
  `purchase_orders.status`, and `purchase_orders.payment_status`; keep the same
  `DEFAULT 'PENDING'` constants in SQLite. `payment_method` is nullable with no
  default.
- Do **not** rely on `ORDER BY status`: PostgreSQL uses enum declaration order,
  SQLite uses alphabetical order. No current endpoint sorts by status
  (`order-status-distribution` sorts by count; lists sort by `id`). If one is
  added, use an explicit `CASE` expression.

### 1.3 Timestamps

- New columns use `TEXT`, not `DATETIME`, to avoid numeric affinity.
- One format: `2006-01-02T15:04:05.000000000Z` (30 characters, literal `Z`).
- Write with `gormstore.Timestamp`; never `CURRENT_TIMESTAMP`, `datetime('now')`,
  or a numeric epoch string.
- Add a `CHECK ... GLOB` pattern so a malformed timestamp cannot be stored.
- Nullable columns use `CHECK (col IS NULL OR col GLOB '...')`.
- Compare instants in Go; do not compare formatted strings across backends.
- `created_at`, `order_date`, and `changed_at` become `NOT NULL`; Go supplies
  them. `updated_at` stays nullable and is set by the service on update.

### 1.4 Booleans

- No boolean columns exist in the 13 business tables.
- The existing auth tables already use `INTEGER NOT NULL DEFAULT 1/0`.
- If a boolean is added later, use
  `INTEGER NOT NULL DEFAULT 0 CHECK (col IN (0,1))`.
- Do not alter `users`/`refresh_tokens` to add CHECK constraints; that would
  require a SQLite table rebuild for no current benefit.

### 1.5 Nullability and defaults — deliberate differences from PostgreSQL

Everything not listed mirrors PostgreSQL exactly.

| Table.column | PostgreSQL | SQLite decision | Why |
|---|---|---|---|
| `products.stock_quantity` | nullable, no default | `INTEGER NOT NULL`, no default | A `NULL` bypasses `CHECK (stock_quantity >= 0)`. Go always supplies `0`. |
| `products.low_stock_threshold` | nullable, no default | `INTEGER NOT NULL`, no default | `stock <= NULL` is false and silently breaks `stock_status`. Go always supplies `10`. |
| `products.low_stock_threshold` | no CHECK | `CHECK (>= 0)` | Negative thresholds are invalid. Hardening only. |
| `sale_items.sale_id` | nullable | `INTEGER NOT NULL` | Prevents orphan line items. The service always sets it. |
| `sale_items.product_id` | nullable | `INTEGER NOT NULL` | Prevents orphan line items. The service always sets it. |
| `sale_items(sale_id, product_id)` | no unique constraint | `UNIQUE` | Race guard for “one row per product per sale”; the service already checks sequentially. |
| `order_items(order_id, product_id)` | no unique constraint | `UNIQUE` | Same race guard. |
| `supplier_catalogs(product_id, provider_id)` | no unique constraint | `UNIQUE` | Same race guard for the receive-order upsert. SQLite allows multiple `NULL`s; the service always sets both. |
| `clients.created_at` | nullable, `DEFAULT now()` | `TEXT NOT NULL`, no default | Go supplies the UTC instant; no `CURRENT_TIMESTAMP`. |
| `sales.created_at` | nullable, `DEFAULT now()` | `TEXT NOT NULL`, no default | Same. |
| `purchase_orders.order_date` | nullable, `DEFAULT now()` | `TEXT NOT NULL`, no default | Same. The receive logic subtracts `order_date`; `NULL` would fail. |
| `price_history.changed_at` | nullable, `DEFAULT now()` | `TEXT NOT NULL`, no default | Same. |
| Money columns | `NUMERIC(12,2)` | `INTEGER` cents | Exact SQLite arithmetic; see §1.1. |
| Text lengths | `varchar(n)` | `TEXT` + `CHECK (length(col) <= n)` | SQLite ignores `varchar(n)`. |
| `clients.name`, `identity_card`, `email`, `phone` | unbounded `varchar` | `TEXT` with no length CHECK | Mirrors PostgreSQL. |
| Redundant `ix_*_id` indexes | present | not created | The primary key already indexes `id`. |
| `ix_products_name`, `ix_providers_name`, `ix_users_full_name`, catalog name indexes | present | not created | A btree cannot serve `%term%`; search is handled in Go (§5). |

**The five Python-side defaults** — no SQLite `DEFAULT`; Go must supply them:

| Column | Legacy default | Required Go insert |
|---|---:|---|
| `products.stock_quantity` | 0 | `0` |
| `products.low_stock_threshold` | 10 | `10` |
| `sales.amount_paid` | 0 | `0` |
| `sale_items.delivered_quantity` | 0 | `0` |
| `purchase_orders.total_amount` | 0 | `0` (then recalculated from items) |

`amount_paid > total_amount` is **not** rejected by a CHECK. The legacy API
accepts overpayment and marks the sale `paid` (`DOMAIN.md:133-138`,
`MIGRATION.md:170`); `TESTING.md:61` contradicts this. See §7.

---

## 2. Complete DDL draft

Dependency order. The first two tables already exist in `00001_auth.sql` and
are shown only so the foreign keys below are readable. **`00002_business_schema.sql`
must not recreate them.** The new migration starts at `categories`.

```sql
-- +goose Up
-- Phase 3 SQLite business schema. Requires 00001_auth.sql (users, refresh_tokens).
-- Money columns are INTEGER cents. Timestamps are fixed-width UTC TEXT.
-- Five legacy Python-side defaults are supplied by Go, not by SQL DEFAULT.

-- ===========================================================================
-- 0. EXISTING AUTH TABLES (already in 00001_auth.sql — do not recreate)
-- ===========================================================================
-- users(id, full_name, email, hashed_password, is_active, is_superuser)
-- refresh_tokens(id, user_id, token_hash, expires_at, revoked)

-- ===========================================================================
-- 1. CATALOG
-- ===========================================================================

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

-- ===========================================================================
-- 2. PRODUCTS
-- ===========================================================================

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

-- ===========================================================================
-- 3. CLIENTS AND PROVIDERS
-- ===========================================================================

CREATE TABLE clients (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT,
    name          TEXT NOT NULL,
    identity_card TEXT,
    email         TEXT,
    phone         TEXT,
    CONSTRAINT clients_identity_card_key UNIQUE (identity_card),
    CONSTRAINT ck_clients_created_at_format CHECK (
        created_at GLOB
        '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]Z'
    ),
    CONSTRAINT ck_clients_updated_at_format CHECK (
        updated_at IS NULL OR updated_at GLOB
        '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]Z'
    )
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

-- ===========================================================================
-- 4. SUPPLY CHAIN
-- ===========================================================================

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
CREATE UNIQUE INDEX ux_supplier_catalogs_product_provider
    ON supplier_catalogs (product_id, provider_id);

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
    CONSTRAINT ck_purchase_orders_order_date_format CHECK (
        order_date GLOB
        '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]Z'
    ),
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
CREATE UNIQUE INDEX ux_order_items_order_product
    ON order_items (order_id, product_id);

-- ===========================================================================
-- 5. SALES
-- ===========================================================================

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
CREATE UNIQUE INDEX ux_sale_items_sale_product
    ON sale_items (sale_id, product_id);

-- Forward-only: no Down. Restore the pre-migration backup to roll back.
```

### 2.1 Index rationale

Every non-unique index names the query it serves.

| Index | Query served |
|---|---|
| `ix_products_sku` (unique) | `GET /inventory/products` search/uniqueness, `POST` duplicate SKU |
| `ix_products_brand_id` | `GET /inventory/products?brand_id=` |
| `ix_products_category_id` | `GET /inventory/products?category_id=` |
| `ix_products_material_id` | `GET /inventory/products?material_id=` |
| `ix_products_stock_quantity` | `GET /inventory/products?stock_status=in_stock|out_of_stock` (`> 0`, `= 0`); the `low_stock` predicate also compares to `low_stock_threshold` and may still scan |
| `ix_price_history_product_id` | `DELETE FROM products` cascade, price-history list |
| `ix_supplier_catalogs_product_id` | `GET /supply-chain/supplier-catalogs?product_id=` |
| `ix_supplier_catalogs_provider_id` | `GET /supply-chain/supplier-catalogs?provider_id=` |
| `ix_purchase_orders_provider_id` | `GET /supply-chain/purchase-orders?provider_id=` |
| `ix_purchase_orders_status` | `GET /supply-chain/purchase-orders?status=`, dashboard `pending_orders` |
| `ix_purchase_orders_order_date` | `min_date`/`max_date`, 7-day orders trend |
| `ix_purchase_orders_payment_status` | `GET /supply-chain/purchase-orders?payment_status=` |
| `ix_order_items_order_id` | `GET /supply-chain/purchase-orders/{id}/items` |
| `ix_order_items_product_id` | product delete FK check, order-item joins |
| `ix_sales_created_at` | `GET /sales?start_date=&end_date=`, dashboard today/7-day ranges |
| `ix_sales_payment_status` | `GET /sales?payment_status=`, dashboard pending payment |
| `ix_sales_delivery_status` | `GET /sales?delivery_status=`, dashboard delivered count |
| `ix_sales_client_id` | `GET /sales?client_id=`, `no_client=true` |
| `ix_sales_created_by_id` | `GET /sales?user_id=` |
| `ix_sales_payment_method` | `GET /sales?payment_method=` |
| `ix_sale_items_sale_id` | `GET /sales/{id}/items`, dashboard top-products join |
| `ix_sale_items_product_id` | product delete FK check, top-products join |
| unique catalog names | create/update duplicate-name checks |

**Not ported on purpose:** `ix_*_id` (primary key already indexes `id`),
`ix_products_name`, `ix_providers_name`, `ix_users_full_name`, and catalog name
indexes (a btree cannot serve `%term%`; see §5).

**Measure before adding compound/partial indexes:** `sales(payment_status,
created_at)`, `sales(created_at, id)`, `purchase_orders(status, order_date)`,
`products(category_id, id)`. Add only after `EXPLAIN ANALYZE` on a
production-sized fixture.

---

## 3. Migration strategy

### 3.1 File and versioning

- Add **`migrations/sqlite/00002_business_schema.sql`**. Do not edit
  `00001_auth.sql`; it has shipped and is immutable.
- SQLite versions are independent of PostgreSQL versions:

| Backend | Version | File | Contents |
|---|---:|---|---|
| PostgreSQL | 1–6 | `00001_enums` … `00006_sales` | full legacy schema |
| SQLite | 1 | `00001_auth.sql` | `users`, `refresh_tokens` |
| SQLite | 2 | `00002_business_schema.sql` | 13 business tables + indexes |

- The two `goose_db_version` tables are separate. Do not try to keep the
  version numbers equal. Document this map in `docs/MIGRATION.md`.

### 3.2 Fresh versus existing SQLite

- **Fresh database:** goose applies `00001` then `00002`. 00002 references
  `users(id)`, which exists by then.
- **Existing database at version 1:** goose applies 00002 only. It creates new
  tables; it does not touch the auth rows. This is the normal upgrade path.
- **Database created outside goose** (manual `CREATE TABLE`, no
  `goose_db_version` row): 00001 will fail. The file must be created by the app.
  Document it; do not add `IF NOT EXISTS` to shipped files.
- **Collision with ad-hoc business tables:** `CREATE TABLE` fails, goose wraps
  the migration in a transaction, and the database stays at version 1. Do not
  “fix” it by editing 00002; inspect and remove the ad-hoc tables after a
  backup.
- **Auth tables:** no change. No `created_at` on `users`, no sync columns on
  `refresh_tokens`. The existing `refresh_tokens.user_id` index and
  `ON DELETE CASCADE` are sufficient.

### 3.3 Forward-only and retry

- **No `-- +goose Down`.** Dropping business tables destroys data. Reverting
  means restoring the pre-migration backup.
- Goose wraps the DDL in a transaction on SQLite; a failed `up` rolls back.
  Retrying after fixing the cause is safe. Do not add `IF NOT EXISTS`; it hides
  drift.
- The existing `00001_auth.sql` has a destructive Down. Do not change it in this
  task (immutable), but flag it for a follow-up under the forward-only policy.
- `AUTO_MIGRATE=false` on a fresh file leaves it empty. Phase 3 should either
  require `AUTO_MIGRATE=true` on first run or add an explicit `migrate` command.
  This is a pre-existing gap, not caused by 00002.

### 3.4 Concurrent startup

Two app processes can race goose. SQLite has a 5-second busy timeout and one
connection per process, but goose has no cross-process migration lock. Phase 3
should keep “one instance per SQLite file” until an OS single-instance lock or a
migration lock exists. Document it.

---

## 4. Parity test plan — catalog and products

The repository contract suite must run the same fixtures through the
`gormstore` against PostgreSQL 17 and SQLite. Use the existing
`internal/adapters/outbound/persistence/contract_test.go` pattern. Seed through
the repositories, not raw SQL, so the money adapter is exercised.

| ID | Fixture | Assertions on both backends |
|---|---|---|
| P-01 | Users: create two, duplicate email, list `search=ana`, page with `limit=1` | Same status/code; same IDs; `has_next`; no duplicate/skipped rows |
| P-02 | Catalog: create category/material/brand/unit; duplicate `name`, `code`, `abbreviation`; delete referenced and unreferenced | Same 4xx and error code; `PRAGMA foreign_key_check` empty; FK violation on referenced delete |
| P-03 | Products: create with generated `SKU-XXXXXXXX`; duplicate `sku`; `price`/`cost` 0 and negative; `stock_quantity` negative; `stock_status` filters; `category_id`/`brand_id`/`material_id` filters; related objects loaded | Same 4xx/422; same ordered IDs; no N+1; generated SKU shape matches |
| P-04 | Money exactness: seed `0.01`, `0.10`, `0.20`, `1234.56`, `9999999999.99`; sum them | API JSON compares as decimals, never float; SQLite `typeof(price)='integer'`; PostgreSQL `pg_typeof(price)='numeric'`; `SUM` equals the decimal expectation (e.g. `0.30`, `2469.12`) |
| P-05 | Enums: write/read/filter every payment method and status; insert an invalid label directly | DB stores uppercase labels, API returns lowercase; invalid API value 422; direct invalid insert fails the CHECK on both |
| P-06 | Timestamps: create rows with 0 ns, trailing-zero ns, microseconds; equal timestamps; `start_date`/`end_date` boundaries | Stored value is 30-char UTC `...Z`; lexical order equals chronological; API is RFC 3339; no `YYYY-MM-DD HH:MM:SS` |
| P-07 | Defaults: omit `stock_quantity`, `low_stock_threshold`, `amount_paid`, `delivered_quantity`, `total_amount` | API returns `0`, `10`, `0`, `0`, `0`; SQLite rejects a direct insert that omits a `NOT NULL` default; PostgreSQL accepts a direct `NULL` (documented DB-level difference, never reachable through the API) |
| P-08 | Pagination ties: 5 products with equal `name` and 5 sales with equal `created_at`; page `limit=2` | Same IDs in the same order (`id DESC` lists, `id ASC` items); no duplicates/skips; `has_next` correct at exact boundaries |
| P-09 | FK deletion: product with price history; product referenced by sale item/order item/catalog; category referenced by product; client referenced by sale; user referenced by sale/provider/order | `price_history` cascades; `refresh_tokens` cascades; every other delete fails with the same status/code; child rows unchanged |
| P-10 | Search: `árbol` vs `ÁRBOL`; `%` and `_` in the term | Same IDs for both cases on both backends; literal wildcards are escaped; no SQL injection |
| P-11 | Dashboard (later gate): sales at `04:59:59Z`, `05:00:00Z`, `05:00:01Z`; 7-day window | Same `America/Bogota` buckets; zero-filled days; same sums |
| P-12 | Stock (Sales gate): two concurrent last-unit sales; receive an order twice | Exactly one sale succeeds; stock never negative; received stock increments once |

Direct-SQL parity checks:

```sql
-- SQLite
PRAGMA foreign_key_check;
SELECT typeof(price), typeof(created_at) FROM products LIMIT 1; -- integer, text

-- PostgreSQL
SELECT pg_typeof(price), pg_typeof(created_at) FROM products LIMIT 1; -- numeric, timestamptz
```

For P-04 and P-07, separate the **API-level** assertion (must be identical) from
the **DB-level** assertion (documents the intentional SQLite strictness). Do not
weaken the API fixture to make the DB difference pass.

---

## 5. SQL gaps and handling

| Gap | PostgreSQL | SQLite | Handling | Gate |
|---|---|---|---|---|
| `ILIKE` | locale-aware case folding | `LIKE`/`lower()` fold ASCII only | For SQLite, fetch the filtered set and apply Go `strings.ToLower`/`strings.Contains` (Unicode-aware); keep PostgreSQL `ILIKE`. This is a documented adapter branch. | No; measure |
| Accented search (`árbol` vs `ÁRBOL`) | matches | does not match with SQL `LIKE` | Same Go-side case folding for SQLite. This is **case folding**, not accent stripping; PostgreSQL `ILIKE` does not strip accents either. | No |
| Leading `%term%` index use | needs `pg_trgm` + GIN | no btree; FTS5 possible | Do not add a btree for search. Measure before adding `pg_trgm`/FTS5. | Measure |
| `RETURNING` | supported | supported (3.35+) | The bundled driver is SQLite 3.53.4. Use `RETURNING id`/GORM where needed; test on the release build. | No |
| Enum sort | declaration order | alphabetical | No current endpoint sorts by status. If one is added, use an explicit `CASE`; never rely on enum order. | No |
| Money aggregates | `numeric` | integer cents | Normalize in the adapter (`int64` cents → `decimal.New(v,-2)`; string → `decimal.NewFromString`). Never float. | No |
| `price * stock_quantity` | `numeric` | `int64` can overflow | Compute inventory value in Go with `decimal`, not SQLite multiplication. | No |
| Date grouping `America/Bogota` | `AT TIME ZONE` + `date()` | no `AT TIME ZONE`/`date_trunc` | Compute the Bogotá UTC range in Go, filter with the indexed `created_at`/`order_date`, and bucket/zero-fill in Go. Do not use `datetime(ts,'-5 hours')` against 9-digit fractional text without a test. | No |
| `CURRENT_TIMESTAMP` | `now()` | wrong format (`YYYY-MM-DD HH:MM:SS`) | Go supplies `gormstore.Timestamp`; no DB default. | No |
| `FOR UPDATE` | row locks | no row locks | UoW + `BEGIN IMMEDIATE` (`_txlock=immediate`) + conditional `UPDATE ... WHERE stock_quantity >= ?` and `RowsAffected`; PG locks sorted IDs. | Sales/Orders |
| `ON CONFLICT` upsert | no unique index to target | has the new unique index | Keep the legacy select-then-update; do not add a PG unique index in this phase. | No |
| `NUMERIC(12,2)` | exact | no decimal type | Integer cents. | No |
| Boolean | `boolean` | `INTEGER` | No business booleans; auth already uses 0/1. | No |

---

## 6. Transaction / unit of work

### 6.1 What Catalog needs now

**Nothing beyond single-row CRUD.** The legacy product create is one insert
(`services/inventory.py:81-84`); it does not write `price_history`. Product
delete with a price history is a single `DELETE` with the FK cascade. Catalog
lists, filters, and pagination are reads. Do not build a transaction port for
Catalog.

### 6.2 What Sales/Orders must have

Before Sales, add a transaction boundary that spans repositories and a stock
port. Do not design the full sales flow here.

Minimal port shape:

```go
type TxRepos struct {
    Sales    outbound.SaleRepository
    Products outbound.ProductRepository
    Clients  outbound.ClientRepository
    Orders   outbound.PurchaseOrderRepository
    Catalogs outbound.SupplierCatalogRepository
    Stock    outbound.StockRepository
}

type UnitOfWork interface {
    WithinTx(ctx context.Context, fn func(context.Context, TxRepos) error) error
}

type StockRepository interface {
    // LockForUpdate returns current quantities in ascending product-id order.
    LockForUpdate(ctx context.Context, ids []int64) (map[int64]int64, error)
    // ApplyDelta is conditional; it returns ErrInsufficientStock when the
    // new quantity would be negative.
    ApplyDelta(ctx context.Context, id int64, delta int64) error
}
```

- `WithinTx` on PostgreSQL: `db.Transaction`. Lock products in ascending `id`
  order to avoid deadlocks.
- `WithinTx` on SQLite: `BEGIN IMMEDIATE` (add `_txlock=immediate` to the DSN)
  so two processes cannot interleave a read-then-write stock check. Keep
  `SetMaxOpenConns(1)` and the 5-second busy timeout until measured.
- `ApplyDelta` on SQLite:
  `UPDATE products SET stock_quantity = stock_quantity + ? WHERE id = ? AND
  stock_quantity + ? >= 0`; check `RowsAffected`. The schema CHECK is the last
  line of defense.
- Order receiving idempotency:
  `UPDATE purchase_orders SET status = 'RECEIVED' WHERE id = ? AND status <>
  'RECEIVED'`; apply stock and upsert the catalog only when `RowsAffected == 1`.
- Error mapping: add `domain.ErrConflict` (unique), `ErrForeignKeyViolation`,
  `ErrInsufficientStock`, and an invalid-state error. Map PostgreSQL SQLSTATE
  (`23505`, `23503`, `23514`) and SQLite constraint codes (`2067`, `787`,
  `275`) in the adapter.
- Never issue a second query on `r.db` while a `Rows` cursor is open on the
  single SQLite connection. Join product relations in the list query; do not
  N+1.

---

## 7. Ranked risks and open questions

### 7.1 Must decide before implementation

1. **Money model mapping (High).** Recommended: integer cents in columns named
   like PostgreSQL, with a `gormstore.Money` type whose write form is
   dialect-aware and whose read form infers `int64` vs string. The zero value
   must serialize in PostgreSQL form so `STRICT` rejects a forgotten SQLite
   conversion. If the backend prefers `_cents` names, it must accept duplicate
   per-dialect GORM models; do not mix the two approaches.
2. **Stock / unit-of-work before Sales (High).** The schema alone cannot prevent
   oversell across two processes. The port in §6.2 and `_txlock=immediate` must
   land before Sales/Orders. Catalog is not blocked.
3. **Overpayment rule conflict (Medium).** `DOMAIN.md:133-138` and
   `MIGRATION.md:170` say preserve `amount_paid > total_amount`; the system
   task guidance and `TESTING.md:61` say reject it. This proposal **does not add
   a CHECK** and preserves legacy. Product/manager must confirm before Sales.
4. **Search case folding (Medium).** Recommended: Go-side Unicode case folding
   for SQLite so `árbol`/`ÁRBOL` match like PostgreSQL `ILIKE`; mark the
   full-scan cost and measure. The alternative is to gate non-ASCII search as a
   documented difference.
5. **Natural-key uniqueness (Medium).** The proposal adds unique indexes on
   `sale_items`, `order_items`, and `supplier_catalogs`. PostgreSQL does not
   have them; the service checks sequentially. This is a deliberate SQLite
   race guard. If exact DB-level parity is required, drop them and document
   that only the service enforces the invariant.
6. **Timestamp nullability (Medium).** Recommended: `created_at`, `order_date`,
   `changed_at` `NOT NULL`; Go supplies them. This is stricter than PostgreSQL’s
   nullable `DEFAULT now()`. Confirm the Go insert paths set them.
7. **`STRICT` support (Low).** Requires SQLite ≥ 3.37; the bundled driver is
   3.53.4. Confirm on every target OS/release build. If the driver is ever
   downgraded, `STRICT` must be removed in a new migration, not edited.

### 7.2 Measure first

- **Index set.** Run `EXPLAIN`/`EXPLAIN ANALYZE` on a production-sized fixture
  for product filters, sales date/status filters, and dashboard ranges. Add
  compound indexes only with evidence.
- **Go-side dashboard grouping and search.** Measure the 7-day row volume and
  catalog size. If the Go-side path is too slow, add FTS5 for SQLite or
  `pg_trgm` for PostgreSQL—not a speculative btree.
- **N+1 and the one connection.** A nested query while a cursor is open will
  deadlock SQLite. Join relations; measure before raising the pool.
- **Inventory value.** Go-side decimal multiplication avoids `int64` overflow;
  measure product count before deciding whether SQL is safe.
- **`price * stock` and `SUM` overflow.** The per-column range checks bound
  values, but large row counts can still overflow `int64`; keep the Go-side
  multiplication and additive SQL sums.

### 7.3 Deferred / out of scope

- Sync columns, stock movement ledger, tombstones, outbox, device/cursor
  tables (`docs/SYNC.md:50-62`).
- Any PostgreSQL schema change. This is a SQLite-only migration; PG stays
  faithful to the legacy dump.
- Writing `price_history` on product price updates. The legacy service does not
  do it; adding it is a behavior change requiring product approval.
- SQLite encryption at rest, fallback visibility, single-instance lock, and
  file permissions. These are existing security/ops items, not schema changes.
- `users`/`refresh_tokens` changes. The auth schema is correct for Phase 3.

---

## Safe migration plan

1. **Back up** the SQLite file before upgrading:
   `sqlite3 <path> ".backup '<path>.pre-00002.bak'"` (or `VACUUM INTO`), then
   `PRAGMA integrity_check` on the backup. Copying the main file alone is not
   enough while WAL is active; include `-wal`/`-shm` or use the backup API.
2. Confirm the existing DB is at `goose_db_version` version 1 and that the
   bundled SQLite is ≥ 3.37 (`STRICT`).
3. Add `migrations/sqlite/00002_business_schema.sql` exactly as §2. Do not
   edit `00001_auth.sql`.
4. Rehearse on a copy of a production-sized SQLite DB: run `goose up`, then
   `PRAGMA foreign_key_check`, `PRAGMA integrity_check`, and assert 13 new
   tables plus the expected indexes.
5. Run the P-01…P-10 parity suite against PostgreSQL 17 and the migrated SQLite
   copy. Do not expose Catalog until it passes.
6. Roll out with `AUTO_MIGRATE=true` (default) or an explicit `migrate` command.
   Goose runs 00002 in a transaction; a failure leaves the DB at version 1.
7. Post-flight: `goose status` shows version 2; `PRAGMA foreign_key_check` is
   empty; login and a catalog read/write smoke test pass on both backends.
8. **Rollback:** there is no Down. Restore the backup. Never drop business
   tables on a machine that may already contain data.

---

## Open questions

1. Does product/manager confirm preserving `amount_paid > total_amount`?
2. Does backend accept the dialect-aware `Money` type and same column names, or
   does it prefer `_cents` names plus per-dialect models?
3. Is Go-side Unicode case folding acceptable for SQLite search, or is the
   non-ASCII difference gated for Phase 3?
4. Are the three natural-key unique indexes approved as SQLite-only hardening?
5. Is `STRICT` approved for the new tables on every target OS/release build?
6. Who owns the `UnitOfWork`/`StockRepository` port, and does it land before
   Sales or before Orders?
7. Should the `AUTO_MIGRATE=false` fresh-file gap and the destructive
   `00001_auth.sql` Down be fixed in a separate migration/ops task?
