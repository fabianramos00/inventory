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

```text
main.go                 Wails entry and server lifecycle
app.go                  Wails bindings (GetAPIBaseURL)
internal/
  application/
    auth.go             auth use cases
    catalog.go          catalog validation and use cases
    product.go          product validation, SKU generation, and use cases
    client.go           client validation and use cases
    sale.go             sale totals, statuses, stock, and item mutations
    inventory.go
    supply_chain.go
    dashboard.go
    users.go
  ports/
    inbound/
      auth.go           AuthUseCase, LoginResult, TokenPair
      catalog.go        CatalogUseCase, list and create/update inputs
      product.go        ProductUseCase, filters, and partial-update inputs
      client.go         ClientUseCase, list filter, and partial-update inputs
      sale.go           SaleUseCase, filters, and create/update/item inputs
      users.go          UserUseCase, UserList, create/update inputs
    outbound/
      catalog.go        CatalogRepository and ProductRepository interfaces
      client.go         ClientRepository interface
      sale.go           SaleRepository interface
      stock.go          StockRepository interface
      uow.go            UnitOfWork and TxRepos
      users.go          UserRepository interface
      refresh_tokens.go RefreshTokenRepository interface
      password_hasher.go
      access_token_issuer.go
      refresh_token_service.go
  domain/
    catalog.go          Category, Brand, Material, MeasurementUnit, Product
    client.go           Client
    user.go             User, RefreshToken (no JSON tags)
    errors.go           domain sentinel errors (not found, conflict, validation, auth)
  adapters/
    inbound/
      httpapi/
        router.go       route table
        middleware.go   auth, CORS, recovery, request ID, logging
        response.go     JSON and error helpers
        auth.go         auth handlers
        catalog.go      catalog CRUD handlers and request/response DTOs
        product.go      product CRUD, stats handlers, and money JSON DTOs
        client.go       client CRUD handlers and request/response DTOs
        sale.go         sale/item handlers, filters, and response DTOs
        user_dto.go     shared user response DTO and mapper
        users.go        user CRUD handlers and superuser guard
    outbound/
      persistence/
        gormstore/
          timestamp.go  fixed-width UTC Timestamp type
          catalog.go    catalog models and repositories
          product.go    product queries and money normalization
          client.go     client queries and timestamp round-trip
          sale.go       sale/item queries, status mapping, money conversion
          stock.go      sorted locking and conditional stock deltas
          uow.go        transaction boundary and tx-bound stores
          user_model.go
          refresh_token_model.go
          users.go      UserRepository
          refresh_tokens.go RefreshTokenRepository
      security/
        jwt.go          AccessTokenIssuer
        password.go     PasswordHasher
        refresh.go      RefreshTokenService
  config/
    config.go
  db/
    postgres.go         OpenPostgresGORM, MigratePostgres
    sqlite.go           OpenSQLiteGORM, MigrateSQLite
  bootstrap/
    database.go         DB_MODE selection and store wiring
migrations/
  postgres/
  sqlite/
```

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
- Item shape: `{id, name, identity_card, email, phone, created_at,
  updated_at}`. Timestamps are RFC 3339 UTC. `created_at` is set on create and
  `updated_at` stays `null` until a field actually changes.
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
  get. There are no create/update/delete routes in this phase; catalog rows are
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

## Sales

- Endpoints under `/api/v1/sales`: list, create, stats, get, update, delete,
  item list/add/update/delete. All require an active user.
- The response shape is `{id, total_amount, debt_amount, payment_status,
  delivery_status, payment_method, amount_paid, client_id, created_at,
  updated_at, created_by_id, created_by, client}`. `created_by` is the full
  user without password fields; `client` is the full client or `null`.
  `debt_amount` is `max(total_amount - amount_paid, 0)`.
- Lists support `user_id`, `client_id`, `no_client`, `start_date`, `end_date`,
  `payment_status`, `delivery_status`, and `payment_method`. Dates are RFC 3339
  and inclusive; invalid dates or enum values are `422`. Ordering is `id DESC`
  with the shared 100/200 pagination rules.
- Create requires at least one item, one item per product, `quantity > 0`,
  and `0 <= delivered_quantity <= quantity`. `unit_price` defaults to the
  product price. Totals, payment status, and delivery status are derived by
  the service. `created_by_id` comes from the authenticated user, never from
  the body.
- Update is partial: omitted fields stay unchanged, `payment_method` and
  `client_id` accept `null` to clear, and `amount_paid` must be present and
  non-negative when supplied. Overpayment is accepted and marks the sale
  `paid`, preserving legacy behavior.
- Item mutations adjust stock by the quantity delta, recalculate the sale,
  and set `updated_at`. Deleting a sale or one of its items restores stock.
  An empty sale has total `0` and both statuses `pending`.
- Status values are lowercase in the API and uppercase in the database. The
  adapter maps explicitly; see [DOMAIN.md](DOMAIN.md#database-representation).
- Item list responses are ordered `id ASC` and include product name/SKU plus
  brand, material, and category names.

## Transactions and stock

Services own mutations through `outbound.UnitOfWork.WithinTx`, which provides
`outbound.TxRepos` (`Sales`, `Products`, `Clients`, `Stock`). Repositories do
not start nested transactions.

Sale creation:

```text
BEGIN
  validate client and products
  StockRepository.LockForUpdate(product ids sorted ascending)
  verify stock for every item
  StockRepository.ApplyDelta(-quantity)
  insert sale
  insert items
COMMIT
```

Any failure rolls back the whole sale. Item mutations use the same boundary,
adjusting stock by the delta, recalculating the sale, and setting
`updated_at`. Deleting a sale locks the product ids, restores each item
quantity, deletes the items, then deletes the sale. PostgreSQL serializes
competing stock writers with `SELECT ... FOR UPDATE`; SQLite uses its single
connection. The same rule applies to order receiving.

Order receiving:

```text
BEGIN
  lock order
  if old status != received and new status == received:
    for each item:
      increment product stock
      upsert supplier catalog (cost, lead time, supplier SKU)
COMMIT
```

Status and totals are derived on the server. Never trust client totals.

## Configuration

Load from environment, then the OS config file, then defaults. See
[ARCHITECTURE.md](ARCHITECTURE.md). `DB_MODE` selects `auto` (default),
`postgres`, or `sqlite`. In `auto`, PostgreSQL is probed with a 3-second
timeout and SQLite is opened on failure. Validate the DSN at startup when it
is used. Generate and persist `SECRET_KEY` when missing.

## Migrations

- SQL files live under `migrations/postgres/` and `migrations/sqlite/`, each
  embedded with `embed.FS`.
- Naming: `00001_enums.sql`, `00002_catalog.sql`, `00003_identity.sql`,
  `00004_products.sql`, `00005_purchases.sql`, `00006_sales.sql`.
- The initial schema is split by domain across versions 1–6; future changes
  get one new file each (`00007_add_index.sql`).
- The split changed the baseline: databases created before the split must be
  recreated. A goose row for version `1` alone is not a valid baseline for the
  split set.
- SQLite uses goose dialect `sqlite3`. Version 1 creates the auth tables
  (`users`, `refresh_tokens`); versions 2–6 create the Phase 3 business schema
  split by domain: catalog, identity, products, purchases, sales.
- The catalog, product, client, provider, supplier catalog, and sales
  repositories and handlers are implemented. Purchase orders and dashboard
  remain pending. Do not translate business tables piecemeal; parity tests come
  with the adapters.
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
go vet ./...
gofmt -w .
go run .            # dev once the Wails scaffold exists
wails build
```

## Boundaries

- Backend owns `main.go`, `app.go`, `internal/**`, `migrations/**`,
  `go.mod`, and `go.sum`.
- Do not edit `frontend/**`, `wails.json`, build scripts, or CI.
- Request schema changes through `database`; implement them here.
- Report contract changes to `frontend` and `qa`.
