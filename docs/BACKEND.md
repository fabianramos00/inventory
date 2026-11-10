# Backend

The backend is a Go REST API embedded in the Wails process. It mirrors the
legacy FastAPI contract and owns database access through backend-specific
adapters.

## Stack

- Go 1.26.
- Standard library `net/http` with `http.ServeMux` method patterns.
- GORM (`gorm.io/gorm`) behind outbound adapters, with
  `gorm.io/driver/postgres` and pure-Go `github.com/glebarez/sqlite`.
- `pgx/v5` underneath the PostgreSQL GORM driver.
- `goose` for embedded SQL migrations, run on the `*sql.DB` GORM exposes.
- `golang-jwt/jwt/v5` for access tokens.
- `golang.org/x/crypto/bcrypt` for passwords.
- `shopspring/decimal` for money.
- `log/slog` for structured logs.
- `testing` + `httptest` for tests.

GORM is limited to `internal/adapters/outbound/persistence/gormstore` and the DB open helpers in `internal/db`; it
never leaks into domain, ports, application, or inbound HTTP. Migrations are
always goose SQL, never GORM AutoMigrate. Raw SQL remains allowed inside the
adapter for dashboard aggregates, stock locking, and complex queries. No web
framework. No new dependency without an entry in [DECISIONS.md](DECISIONS.md).

## Layout

- `main.go`, `app.go`, `main_bindings.go`: Wails entry, lifecycle, and bindings.
- `internal/application/`: use cases; imports only domain and ports.
- `internal/ports/{inbound,outbound}/`: use-case and repository/security interfaces.
- `internal/domain/`: entities and domain errors (no database or HTTP tags).
- `internal/adapters/inbound/httpapi/`: router, middleware, handlers, HTTP DTOs.
- `internal/adapters/outbound/persistence/gormstore/`: GORM models and repositories.
- `internal/adapters/outbound/security/`: JWT, bcrypt, refresh-token crypto.
- `internal/{config,db,bootstrap}/`: configuration, database open/migrate, wiring.
- `migrations/{postgres,sqlite}/`: embedded goose SQL files.

Dependency direction: `adapters/inbound/httpapi → ports/inbound ← application → ports/outbound ← adapters/outbound/{persistence,security}`.
The domain package is shared. Application services depend only on inbound and
outbound port interfaces. Inbound handlers depend on `inbound.AuthUseCase`, not
on concrete services. Handlers never issue SQL. Repositories never write HTTP
responses.

## HTTP layer

- Register routes with method patterns: `mux.HandleFunc("GET /api/v1/sales/{id}", h)`.
- Path values: `r.PathValue("id")`.
- Decode JSON with `json.NewDecoder`, reject unknown fields where practical,
  and enforce a body size limit (10 MiB, matching the legacy middleware).
- Always set `Content-Type: application/json`.
- Use `http.MaxBytesReader` for request bodies.
- Timeouts: read header 5s, read 15s, write 30s, idle 60s.
- Graceful shutdown with a 10s context.
- Request ID middleware adds `X-Request-Id`; do not log bodies.

Middleware order:

```text
recovery → request ID → logging → CORS → auth → handler
```

Auth whitelist: `/api/v1/auth/login`, `/api/v1/auth/refresh`,
`/api/v1/auth/logout`, `GET /healthz`.

## Service layer

Services own business rules and transactions:

- Validate references and state transitions.
- Calculate sale/order totals and statuses.
- Apply stock changes with row locks.
- Recalculate aggregate fields after item mutations.
- Return typed domain errors that handlers map to HTTP status codes.

Do not put business rules in handlers or repositories. Do not start a
transaction in a repository method that is called inside a service
transaction.

## Repository adapters

Ports live in `internal/ports`. `internal/ports/inbound` defines use cases
consumed by inbound adapters; `internal/ports/outbound` defines repositories and
security services consumed by application services. One GORM adapter,
`internal/adapters/outbound/persistence/gormstore`, implements the same
outbound interfaces for PostgreSQL and SQLite. Bootstrap selects the backend
and injects the repositories.

Rules:

- Keep GORM types and imports inside `gormstore` and `internal/db`; return domain entities.
- Use GORM for CRUD. Use raw SQL through the same `*gorm.DB` session
  for dashboard aggregates, stock locking, `RETURNING` rotations, and other
  complex statements.
- Parameterized queries only. Never concatenate SQL values.
- Pagination fetches `limit + 1` rows and returns `(items, hasNext)`.
- Search uses the legacy columns and stays case-insensitive; wildcard
  handling is documented per module. Cap input length.
- Lock products with `SELECT ... FOR UPDATE` before stock changes.
- Map `gorm.ErrRecordNotFound` and `sql.ErrNoRows` to `domain.ErrNotFound`.
- Multi-row writes use `db.Transaction` so both dialects commit atomically.
- Configure GORM with the silent logger; never log SQL values, tokens,
  passwords, or full rows.
- `gormstore.Timestamp` stores fixed-width UTC text. PostgreSQL accepts the
  same string for `timestamptz`, and SQLite keeps the lexical order used by
  `expires_at > ?`.
- SQLite booleans are stored as integers; GORM converts them on read.
- Scan `NUMERIC` into a decimal, never into `float64`:

```go
var raw string
if err := row.Scan(&raw); err != nil { ... }
amount, err := decimal.NewFromString(raw)
```

Use `pgtype.Numeric` only when a raw query needs its integer/scale parts. Add a
compile-time interface assertion when implementing a port.

## Data model

Domain structs live in `internal/domain` and carry no database or HTTP
coupling, including JSON tags. The inbound HTTP adapter owns request/response
DTOs and their JSON tags; behavior must match
[API-CONTRACT.md](API-CONTRACT.md).

Money fields are `decimal.Decimal`. JSON marshaling must emit a number.
Implement a small `Money` wrapper only if both encoding and scanning are
simpler with it; otherwise marshal `decimal.Decimal` through a custom type at
the HTTP boundary.

Dates are `time.Time` in UTC. Format responses with `time.RFC3339`.

## Auth

- Access token: HS256 JWT, `sub` = user ID, expiry 30 minutes.
- Refresh token: 32 random bytes, URL-safe base64; store only a SHA-256 hash.
- Refresh rotates the token and revokes the old row in one transaction.
- Logout revokes the presented refresh token.
- Login accepts form data and uses the email in `username`.
- bcrypt cost from config, default 12; tests use 4.
- Reject inactive users on login and on every authenticated request.
- `is_superuser` is required for all `/users` routes.
- JWT, bcrypt, and refresh-token crypto are outbound security adapters under
  `internal/adapters/outbound/security`; the composition root injects them into
  `application.AuthService` through the outbound ports.
- Auth sentinel errors live in `internal/domain` and are mapped to HTTP
  responses by the inbound adapter.

## Users

- All `/users` routes require an authenticated superuser; other users get `403`.
- List supports `skip`, `limit` (default 100, max 200), and `search` (max 256)
  over `full_name` and `email`, case-insensitive on both backends.
- Email is trimmed and validated with `net/mail`; password is 8–128 characters
  and at most 72 bytes.
- `POST /users` returns `200` to match legacy behavior. Update is partial; an
  empty password means unchanged.
- Duplicate email maps to `400` with the legacy detail. Missing users map to
  `404`. Foreign-key conflicts map to `400` with `FOREIGN_KEY_VIOLATION`.

Never log passwords, tokens, authorization headers, or full request bodies.

## Catalog

- Endpoints under `/api/v1/inventory`: categories and brands expose list,
  create, get, update, and delete; materials and measurement units expose list,
  create, update, and delete (no `GET` by id, matching `include_get=False`).
- All routes need any active user; only `/users` requires a superuser.
- Lists return `{items, has_next, skip, limit}`, order `id DESC`, and search
  `name` only. `search` is trimmed and capped at 256 runes; `limit` defaults to
  100 and is capped at 200.
- Create/update/delete return the entity with `200`. Update is partial; omitted
  and `null` string fields stay unchanged, and an empty string clears an
  optional field. Required names and unit abbreviations reject empty values.
- Validation mirrors database lengths: names 150, category code 10, category
  description 500, brand `logo_url` 255, unit abbreviation 5.
- Duplicates map to `400 UNIQUE_VIOLATION`; referenced deletes map to
  `400 FOREIGN_KEY_VIOLATION`; missing rows use the legacy details
  (`Category not found`, `Brand not found`, `Material not found`,
  `Unit of Measure not found`).
- Search treats `%` and `_` literally on both backends — a deliberate
  difference from legacy `ILIKE` wildcard behavior, documented in
  [ADR-0015](DECISIONS.md#adr-0015--inventory-search-treats-sql-wildcards-literally).
  PostgreSQL uses `strpos(lower(name), lower(?)) > 0`; SQLite uses Go Unicode
  case folding over a full-table scan (`ponytail:` add FTS5 if catalog size
  makes the scan measurable).

## Products

- Endpoints under `/api/v1/inventory/products`: list, create, get, update, and
  delete, plus `GET /products/stats/overview`. Create/update/delete return
  `200` with the plain product; list and get include the category, material,
  brand, and measurement unit.
- List filters: `search`, `category_id`, `brand_id`, `material_id`, and
  `stock_status` (`in_stock`, `low_stock`, `out_of_stock`). Search matches
  `name` and `sku`, case-insensitive and literal; order is `id DESC`; pagination
  follows the shared `{items, has_next, skip, limit}` rules.
- `stock_status`: `in_stock` is stock > 0, `low_stock` is stock > 0 and stock <=
  `low_stock_threshold`, and `out_of_stock` is stock = 0.
- Create requires `name`, `price`, `cost`, `brand_id`, `category_id`, and
  `material_id`. `stock_quantity` defaults to 0 and `low_stock_threshold` to
  10; explicit nulls are `422`. An omitted or empty `sku` generates
  `SKU-XXXXXXXX` from cryptographic randomness and retries on generated
  collisions. A user-supplied duplicate sku is `400 UNIQUE_VIOLATION`.
- Product `name` and any supplied `sku` are trimmed before validation (legacy
  does not trim). `sku` and `size_value` have rune limits of 100 and 20.
  `price` and `cost` must be positive and at most `9999999999.99`; they are
  rounded half away from zero to two decimals. IDs must be positive.
- JSON bodies reject unknown fields with `422 VALIDATION_ERROR`; see
  [ADR-0017](DECISIONS.md#adr-0017--reject-unknown-json-fields-at-the-http-boundary).
- Updates are partial. Omitted fields stay unchanged; explicit null clears only
  `size_value` and `measurement_unit_id`, and is `422` for every other field.
  An empty `size_value` also clears it.
- Stats: `total_products`, `total_stock`, `low_stock_count`, and
  `total_inventory_value`. Inventory value uses sale `price`, not `cost`, and
  is returned as a decimal-derived JSON number.
- PostgreSQL `price`/`cost` are `NUMERIC(12,2)`, read as text and written with
  two fixed decimals. SQLite stores integer cents: reads use
  `decimal.New(cents, -2)` and writes use `rounded.Shift(2).IntPart()`; stats
  sums stay integer until conversion. See
  [ADR-0016](DECISIONS.md#adr-0016--sqlite-money-is-stored-as-integer-cents).
- `price_history` is never written. The legacy API declared the table and
  cascade but did not record product price changes, so the port preserves that
  behavior.

## Clients

- Endpoints under `/api/v1/sales/clients`: list, create, get, update, and
  delete. Any active user may call them; only `/users` requires a superuser.
- Lists return `{items, has_next, skip, limit}`, order `id DESC`, and share the
  default/cap (100/200) and search limit (256 runes) rules. Search matches
  `name`, `identity_card`, `email`, and `phone`, case-insensitively and
  literally.
- Item shape: `{id, name, identity_card, email, phone, debt_amount,
  created_at, updated_at}`. Timestamps are RFC 3339 UTC. `created_at` is set on
  create and `updated_at` stays `null` until a field actually changes.
  `debt_amount` is the non-negative server-maintained cache
  `SUM(max(sale.total_amount - sale.amount_paid, 0))` over the client's sales,
  stored as `NUMERIC(12,2)` on PostgreSQL and integer cents on SQLite, and
  never accepted from a request body.
- `name` is required and trimmed. The three optional fields are trimmed;
  empty strings become `null`. Create accepts them as omitted, `null`, or
  empty. Update is partial: omitted fields stay unchanged, explicit `null`
  clears an optional field, and `null`/empty/blank `name` is `422`.
- `updated_at` is set only when at least one field changes; a no-op or
  whitespace-only update leaves it untouched.
- Duplicate `identity_card` maps to `400 UNIQUE_VIOLATION`; deleting a client
  referenced by a sale maps to `400 FOREIGN_KEY_VIOLATION`; missing rows map
  to `404 Client not found`. Email and phone have no format validation,
  matching legacy.
- Search treats `%` and `_` literally on both backends. PostgreSQL uses
  `strpos(lower(column), lower(?)) > 0` over the four fields; SQLite scans the
  table and folds case in Go (`ponytail:` add FTS5 when the client table makes
  the scan measurable).
- Timestamps are stored through `gormstore.Timestamp` on both backends.
  PostgreSQL keeps microseconds and SQLite keeps nanoseconds, so exactness
  beyond microseconds is backend-specific.

## Providers

- Endpoints under `/api/v1/supply-chain/providers`: list, create, get, update,
  and delete. Any active user may call them; the response is
  `{id,name,contact_info,email,phone}` with no timestamps.
- Lists return `{items, has_next, skip, limit}`, order `id DESC`, and share the
  default/cap (100/200) and search limit (256 runes) rules. `search` is trimmed
  and matches `name`, `email`, and `phone` only; `contact_info` is not searched.
- `name` is required, trimmed, and at most 255 characters. `contact_info`,
  `email`, and `phone` are optional, trimmed, and stored as `null` when empty;
  their limits are 500, 255, and 50. Email and phone formats are not validated.
- Create sets `created_by_id` from the authenticated user and never exposes it.
  Update is partial: omitted fields stay unchanged, explicit `null` or an empty
  string clears an optional field, and `null`/blank `name` is `422`. A no-op
  update leaves the row unchanged.
- Duplicate names are allowed; the table has no unique constraint. Deleting a
  provider referenced by a purchase order or supplier catalog maps to `400
  FOREIGN_KEY_VIOLATION`; missing providers map to `404 Provider not found`.
- Search treats `%` and `_` literally on both backends. PostgreSQL uses
  `strpos(lower(column), lower(?)) > 0` over the three fields; SQLite scans the
  table and folds case in Go (`ponytail:` add FTS5 when the provider table
  makes the scan measurable).

## Supplier catalogs

- Read-only endpoints under `/api/v1/supply-chain/supplier-catalogs`: list and
  get. There are no create/update/delete routes through the API; catalog rows are
  written only when a purchase order is received.
- Responses include `id`, `product_id`, `provider_id`, `supplier_sku`,
  `last_quoted_cost`, `lead_time`, the full `provider` object, and the plain
  `products` object (no catalog relations), matching legacy.
- Lists return `{items, has_next, skip, limit}`, order `id DESC`, and share the
  100/200 pagination rules. `provider_id` and `product_id` filters apply only
  when positive.
- `last_quoted_cost` is money. PostgreSQL reads `NUMERIC(12,2)` as text and
  writes/reads decimals; SQLite stores integer cents and converts with
  `decimal.New(cents, -2)`. JSON carries a number. `lead_time` is a nullable
  integer number of days.
- Missing rows map to `404 Supplier catalog not found`.

## Purchase orders

- Endpoints under `/api/v1/supply-chain/purchase-orders`: list, create, get,
  update, delete, item list/add/update/delete. All require an active user.
- The order response is `{id, provider_id, status, payment_status, order_date,
  total_amount, provider}`; `created_by` is never returned. `created_by_id`
  comes from the bearer token on create. `order_date` is set once at create and
  serialized as RFC 3339 UTC.
- `total_amount` is the sum of item subtotals, calculated by the service.
  Money uses `decimal.Decimal`; PostgreSQL stores `NUMERIC(12,2)` and SQLite
  stores integer cents. Statuses are lowercase in the API and uppercase in the
  database.
- Lists return `{items, has_next, skip, limit}`, order `id DESC`, and share the
  100/200 pagination rules. Filters: `provider_id`, `status`,
  `payment_status`, `min_date`, `max_date` (RFC 3339, inclusive, compared to
  `order_date`). Invalid dates or enums are `422`.
- Create requires an existing provider (`400 Provider with id X not found`),
  at least one item, one item per product, `quantity > 0`,
  `unit_cost >= 0`, and a supplier SKU of at most 100 characters. Missing
  products are `404 Product with id X not found`. Creating directly with
  `status: received` stores the order but does **not** apply stock or catalogs;
  only the update transition does (legacy behavior).
- Update is partial over `provider_id`, `status`, and `payment_status`.
  Explicit nulls are `422`. Transitioning from any non-`received` status to
  `received` locks the item products in ascending id order, increments stock by
  each item quantity, and upserts `supplier_catalogs` by `(product_id,
  provider_id)` with the item cost, the whole-day lead time since `order_date`,
  and the supplier SKU when non-empty. The transition is idempotent: an order
  already `received` changes neither stock nor catalogs.
- Deleting an order deletes its items then the order and never reverses
  received stock.
- Item list responses are ordered `id ASC` and include product name/SKU plus
  brand, material, and category names. Item mutations recalculate the order
  total. Duplicate products are `400`; a missing item is `404 Order item not
  found`. On received orders item mutations are allowed and do not adjust stock
  or catalogs (legacy behavior).

## Sales

- Endpoints under `/api/v1/sales`: list, create, stats, get, update, delete,
  item list/add/update/delete, payment list/create/delete, and client-level
  payment create. All require an active user.
- The response shape is `{id, total_amount, debt_amount, payment_status,
  delivery_status, amount_paid, client_id, created_at, updated_at,
  created_by_id, created_by, client}`. `created_by` is the full user without
  password fields; `client` is the full client or `null`. Sale responses do
  not include `payment_method`. `debt_amount` is
  `max(total_amount - amount_paid, 0)`.
- Lists support `user_id`, `client_id`, `no_client`, `start_date`, `end_date`,
  `payment_status`, `delivery_status`, and `payment_method`. Dates are RFC 3339
  and inclusive; invalid dates or enum values are `422`. Ordering is `id DESC`
  with the shared 100/200 pagination rules.
- Create requires at least one item, one item per product, `quantity > 0`,
  and `0 <= delivered_quantity <= quantity`. `unit_price` defaults to the
  product price. Totals, payment status, and delivery status are derived by
  the service. `created_by_id` comes from the authenticated user, never from
  the body. A sale with an outstanding balance requires `client_id`; fully paid
  anonymous sales remain valid.
- `sales.amount_paid` is a cache of the sum of `sale_payments.amount`.
  `POST /sales` with `amount_paid > 0` requires a method and inserts exactly
  one opening payment for the authenticated user; zero paid inserts none.
  `amount_paid > total_amount` is `422`.
- Update accepts only `client_id`; omitted stays unchanged and `null` clears,
  unless the sale still carries debt (`422`). `amount_paid` and
  `payment_method` are rejected as unknown fields (`422`).
- Item mutations adjust stock by the quantity delta, recalculate the sale,
  and set `updated_at`. Deleting a sale or one of its items restores stock.
  Deleting a sale cascades its payment rows. An empty sale has total `0` and
  both statuses `pending`.
- Payment list is ordered `id DESC` with the shared pagination rules. Each
  payment returns `{id, sale_id, amount, payment_method, created_at,
  created_by_id, created_by}`; `created_by` is the full user without password
  fields. Legacy backfilled rows may have a null method or creator.
- Creating or deleting a payment locks the client then the sale row, validates
  against the remaining debt, recomputes `amount_paid` from
  `SUM(sale_payments.amount)` (never `+=`), then re-derives status, debt, the
  client cache, and `updated_at` in the same transaction. A paid sale or an
  amount above the debt is `422`.
- `POST /sales/clients/{id}/payments` locks the client, allocates the amount
  over that client's debtor sales ordered `created_at ASC, id ASC`, inserts one
  payment row per affected sale with the chosen method and authenticated user,
  recomputes each sale, and recomputes the client cache once. The response
  carries `client_id`, `amount`, `remaining_debt`, and `allocations`. The
  amount is validated against the in-transaction ledger sum, not the cache; it
  must be a plain JSON number with at most two decimals inside
  `(0, 9999999999.99]`, and a batch that would touch more than 1,000 open
  sales is rejected before writing. Unknown client is `404`; no debt,
  overpayment, zero/negative amount, more than two decimals, exponent/NaN/Inf,
  amount above the ceiling, too many affected sales, or invalid method is
  `422`; nothing is written on error.
- `GET /sales?payment_method=` uses `EXISTS` against `sale_payments`, so it
  matches sales paid with that method at least once.
- Status values are lowercase in the API and uppercase in the database. The
  adapter maps explicitly; see [DOMAIN.md](DOMAIN.md#database-representation).
- Item list responses are ordered `id ASC` and include product name/SKU plus
  brand, material, and category names.

## Dashboard

- Endpoints under `/api/v1/dashboard`: summary, sales-trend, top-products, and
  order-status-distribution. All require an active user and are read-only.
- Aggregates run as raw SQL in `gormstore`. PostgreSQL sums `NUMERIC(12,2)` as
  text decimals; SQLite sums integer cents and converts once in the adapter.
  No `float64` money path.
- Summary: `inventory_value` is `SUM(price * stock_quantity)` at sale price;
  `total_sales_today` sums sales created since Bogota midnight;
  `pending_payment_sales` is `SUM(total_amount - amount_paid)` for unpaid or
  partially paid sales; `pending_orders` counts pending purchase orders;
  `delivered_orders` counts delivered sales, not purchase orders.
- Sales trend returns exactly seven Bogota calendar days ending today, oldest
  first, zero-filled. Sales bucket by `created_at`, orders by `order_date`.
- Top products covers the last seven days from now, groups by product, sums
  `delivered_quantity` for quantity, sums `subtotal` for amount, and orders by
  the amount descending. Brand, category, and material names are nullable.
  `limit` defaults to `10` and must be `1..100`, otherwise `422`.
- Order-status distribution groups purchase orders by status, lowercases the
  status, orders by count descending, and returns the percentage of the total
  (two decimals) with `0` when there are no orders.
- Bogota day bucketing uses a fixed `-05:00` offset because Colombia has no
  DST, so the runtime does not need tzdata and never uses the host timezone.
  PostgreSQL uses `AT TIME ZONE 'America/Bogota'`; SQLite uses
  `date(created_at, '-5 hours')` on the stored UTC text.

## Transactions and stock

Services own mutations through `outbound.UnitOfWork.WithinTx`, which provides
`outbound.TxRepos` (`Sales`, `Products`, `Clients`, `Stock`, `Orders`,
`Catalogs`, `Providers`). Repositories do not start nested transactions.

Sale creation:

```text
BEGIN
  validate client and products
  ClientDebtRepository.Lock(client id)                -- when a client is set
  StockRepository.LockForUpdate(product ids sorted ascending)
  verify stock for every item
  StockRepository.ApplyDelta(-quantity)
  insert sale
  insert items
  recompute client debt
COMMIT
```

Any failure rolls back the whole sale. If `amount_paid > 0`, the same
transaction inserts its opening payment and recomputes the cached total.

Sale and payment mutations lock the client row before the sale row:

```text
BEGIN
  ClientDebtRepository.Lock(client id)                -- SQLite: write txn
  SELECT ... FROM sales WHERE id = ? FOR UPDATE
  read amount_paid / validate remaining debt
  insert or delete sale_payments row
  amount_paid = SUM(sale_payments.amount) for this sale
  update payment_status, debt_amount, updated_at
  recompute client debt from source
COMMIT
```

The lock is taken by `Update`, item add/update/delete, recalculation, sale
delete, and payment insert/delete. A stale item recalculation therefore cannot
overwrite a committed payment. Item mutations use the same boundary, adjusting
stock by the delta and recalculating the sale. Deleting a sale locks the
product ids, restores each item quantity, deletes the items, deletes the sale
(payments cascade), then recomputes the client debt. PostgreSQL serializes
competing writers with `SELECT ... FOR UPDATE`; SQLite forces a write
transaction before the read. Client reassignment locks both client rows in
ascending id order, then the sale. The same stock rule applies to order
receiving.

Order receiving:

```text
BEGIN
  SELECT ... FROM purchase_orders WHERE id = ? FOR UPDATE
  reread status (another writer may have committed)
  if old status != received and new status == received:
    load items
    StockRepository.LockForUpdate(product ids sorted ascending)
    verify every product exists
    for each item:
      increment product stock
      upsert supplier catalog (cost, lead time, supplier SKU)
  update order status/provider/payment
COMMIT
```

The order row lock serializes concurrent receiving transitions, so the stock
increment happens exactly once. Status and totals are derived on the server.
Never trust client totals.

## Configuration

Load from environment, then the OS config file, then defaults. See
[ARCHITECTURE.md](ARCHITECTURE.md). `DB_MODE` selects `auto` (default),
`postgres`, or `sqlite`. In `auto`, PostgreSQL is probed with a 3-second
timeout and SQLite is opened on failure. Validate the DSN at startup when it
is used. Generate and persist `SECRET_KEY` when missing.

Every value read through the config lookup is trimmed, so a blank value
(`DATABASE_URL="  "`) counts as unset and falls through to the next source
instead of becoming a broken DSN. `SQLITE_PATH` is the exception: a blank
value is rejected with an error, because an empty path resolves to the
working directory, which the SQLite open helper would chmod to `0700` and
turn into a stray database file.

`DATABASE_URL` defaults to the DSN in `internal/config/config.go`
(`defaultDatabaseURL`). That literal lives only in that file; every other
component reads the resolved value from the config.

The resolved database target is logged at startup. `database ready` carries
the backend plus the PostgreSQL host, port, and database name, and the `auto`
fallback warning carries the same target plus the hint `set DB_MODE=postgres
to fail hard`. Targets are parsed from the DSN with `net/url` and never
include the user or the password; a DSN that is not a URI logs the generic
`target=unknown` marker.

In `postgres` mode the connection attempt is bounded by a 10-second ping
timeout, so a black-holed host fails fast instead of hanging until the OS
TCP timeout. The `auto` probe keeps its 3-second timeout.

## Migrations

- SQL files live under `migrations/postgres/` and `migrations/sqlite/`, each
  embedded with `embed.FS`.
- Naming: `00001_enums.sql`, `00002_catalog.sql`, `00003_identity.sql`,
  `00004_products.sql`, `00005_purchases.sql`, `00006_sales.sql`,
  `00007_sale_payments.sql`, `00008_client_debt.sql`.
- PostgreSQL integer ids use `GENERATED BY DEFAULT AS IDENTITY`. The legacy
  per-table `id` and name indexes (`ix_*_id`, `ix_products_name`,
  `ix_providers_name`, `ix_users_full_name`) are gone.
- The initial schema is split by domain across versions 1–6; version 7 adds
  the sale payment ledger, backfills one opening payment per paid sale, and
  drops `sales.payment_method`. SQLite rebuilds `sales` to remove the column
  and its table CHECK. Version 8 adds `clients.debt_amount` with a
  non-negative CHECK and backfills it from `sales`; SQLite stores integer
  cents. Future changes get one new file each.
- The split changed the baseline: databases created before the split must be
  recreated. A goose row for version `1` alone is not a valid baseline for the
  split set.
- SQLite uses goose dialect `sqlite3`. Version 1 creates the auth tables
  (`users`, `refresh_tokens`); versions 2–6 create the full business schema
  split by domain: catalog, identity, products, purchases, sales; version 7
  adds `sale_payments` and rebuilds `sales` inside one atomic migration.
- The catalog, product, client, provider, supplier catalog, purchase order,
  and sales repositories and handlers are implemented. Dashboard aggregates
  are implemented and contract-tested on both backends. Do not translate
  business tables piecemeal; parity tests come with the adapters.
- Every migration is forward-only unless a rollback is safe and tested.
- Run at startup when `AUTO_MIGRATE=true`; otherwise expose an explicit
  migrate command.
- The runner is goose on the `*sql.DB` returned by `OpenPostgresGORM` or
  `OpenSQLiteGORM`. Never use GORM AutoMigrate, and never edit a migration that
  has shipped.

## Errors

Define a small set of sentinel errors or a typed `AppError`:

```go
type AppError struct {
    Status int
    Code   string
    Detail string
}
```

Map them in one response helper to
`{"detail": "...", "code": "..."}`. Unknown errors return `500` with a
generic detail; log the cause with the request ID.

## Logging

- `slog` with JSON in production and text in development.
- Include request ID, method, path, status, and duration.
- Exclude query strings that may contain sensitive values.
- Never log tokens, passwords, or customer PII.

## Commands

```bash
go test ./...
go test -race ./...
go vet ./...
gofmt -w .
make dev
make build
```

### Seed commands

`go run ./cmd/seed` and `go run ./cmd/seed-catalogs` print the resolved
backend and target before writing, so seeding the wrong database is
self-evident:

```text
seeding backend=postgres host=127.0.0.1:5432 database=inventory_desktop
seeding backend=sqlite path=/home/user/.config/inventory-desktop/data/inventory.sqlite
```

When the resolved backend is SQLite, both commands also print a warning that
they seed the local offline database; the app may still be running on
PostgreSQL. No credentials are printed.

## Boundaries

- Backend owns `main.go`, `app.go`, `internal/**`, `migrations/**`,
  `go.mod`, and `go.sum`.
- Do not edit `frontend/**`, `wails.json`, build scripts, or CI.
- Request schema changes through `database`; implement them here.
- Report contract changes to `frontend` and `qa`.
