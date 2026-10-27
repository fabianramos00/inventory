# Phase 2 SQLite Review — Auth Schema, Adapter Parity, Migrations, Fallback, Sync Readiness

**Status:** Review only. No code, SQL, migration, or configuration changed.
**Reviewer scope:** `database` (Markdown only).
**Method:** Source inspection. No shell commands were run.
**Inputs:** `AGENTS.md`, `docs/DOMAIN.md`, `docs/ARCHITECTURE.md`,
`docs/BACKEND.md`, `docs/SYNC.md`, `docs/MIGRATION.md`, `docs/TESTING.md`,
`migrations/postgres/00001_initial_schema.sql`,
`migrations/sqlite/00001_auth.sql`,
`internal/adapters/persistence/{postgres,sqlite}`, `internal/db`,
`internal/bootstrap`, `internal/auth`, `internal/service`, `internal/ports`,
`cmd/seed`, `go.mod`, and the existing
`docs/work/database/phase-2-schema-review.md` and
`docs/work/security/phase-2-sqlite-security-review.md`.

## Verdict

The SQLite auth schema is a faithful, deliberately stricter translation of the
PostgreSQL `users` and `refresh_tokens` tables. I found no column, type,
uniqueness, or cascade mismatch that changes Phase 2 behavior. The adapter
contract for `domain.ErrNotFound` and refresh rotation is in parity, and
SQLite `RETURNING` is already exercised by the contract suite.

There are no **Critical** defects in the current auth schema. The blocking
items are Phase 3 gates and one current auth bug:

1. **Current bug:** the SQLite refresh-token expiry read is not round-trip
   safe for timestamps whose nanosecond field ends in zero (DB-3). This is
   already noted as a reported defect in `docs/TESTING.md:129-134`.
2. **Phase 3 gates:** no transaction propagation across repositories (DB-4),
   no `FOR UPDATE` equivalent (DB-5), and no exact money representation
   decision for SQLite (DB-2).
3. **Migration gate:** a populated legacy PostgreSQL database still has no
   goose baseline (DB-6, previously F5).
4. **Auth boundary:** a token minted on one dataset can authenticate as a
   different user on the other dataset (DB-1, previously security SQL-4).

The remaining findings are parity/hardening items and are separated into
must-fix and measure-first tables in section 6.

---

## 1. SQLite auth schema vs PostgreSQL auth schema

### `users`

| Column | PostgreSQL (`00001_initial_schema.sql:555-562`) | SQLite (`00001_auth.sql:4-11`) | Parity |
|---|---|---|---|
| `id` | `integer NOT NULL`, PK, `DEFAULT nextval('users_id_seq')`; redundant `ix_users_id` (`:1104-1107`) | `INTEGER PRIMARY KEY AUTOINCREMENT` | Equivalent for the app's `int64`. `AUTOINCREMENT` prevents rowid reuse and creates `sqlite_sequence`; PG sequences also do not reuse after delete. No behavior change. |
| `full_name` | `character varying` NULL; `ix_users_full_name` (`:1097-1100`) | `TEXT` NULL; **no index** | Same nullability. Missing index only matters for the legacy `ILIKE '%term%'` user search, which a btree cannot serve on PG either. See DB-14. |
| `email` | `character varying NOT NULL`; unique index `ix_users_email` (`:1090-1093`) | `TEXT NOT NULL UNIQUE` | Same case-sensitive uniqueness. SQLite creates `sqlite_autoindex_users_1`; the index name is not referenced anywhere. |
| `hashed_password` | `character varying NOT NULL` | `TEXT NOT NULL` | Same. No length check in either. Bcrypt is 60 chars. |
| `is_active` | `boolean` NULL, `DEFAULT true` (`:560`) | `INTEGER NOT NULL DEFAULT 1` | SQLite is stricter. The PG adapter uses `COALESCE(is_active, true)` (`postgres/users.go:15`); SQLite scans an `int64` (`sqlite/users.go:74-83`). A `NULL` PG row is treated as **active**, while legacy `not user.is_active` treats `NULL` as **inactive**. See DB-15. |
| `is_superuser` | `boolean` NULL, `DEFAULT false` (`:561`) | `INTEGER NOT NULL DEFAULT 0` | Same as above. |

Neither schema has `created_at`/`updated_at`. That is legacy parity, but it
matters for future sync (DB-S3).

### `refresh_tokens`

| Column | PostgreSQL (`00001_initial_schema.sql:412-418`) | SQLite (`00001_auth.sql:13-19`) | Parity |
|---|---|---|---|
| `id` | `integer NOT NULL`, PK, sequence; redundant `ix_refresh_tokens_id` (`:1013-1016`) | `INTEGER PRIMARY KEY AUTOINCREMENT` | Equivalent; see DB-16. |
| `user_id` | `integer NOT NULL`; FK `ON DELETE CASCADE` (`:1199-1203`); **no index** | `INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE`; `ix_refresh_tokens_user_id` (`:21`) | Cascade semantics match. Index parity is inverted: SQLite has the index PG lacks (prior F7). See DB-14. |
| `token_hash` | `character varying NOT NULL`; unique index `ix_refresh_tokens_token_hash` (`:1020-1023`) | `TEXT NOT NULL UNIQUE` | Same. Go stores SHA-256 hex, 64 chars (`internal/auth/refresh.go:18-21`), matching legacy. No length check in either. |
| `expires_at` | `timestamp with time zone NOT NULL` | `DATETIME NOT NULL` | **Semantic difference.** SQLite stores a fixed-width UTC string; the declared type has NUMERIC affinity. See section 1.1 and DB-3. |
| `revoked` | `boolean` NULL, `DEFAULT false` (`:417`) | `INTEGER NOT NULL DEFAULT 0` | SQLite is stricter. Both queries use `revoked = false` / `revoked = 0`; PG `NULL` rows are excluded in both legacy and Go behavior. See DB-15. |

No `created_at` in either table. No purge path in either backend.

### 1.1 Timestamp storage

- Go writes `2006-01-02T15:04:05.000000000Z` in UTC
  (`sqlite/values.go:8-12`). The value is 30 chars, fixed width, with a
  literal `Z`.
- The SQLite column is declared `DATETIME`. That gives it **NUMERIC
  affinity**, but the value is not a numeric literal, so it is stored as
  `TEXT`. `typeof(expires_at)` should return `text`.
- The expiry predicate compares stored TEXT with a bound TEXT parameter of
  the same shape. Because the parameter is non-numeric, NUMERIC affinity does
  not convert it, and the comparison is lexicographic. With a fixed-width
  format, lexicographic order equals chronological order.
- **Do not store numeric timestamp strings.** If a value or parameter is
  numeric-looking (for example an epoch string), SQLite applies NUMERIC
  affinity, converts it, and compares TEXT with INTEGER. SQLite orders every
  TEXT value above every INTEGER value, so `expires_at > <epoch>` would be
  true for all rows and expired tokens would be accepted.
- The driver behavior is the source of DB-3. `modernc.org/sqlite` returns a
  `time.Time` for TEXT in a `DATE`/`DATETIME`/`TIMESTAMP` column when the text
  matches one of its parse formats
  (`modernc.org/sqlite@v1.60.1/rows.go:195-201`,
  `conn.go:170-201`). Scanning that into a `string` re-formats it with
  `time.RFC3339Nano`, which **trims trailing zeros**. `parseTimestamp` then
  requires exactly nine fractional digits (`values.go:14-20`), so the parse
  fails.
- Precision: SQLite keeps nanoseconds; PG keeps microseconds
  (`timestamp with time zone`). The contract test already tolerates this
  (`internal/adapters/persistence/contract_test.go:247-254`). Compare
  instants, not strings.

### 1.2 Booleans, uniqueness, cascade, indexes

- Booleans are `0`/`1` in SQLite, written by `boolToInt` and read with
  `!= 0` (`sqlite/values.go:22-27`, `sqlite/users.go:82-83`,
  `sqlite/refresh_tokens.go:57`).
- `PRAGMA foreign_keys=ON` is in the DSN (`db/sqlite.go:22`). Without it, the
  `ON DELETE CASCADE` would silently not run. The app always opens through
  `OpenSQLite`; tests that open the file directly must set it too.
- Unique constraints: `users.email` and `refresh_tokens.token_hash` are
  unique in both. Neither has a length CHECK; not a current problem.
- Indexes:
  - Both: unique email, unique token hash.
  - PG only: `users.full_name`, and redundant `ix_*_id` btree indexes on
    every table (prior F14).
  - SQLite only: `ix_refresh_tokens_user_id` (`00001_auth.sql:21`). PG needs
    the same index if user deletion is exposed (prior F7).
- Cascade behavior for the tables that exist today matches: refresh tokens
  cascade on user delete; there is no product/price-history/sale-item table
  in SQLite yet.

---

## 2. Adapter parity risks

| Area | Finding | Verdict |
|---|---|---|
| Placeholders and SQL shape | `$1` in PG, `?` in SQLite; all values parameterized. `userColumns` is static concatenation. | Parity. |
| Not-found mapping | PG maps `pgx.ErrNoRows` to `domain.ErrNotFound` (`postgres/users.go:69-71`, `postgres/refresh_tokens.go:46-48,70-72`). SQLite maps `sql.ErrNoRows` the same way (`sqlite/users.go:76-78`, `sqlite/refresh_tokens.go:46-48,75-77`). `Update` uses `RowsAffected` in both. | Parity. |
| Unique-violation mapping | Neither adapter maps unique violations. PG raises SQLSTATE `23505`; SQLite raises constraint code `2067`/`1555`. The service only knows `ErrNotFound`. | **Gap** (DB-9). Only matters when user creation is exposed; currently only the seed writes users. |
| Transactions | `Rotate` uses a transaction in both adapters (`postgres/refresh_tokens.go:55-89`, `sqlite/refresh_tokens.go:61-94`). The service cannot wrap multiple repo calls in one transaction because the ports expose no transaction handle. | **Gap** (DB-4). On PG this silently splits a business transaction; on SQLite it can deadlock. |
| Timestamp round-trip | Writes are canonical in SQLite; reads are normalized by the driver and can fail the 9-digit parser. PG returns microseconds. | **Defect** (DB-3). |
| `RETURNING` | PG uses `RETURNING` in `Create` and `Rotate`; SQLite uses `LastInsertId` in `Create` and `UPDATE ... RETURNING` in `Rotate`. The contract suite runs `Rotate` on SQLite (`contract_test.go:191-210`) and passes. modernc v1.60.1 bundles SQLite 3.53.4, well above 3.35. | Supported; no action. Keep an integration test on the release build/OS. |
| IDs | SQLite uses `LastInsertId`; PG uses `RETURNING id`. Both set a positive `int64`. | Parity. |
| Boolean/NULL | PG booleans are nullable with server defaults; SQLite is `NOT NULL` with `0`/`1`. PG scans into `bool`, SQLite into `int64`. PG `COALESCE(is_active, true)` also turns a `NULL` row into an active user, while legacy treats `NULL` as inactive (`../inventory-api/app/services/users.py:47`). | **Parity gap** for legacy `NULL` rows (DB-15). |
| `ILIKE` and search | SQLite has no `ILIKE`; `LIKE` folds ASCII only, and `lower()` is ASCII-only. PG `ILIKE` and `pg_trgm` are not portable. | Future divergence (DB-21). Document and test. |
| `SELECT ... FOR UPDATE` | SQLite has no row locks. `SetMaxOpenConns(1)` serializes writes inside one process, but not across processes, and it is not a substitute for row locking. | **Gap** (DB-5). |
| `now()` / server time | Do not use SQLite `CURRENT_TIMESTAMP`: it produces `YYYY-MM-DD HH:MM:SS`, which breaks the canonical `T...Z` comparison and the 9-digit parser. Pass UTC values from Go. | Rule for Phase 3. |
| `LIMIT`/sort stability | Not used yet. PG and SQLite can return different tie order unless the query uses a total order. | Future rule (DB-23). |
| Dashboard timezone | SQLite has no `AT TIME ZONE` or `date_trunc`. Bogotá is fixed UTC-5, so `datetime(ts, '-5 hours')` or Go-side grouping works, but only with canonical timestamps. | Future rule (DB-24). |

### 2.1 The one-connection WAL tradeoff

`OpenSQLite` sets `SetMaxOpenConns(1)` (`db/sqlite.go:27-28`) with a `ponytail:`
comment. This is the correct lazy choice for Phase 2 auth:

- It avoids `SQLITE_BUSY` inside one process.
- It makes migrations and auth writes trivially serialized.
- WAL still helps other processes read while this process writes and improves
  crash behavior, but the single connection removes reader/writer concurrency
  **inside this process**, so WAL's main benefit is not realized here.

The Phase 3 risk is not throughput, it is **nested queries**:

1. With one connection, if any code holds an open `*sql.Rows` and issues a
   second query on the same `*sql.DB`, `database/sql` waits for the only
   connection until the context is canceled. A loop that queries relations
   inside `rows.Next` is an immediate deadlock. This is exactly the N+1 shape
   Phase 3 sale/order lists tend to take.
2. If a service starts `BeginTx` and then calls a repository method that uses
   `r.db` instead of the `*sql.Tx`, it waits for the connection already held by
   the transaction. The same mistake on PG does not deadlock; it runs outside
   the transaction, which is worse because it silently loses atomicity.

Keep one connection until Phase 3, but design the transaction boundary first
(DB-4). Raise the pool only after measuring, and when you do, add
`_txlock=immediate` (or issue `BEGIN IMMEDIATE` for write transactions) and
keep the 5-second busy timeout. A separate read-only pool is an option, not a
default.

---

## 3. Migration safety

### 3.1 Providers and version numbering

- `MigratePostgres` uses `goose.DialectPostgres` with the embedded
  `migrations/postgres` FS (`internal/db/postgres.go:31-45`).
- `MigrateSQLite` uses `goose.DialectSQLite3` with the embedded
  `migrations/sqlite` FS (`internal/db/sqlite.go:40-49`).
- Each database has its own goose version table. The two dialects cannot
  contaminate each other unless an operator points both at the same database,
  which the configuration does not allow.
- Both dialects currently use version `1`. That is fine because versions are
  per dialect/database, but future migrations must never reuse a version
  within a dialect and must be added to each dialect deliberately. There is no
  cross-dialect version map; add one to `docs/MIGRATION.md` when Phase 3 starts.

### 3.2 Fresh database

- PostgreSQL: `00001` creates enums, tables, indexes, and FKs from an empty
  database.
- SQLite: `00001_auth.sql` creates the two auth tables on an empty file.
- Both are covered by the contract/bootstrap tests.

### 3.3 Existing database

- **SQLite:** a database already at goose version 1 is fine. A database
  created outside goose (manual `CREATE TABLE`) has no `goose_db_version` row
  and will fail on `00001`. Document that the file must be created by the app.
- **PostgreSQL:** this is the unresolved baseline problem. The migration is
  version 1 "so existing databases do not re-run it" (`docs/BACKEND.md:211-212`),
  but goose reads `goose_db_version`, not `alembic_version`. A populated legacy
  database has no goose row, so `provider.Up` attempts `00001` and fails on the
  first existing object; startup aborts. Keeping the file at version 1 is not a
  baseline. See DB-6 and prior F5.

### 3.4 Rollback and retry

- `00001_auth.sql` has a `Down` that drops `refresh_tokens` then `users`
  (`:23-25`). It is destructive: it deletes local users and tokens. PG `00001`
  has no `Down` at all. This is asymmetric and conflicts with the forward-only
  policy in `docs/BACKEND.md:217`. Automatic startup uses `provider.Up`, so the
  Down is not normally reached, but a developer or operator running goose down
  would wipe the offline identity set.
- Goose normally wraps a migration in a transaction unless the file says
  `NO TRANSACTION`; SQLite DDL is transactional. A failed `up` should therefore
  roll back. Verify this with a forced failure; do not rely on the assumption.
- Once a migration has shipped, it must not be edited. Adding `IF NOT EXISTS`
  to an already-deployed `CREATE TABLE` does not help existing databases and
  breaks the “shipped files are immutable” rule.
- A retry of a failed migration is safe only because of the transaction
  wrapper. A manual re-run of the raw SQL is not idempotent.

### 3.5 `AUTO_MIGRATE=false`

`bootstrap.openSQLite` skips `MigrateSQLite` when `AUTO_MIGRATE=false`
(`internal/bootstrap/database.go:79-84`), and there is no explicit migrate
command in `cmd/`. A fresh SQLite file then has no tables, and login fails with
a raw “no such table” error. PG has the same problem. Either require
`AUTO_MIGRATE=true` for first run or add a `migrate` command as
`docs/BACKEND.md:218-219` already suggests.

### 3.6 File permissions

The migration files are embedded and read-only. The permission issue is the
database file and its WAL sidecars:

- `os.MkdirAll(dir, 0o700)` only affects new directories; an existing `data/`
  directory is not tightened (`db/sqlite.go:18`).
- `os.Chmod(path, 0o600)` runs after `Ping` (`db/sqlite.go:33`), so the file is
  created under the process umask first. If `Ping` fails, the chmod is never
  reached.
- WAL mode creates `inventory.sqlite-wal` and `inventory.sqlite-shm`; the code
  never chmods or checks them. With a `0700` directory they are protected by
  traversal; with a pre-existing `0755` directory they are not.
- On Windows, `os.Chmod` only toggles the read-only bit; it does not set an
  ACL. The `0600` claim is Unix-only.
- The existing permission test checks only the directory and the main file
  (`internal/bootstrap/database_test.go:95-124`), not sidecars or a
  pre-existing broad directory.

This overlaps security SQL-2/SQL-7; the DB-side fix is in DB-12.

### 3.7 Concurrent startup

Two app processes can open the same SQLite file. Goose has no cross-process
migration lock here: both can observe version 0 and both can attempt
`00001`; one fails with “table already exists” and aborts. WAL serializes
writers, but a second instance can also hit `SQLITE_BUSY` after the 5-second
busy timeout. Add an OS single-instance lock or a migration lock and a retry
policy, or explicitly document that two instances are unsupported.

---

## 4. Offline/fallback risks

### 4.1 Separate datasets

`DB_MODE=auto` probes PostgreSQL for 3 seconds, logs to stderr, and opens
SQLite on any failure (`internal/bootstrap/database.go:33-41`). The two
databases are completely independent:

- No data is copied or reconciled.
- A write performed offline is invisible to PostgreSQL after recovery.
- Both start `users.id` at 1, so the same integer identifies different people
  in each dataset.

This is documented in `docs/SYNC.md:7-15` and `docs/ARCHITECTURE.md:139-142`.
It is acceptable for Phase 2 auth and dangerous in Phase 3.

### 4.2 Auth divergence

- **Access tokens cross datasets.** The JWT has only `sub` and `exp`
  (`internal/auth/jwt.go:22-29`). `AuthenticateAccessToken` loads `sub` from
  whichever store is active (`internal/service/auth.go:130-145`). The
  `SECRET_KEY` is per installation and shared by both backends
  (`internal/config/config.go:75-89`). An access token minted against
  PostgreSQL for user 1 is therefore accepted by SQLite as SQLite user 1,
  which may be a different person and possibly a superuser. The reverse also
  holds. This is security SQL-4; it is latent only because there are no
  protected business routes yet. It becomes the first blocker for the first
  protected route (DB-1).
- **Refresh tokens do not cross datasets.** A PG refresh token is not present
  in SQLite, so refresh fails closed with 401 (`internal/service/auth.go:84-88`).
  The access token remains valid for up to 30 minutes, which is the crossover
  window.
- **Logout is local.** Revoking a token in one backend does not revoke the
  same token in the other; only the active DB is updated.
- **User provisioning is independent.** `cmd/seed` follows `DB_MODE` and its
  `auto` path (`cmd/seed/main.go:44-56`), so a seed run while PostgreSQL is
  down silently creates or updates the SQLite user set. The success line does
  not print the backend. This is security SQL-5.

### 4.3 Silent fallback

The only fallback signal is a `slog` warning on stderr
(`internal/bootstrap/database.go:40`), which a GUI release user does not see.
`Stores.Backend` is set (`:24,70,90`) but never surfaced. `GET /healthz`
returns only `{"status":"ok"}` (`internal/httpapi/router.go:15-17`). There is
no authenticated status field and no UI banner. A user can be writing to a
local dataset while believing they are on the shared PostgreSQL database.

This is security SQL-1. The DB-side requirement is to expose the active
backend and make the switch visible before Phase 3 business writes exist.

### 4.4 Data location

- Default: `<OS config dir>/inventory-desktop/data/inventory.sqlite`
  (`internal/config/config.go:201-207`).
- `SQLITE_PATH` overrides it (`:146-152`). There is no absolute-path, symlink,
  owner, or regular-file check in `OpenSQLite` (unlike the config loader at
  `config.go:210-235`).
- The DSN is built with `fmt.Sprintf("file:%s?...", path)` (`db/sqlite.go:22`).
  A path containing `?`, `#`, or `%` changes the URI; `?mode=memory` can force
  an in-memory database and silently lose data. This is security SQL-3.
- On Windows, `os.UserConfigDir()` is the roaming profile. A roaming profile
  can copy a WAL database and the `SECRET_KEY` to a server; WAL is not reliable
  on network filesystems. Security SQL-7 recommends `%LocalAppData%` and an
  explicit ACL.

### 4.5 Backup and restore

There is no backup or restore procedure in the repo.

- A clean backup must include the main file, `-wal`, and `-shm`, or be taken
  with SQLite's online backup API / `VACUUM INTO`. Copying only
  `inventory.sqlite` can lose committed transactions or produce an
  inconsistent copy.
- For PostgreSQL, use `pg_dump -Fc` and rehearse a restore to a scratch
  database before any destructive change.
- Add a restore rehearsal to the release checklist and document the exact
  files. A backup that has never been restored is not a backup.
- Do not propose or run a destructive migration without this step.

### 4.6 Not encrypted at rest

This is documented in `docs/SYNC.md:64-70` and `docs/ARCHITECTURE.md:125-126`.
Today the file holds bcrypt hashes and SHA-256 refresh-token hashes; both are
salted/high-entropy enough that casual reading is not credential theft. Phase
3 adds client PII, prices, costs, and stock. A stolen disk, backup, or roaming
copy then exposes the business. Decide SQLCipher/OS-level encryption or
explicitly accept the risk before Phase 3, and include backups in the
decision. This is security SQL-6.

---

## 5. Future sync constraints vs the current schema

I validated `docs/SYNC.md` against the actual columns. The document's
constraints are sound; the current schema cannot satisfy most of them. That is
expected for Phase 2, but several gaps must be decided **before** business
tables are added to SQLite, because retrofitting them means rewriting rows and
foreign keys.

| SYNC constraint | Current schema | Verdict |
|---|---|---|
| Stable global identity (`SYNC.md:19-21`) | All tables use local integer PKs. SQLite `AUTOINCREMENT` and PG sequences both start at 1 and will collide. `products.sku` is unique per database only and is generated randomly (`SKU-XXXXXXXX`); `clients.identity_card` is unique but optional. | **Not satisfied** (DB-S2). |
| Reliable timestamps and per-device sequence (`:22-23`) | Only `clients` (created/updated) and `sales` (created/updated) have timestamps; `updated_at` is nullable and application-managed, with no trigger. Products, categories, brands, materials, units, providers, catalogs, orders, and items have no `updated_at` at all. There is no monotonic sequence. | **Not satisfied** (DB-S3). |
| Tombstones (`:25-26`) | No `deleted_at` or tombstone table. Sale and order deletes are hard deletes. | **Not satisfied** (DB-S3). |
| Outbox in the same transaction (`:27-29`) | No outbox. | **Not satisfied** (DB-S3). |
| Idempotent apply by `sync_id` (`:30-31`) | No `sync_id` and no natural-key uniqueness for `sale_items`/`order_items`/`supplier_catalogs` (prior F8). Replaying a line item can duplicate it. | **Not satisfied** (DB-S5). |
| Stock is not last-write-wins (`:32-36`) | `products.stock_quantity` is a mutable integer with `CHECK (stock_quantity >= 0)` (`00001:307,314`). There is no movement ledger. Deletes restore stock for sales and do not reverse stock for received orders (DOMAIN.md:157-168). | **Not satisfied** (DB-S1). This is the highest-risk sync gap. |
| Money is decimal text (`:37-38`) | PG uses `NUMERIC(12,2)` and the Go adapter scans into `decimal`. SQLite has no decimal type and no business tables yet. | PG is fine; **the SQLite representation is undecided** (DB-2). |
| Per-entity conflict policy (`:39-42`) | No `version`, no reliable `updated_at`; sales/orders have no client-generated identity. | **Not satisfied** (DB-S3/S5). |
| Device registration (`:43-45`) | None. | Correctly out of scope. |
| Offline auth is local; never sync refresh tokens (`:46-48`) | SQLite has local `users` and `refresh_tokens`; refresh tokens are per database. User identity mapping across datasets is undefined. | Partially satisfied; identity mapping is a design gap (DB-1/S4). |
| Recommended additions (`:50-62`) | Not present and correctly not speculative. | No action now; add in a dedicated sync migration. |

### 5.1 What must change before business tables are added to SQLite

These are schema/design decisions, not code changes in this review:

1. **Money representation.** Choose integer cents or decimal TEXT. Never
   `NUMERIC`/`REAL` in SQLite. This is the one choice that is painful to
   change after data exists. See DB-2.
2. **Canonical timestamps.** Decide `TEXT` with a fixed 30-char format and a
   CHECK, or integer epoch. Use one helper for both backends. Do not use
   SQLite `DATETIME` for new columns and do not use `CURRENT_TIMESTAMP`.
   Fix the read path now (DB-3).
3. **Identity before `created_by_id` rows exist.** If sync is planned, add a
   stable user identity (`sync_id` UUIDv7 or a documented mapping) before
   sales/orders reference users. Otherwise every historical `created_by_id`
   has to be rewritten. See DB-S4.
4. **Natural keys or client-generated IDs for line items.** Add
   `UNIQUE (sale_id, product_id)`, `UNIQUE (order_id, product_id)`, and
   `UNIQUE (product_id, provider_id)` (after dedupe), or give those rows a
   `sync_id`. Without one, replay is not idempotent. See DB-S5.
5. **Stock ledger decision.** If sync is planned, create `stock_movements`
   from the first write and treat `products.stock_quantity` as a cache.
   Retrofitting a ledger requires backfilling an opening balance and every
   sale/order event. Decide now; implement in the sync migration.
6. **Enum representation.** Use the same uppercase DB labels in SQLite with
   `CHECK (... IN (...))`, or one shared mapping, so a single Go path works.
   `ORDER BY status` also differs: PG orders by enum declaration order,
   SQLite TEXT orders alphabetically. Use an explicit `CASE` or a sort column.
7. **Mirror PG constraints, not more.** Copy `NOT NULL`, `UNIQUE`, `CHECK`,
   and FK actions: `refresh_tokens` and `price_history` cascade; `sale_items`
   and `order_items` do **not** cascade (prior F21); `sales.client_id` does not
   cascade. Do not add a stricter `amount_paid <= total_amount` check: the
   legacy overpayment gap is deliberate (`DOMAIN.md:133-138`).
8. **Mirror length limits.** SQLite TEXT does not enforce `varchar(n)`. Add
   `CHECK (length(x) <= n)` for `abbreviation(5)`, `sku(100)`, `name(150)`,
   `size_value(20)`, `supplier_sku(100)`, `email` if a limit is chosen, etc.
9. **Do not add server defaults for the five Python-side defaults.**
   `products.stock_quantity`, `products.low_stock_threshold`,
   `sales.amount_paid`, `sale_items.delivered_quantity`, and
   `purchase_orders.total_amount` have no PG server default (`DOMAIN.md:124-127`).
   SQLite must also require explicit Go values, or the two backends will
   diverge when a caller forgets one.
10. **Transaction port and locking.** Before any multi-row business write,
    make transactions span repositories and define the SQLite stock strategy
    (DB-4/DB-5).

What can wait: `sync_outbox`, device/cursor tables, tombstones, encryption
implementation, conflict UI. Those are correctly deferred by `SYNC.md`.

---

## 6. Prioritized findings

### 6.1 Must fix before Phase 3

| ID | Severity | Location | Category | Issue | Impact | Fix | Verification |
|---|---|---|---|---|---|---|---|
| DB-1 | **High** (latent Critical) | `internal/auth/jwt.go:22-29,31-47`; `internal/service/auth.go:130-145`; `internal/bootstrap/database.go:33-41`; `migrations/sqlite/00001_auth.sql:5` | Auth / identity | The JWT carries only an integer `sub`; both datasets number users from 1 and share `SECRET_KEY`. A token minted on one backend authenticates as a different user on the other. | Identity confusion and possible privilege escalation for up to 30 minutes after a backend switch. Latent only because no protected routes exist. | Add and validate `iss`/`aud` bound to the installation/backend, use a per-backend key, or make `sub` a UUID. Force re-login and clear tokens on a backend switch. | Mint a PG token for user 1, switch to SQLite where user 1 is different, call a protected route: must be 401. Cross-ref security SQL-4. |
| DB-2 | **High** (Phase 3 gate) | `docs/SYNC.md:37-38`; no SQLite business schema; PG money columns at `00001:235-236,305-306,450-451,486-487` | Money | SQLite has no decimal type. A `NUMERIC(12,2)` column has NUMERIC affinity and stores non-integer values as REAL; even binding a decimal string converts it to REAL. SQL `SUM` then uses binary floats. | Financial drift that is hard to detect after the fact; violates the decimal invariant. | Store money in SQLite as integer cents (`INTEGER`) or exact decimal TEXT. Convert in the adapter; keep `shopspring/decimal` in Go. Never rely on SQLite NUMERIC/REAL for money. | Insert `0.10` and `0.20`, check `typeof(price)` and `SUM(price)` against decimal expectations; property test with many values. |
| DB-3 | **High** (current defect) | `migrations/sqlite/00001_auth.sql:17`; `internal/adapters/persistence/sqlite/refresh_tokens.go:36-56`; `internal/adapters/persistence/sqlite/values.go:8-20` | Timestamp round-trip | modernc returns `time.Time` for TEXT in a `DATETIME` column; scanning into `string` re-formats with `RFC3339Nano`, which trims trailing zero nanoseconds. `parseTimestamp` requires exactly nine fractional digits, so refresh fails. | `FindValid` returns a parse error → refresh endpoint 500. Roughly 10% of `time.Now()` values on a nanosecond clock end in zero; a coarser/Windows clock can fail for every token. The contract test hides this by using `.123456789` (`contract_test.go:148-154`). | One-line fix: parse with `time.RFC3339Nano` and `.UTC()` in `parseTimestamp` (accept variable fractional digits). Keep `formatTimestamp` fixed-width at 9 digits so lexicographic comparison stays correct. Add tests for 0 ns and `123000000` ns. Optionally scan into `time.Time`. | Unit test: create a token with zero or trailing-zero nanoseconds, then `FindValid`; currently fails, after fix passes. Run on Linux and Windows. Cross-ref `docs/TESTING.md:129-134`. |
| DB-4 | **High** (Phase 3 gate) | `internal/ports/*.go`; `internal/service/auth.go:48-145`; `internal/db/sqlite.go:27-28`; `docs/BACKEND.md:98-108,122-123` | Transactions / concurrency | Ports expose no transaction handle. A service cannot atomically call multiple repositories. On PG the second repo call escapes the transaction; on SQLite it waits for the single connection and can deadlock. Holding `Rows` while issuing another query has the same deadlock. | Lost atomicity for stock/sale/order operations, or request timeouts. Phase 3 cannot implement the documented transaction boundaries. | Add a minimal unit-of-work/`Querier` abstraction: repositories accept a transaction/querier, or `Store.WithTx(ctx, func(ctx, Repos) error)`. Never issue a query on `db` inside an open `Rows`; join relations instead of N+1. | Force an error after the first write and assert no partial sale/stock change on both backends. Open `Rows` and issue a second query with a 250 ms context; it must not block (or the ban must be enforced). |
| DB-5 | **High** (Phase 3 gate) | `docs/DOMAIN.md:157-171`; `docs/BACKEND.md:127`; no SQLite equivalent of `SELECT ... FOR UPDATE`; `00001:307,314,453-454` | Stock locking | SQLite has no row locks. `SetMaxOpenConns(1)` serializes one process but not two, and there is no backend-neutral stock mutation contract. PG also needs deterministic lock ordering (sort product IDs) to avoid deadlocks between concurrent sales. | Oversell or lost updates when two processes write; deadlocks in PG. Received-order stock can be applied twice if the status transition is not conditional. | Define a stock port: PG uses `SELECT ... FOR UPDATE` in sorted product order; SQLite uses `BEGIN IMMEDIATE` plus conditional `UPDATE ... WHERE stock_quantity >= ?` and checks `RowsAffected`. Transition orders with `UPDATE ... WHERE status <> 'RECEIVED'` and apply stock only when a row changed. | Concurrent sale of the last unit on both backends: exactly one succeeds, stock never negative. Concurrent receive: stock increments once. `EXPLAIN`/test check lock order. |
| DB-6 | **High** (conditional) | `internal/db/postgres.go:31-45`; `migrations/postgres/00001_initial_schema.sql:1-1247`; `docs/BACKEND.md:211-212` | Migration baseline | Goose tracks `goose_db_version`, not `alembic_version`. A populated legacy PG database has no goose row, so `00001` re-runs and fails. “Keeps version 1” does not baseline it. | Startup aborts against the real legacy database; an operator may drop data to “fix” it. | Implement one of: document empty-database-only deployment; or rehearsed baseline: backup, schema diff, insert version 1 into `goose_db_version` (or a goose baseline/stamp), then start at `00002+`. Never drop tables. | Restore a legacy dump; run the app; observe the failure; apply the baseline; `goose status` shows version 1 and startup succeeds. Cross-ref prior F5. |
| DB-7 | **Medium** (Phase 3 gate) | No SQLite business schema; PG enums at `00001:14-62`; `docs/DOMAIN.md:106-122` | Enums | SQLite has no enum type. If Phase 3 stores lowercase values, the shared Go mapping breaks; if it stores TEXT, `ORDER BY status` differs from PG enum declaration order (e.g. `PAID, PENDING, PARTIAL`). | Empty filters, wrong status order, or divergent API output between backends. | Store the same uppercase DB labels in SQLite with `CHECK (col IN (...))`; keep one `toDB`/`fromDB` mapping. For status ordering, use an explicit `CASE` or a sort column rather than relying on enum order. | Round-trip each status on both backends; assert JSON lowercase and DB uppercase; compare `ORDER BY status` on both. |
| DB-8 | **Medium** (Phase 3 gate) | `migrations/sqlite/00001_auth.sql` (future business tables); PG `00001:299-315,445-455,482-494`; prior F8/F10/F21 | Schema parity | SQLite TEXT does not enforce `varchar(n)`, enums, or decimal; it is easy to add extra cascades or defaults that PG does not have. `stock_quantity` is nullable in PG and a `NULL` bypasses `>= 0` (prior F10). | Offline rows accepted that PG rejects on sync; or behavior differences (stock, cascade, defaults) that only appear under SQLite. | Mirror PG nullability, uniques, checks, and FK actions; add length checks; keep the five Python-side defaults explicit; keep `sale_items`/`order_items` without cascade; make stock `NOT NULL` in SQLite. Do not add a stricter overpayment check. | Parity fixture: create the same rows on both; rows accepted offline that PG rejects must be zero for valid inputs. `PRAGMA foreign_key_check`; `INSERT` negative stock/NULL stock. |
| DB-9 | **Medium** | `internal/adapters/persistence/postgres/users.go:37-47`; `internal/adapters/persistence/sqlite/users.go:35-50`; both refresh-token `Create` methods | Error mapping | Unique violations are returned raw. PG uses SQLSTATE `23505`; SQLite uses constraint code `2067`/`1555`. The service has no `ErrConflict`. | Duplicate email/race returns 500 on one backend and 400/409 on the other once user creation is exposed. Contract parity fails. | Add `domain.ErrConflict`; map `23505` and the SQLite constraint codes in the adapters; map it to the documented HTTP status. | Create the same duplicate user against both backends; expect the same 4xx and `errors.Is(err, domain.ErrConflict)`. |
| DB-10 | **Medium** | `migrations/sqlite/00001_auth.sql:23-25`; `internal/db/sqlite.go:40-49`; `internal/bootstrap/database.go:79-84`; `docs/BACKEND.md:217-219` | Migration operations | SQLite `Down` drops the auth tables; PG has no `Down`. There is no migrate command, and `AUTO_MIGRATE=false` on a fresh file leaves no tables. | Accidental loss of local users/tokens in dev; broken startup in a production config. | Remove or guard the destructive Down under the forward-only policy; add a `migrate` command or require migration on first run. Document that Down is dev-only and requires a backup. | Fresh SQLite + `AUTO_MIGRATE=false`: login fails, then `migrate` fixes it. A guarded `goose down` refuses on a non-dev database. |
| DB-11 | **Medium** | `internal/bootstrap/database.go:33-41`; `internal/httpapi/router.go:15-17`; `cmd/seed/main.go:44-56`; `docs/SYNC.md:7-15` | Fallback visibility | Auto fallback is stderr-only, `Stores.Backend` is never surfaced, health has no backend, and the seed command follows `auto`. | Users write to a local dataset unknowingly; operators seed the wrong database; Phase 3 data is stranded. | Surface the backend in an authenticated status field and a persistent UI banner; emit a stable event; refuse `auto` for seed or require `--backend`; print the backend. | Stop PG: UI banner and `backend=sqlite`; seed with `auto` refuses or requires an explicit backend. Cross-ref security SQL-1/SQL-5. |
| DB-12 | **Medium** | `internal/db/sqlite.go:16-38`; `internal/bootstrap/database_test.go:95-124`; `docs/ARCHITECTURE.md:121-126` | Permissions / filesystem | Existing directory not tightened; chmod after `Ping`; WAL/-shm not checked; Windows `0600` is not an ACL; test covers only the main file and directory. | Other local users can read hashes/tokens if the directory is broad; a stale WAL survives an unclean shutdown. | Pre-create the DB `0600` before `sql.Open` (or umask `0077`), chmod an existing parent to `0700`, verify sidecars, reject symlinks, and store under a non-roaming directory on Windows. | `chmod 0755` the data dir; start; `stat` shows `700` for the directory and `600` for DB/WAL/SHM; a second OS user cannot read them. Cross-ref security SQL-2/SQL-7. |

### 6.2 Measure first / defer

| ID | Severity | Location | Category | Issue | Impact | Fix | Verification |
|---|---|---|---|---|---|---|---|
| DB-13 | Low | `internal/db/sqlite.go:27-28` | Pool/WAL | One connection is deliberate and correct for Phase 2, but WAL's concurrency benefit is unused and nested queries can deadlock. | Phase 3 read concurrency and any N+1 pattern. | Keep one connection until the transaction port exists. Then measure 4–8 connections with `busy_timeout` and `BEGIN IMMEDIATE` for writes; consider a read pool. | Load test concurrent auth/list requests; measure p95 and `SQLITE_BUSY`; no deadlock. |
| DB-14 | Low | SQLite `00001_auth.sql:21` vs PG `00001:1013-1023`; PG `:1097-1100` | Index parity | SQLite has `refresh_tokens.user_id`; PG does not (prior F7). SQLite lacks `users.full_name`. PG also has redundant `ix_*_id` indexes (prior F14). | Only matters when user deletion or user search is exposed. A btree cannot serve `ILIKE '%term%'` anyway. | Add `ix_refresh_tokens_user_id` to PG only if user deletion/revoke-all is exposed. Do not add `users.full_name` to SQLite for a leading-wildcard search; measure FTS5 or a scan. Drop `ix_*_id` only with `idx_scan` evidence. | `EXPLAIN` the cascade/delete and the user search. `PRAGMA index_list(users)`. |
| DB-15 | Low | `00001_auth.sql:9-10,18` vs PG `:560-561,417`; `internal/adapters/persistence/postgres/users.go:15` | Boolean/NULL parity | SQLite is `NOT NULL`; PG booleans are nullable. PG maps `NULL is_active` to `true` via `COALESCE`, while legacy `not user.is_active` treats `NULL` as inactive. A future nullable SQLite boolean would also break the `int64` scan. | A manually inserted `NULL is_active` row is active in Go but inactive in the legacy API; a `NULL` SQLite row would fail the scan. | Keep SQLite `NOT NULL`. Change the PG read to `COALESCE(is_active, false)` to match legacy (prior F13), or add `NOT NULL`/backfill later with counts and a backup. Use one bool helper. | Insert a `NULL is_active` row in PG; legacy rejects login, Go must also reject after the fix. Insert `NULL` into SQLite (must fail). |
| DB-16 | Low | `00001_auth.sql:5,14` vs PG sequences | PK semantics | `AUTOINCREMENT` vs sequence; neither reuses IDs after delete; SQLite adds `sqlite_sequence`. | No current behavior change. IDs are not global (DB-S2). | Keep. Do not rely on integer IDs for sync. | Insert/delete/insert on both; assert the same non-reuse expectation. |
| DB-17 | Low | `internal/adapters/persistence/sqlite/values.go:8-20`; PG timestamptz | Precision | SQLite keeps ns; PG keeps µs. RFC3339Nano vs 9-digit format. | <1 µs difference; string equality must not be used. | Compare instants; the contract test already tolerates it. | `contract_test.go:247-254`; a cross-backend timestamp fixture. |
| DB-18 | Low | Both adapters | Token cleanup | Expired/revoked rows are never deleted. No query needs an `expires_at` index. | Slow table growth; no security impact because only hashes are stored. | Add a purge job only if measured; then index `expires_at` for the purge. Do not add the index speculatively. | Row count over time; `EXPLAIN` the purge query. |
| DB-19 | Low | `internal/db/sqlite.go:22`; `internal/config/config.go:146-152,201-207` | Path/DSN | The path is interpolated unescaped; `?`, `#`, `%` change the URI; a relative path lands in the CWD; `:memory:` makes chmod fail. | In-memory DB/data loss or a wrong file path. | Build the DSN with `net/url`, require an absolute clean path, reject symlinks/non-regular files. | `SQLITE_PATH='x?mode=memory'` does not create an in-memory DB; a relative path is refused. Cross-ref security SQL-3. |
| DB-20 | Low | `internal/bootstrap/database.go:74-84`; `internal/db/sqlite.go:40-49` | Multi-instance | Two processes can race migrations or hit `SQLITE_BUSY`. | Startup failure in a rare double-launch. | Add an OS single-instance lock or a migration lock with retry; or document that two instances are unsupported. | Launch two instances on a fresh file; both start or one exits cleanly; `PRAGMA journal_mode` remains `wal`. |
| DB-21 | Low | `docs/BACKEND.md:126`; PG uses `ILIKE` | Search parity | SQLite `LIKE`/`lower()` fold ASCII only; PG `ILIKE` is locale-aware. `pg_trgm` has no SQLite equivalent. | Different search results for accented Spanish text (`árbol` vs `ÁRBOL`). | Decide: normalize in Go, restrict to ASCII, or use SQLite FTS5; document the limitation. Test accented and uppercase terms. | Same fixture on both backends returns the same IDs for `árbol`/`ÁRBOL`. |
| DB-22 | Low | PG status enums `00001:14-62`; future SQLite TEXT | Sort parity | PG sorts by enum declaration order; SQLite sorts TEXT alphabetically. | Different list order for status-sorted lists. | Use an explicit `CASE`/sort column; do not rely on enum order. | `ORDER BY status` on both backends yields the same IDs. |
| DB-23 | Low | No list endpoints yet; `docs/BACKEND.md:125` | Pagination | Without a unique tie-breaker, `limit+1` pagination can duplicate or skip rows across backends. | Phase 3 list instability. | Always order by a total order, e.g. `ORDER BY created_at DESC, id DESC`. | Create rows with equal timestamps; page through and assert no duplicates/skips on both backends. |
| DB-24 | Low | `docs/ARCHITECTURE.md:130-131`; dashboard `America/Bogota` | Timezone aggregation | SQLite has no `AT TIME ZONE`/`date_trunc`; SQL `CURRENT_TIMESTAMP` has the wrong format. Bogotá is fixed UTC-5. | Offline dashboard day boundaries can be wrong. | Group in Go, or use `datetime(ts, '-5 hours')` against canonical text. Never use `CURRENT_TIMESTAMP` for stored timestamps. | Compare today/7-day buckets on both backends around midnight Bogotá. |

### 6.3 Sync-only gates (not Phase 3 local-operation blockers, but design now)

| ID | Severity | Location | Category | Issue | Impact | Fix | Verification |
|---|---|---|---|---|---|---|---|
| DB-S1 | **High** | `00001:299-315` (`products.stock_quantity`); `docs/DOMAIN.md:157-168`; `docs/SYNC.md:32-36` | Stock model | Stock is a mutable scalar with no movement ledger. Sale delete restores stock; received-order delete does not. | Last-write-wins sync corrupts stock; replaying a sale/delete double-applies. | Add `stock_movements` (sale, sale_cancel, purchase_receipt, adjustment) with a source key; derive stock or keep a cache reconciled to the ledger. Backfill from sale/order history plus an opening balance. | Sum the ledger equals `stock_quantity` for every product; two-device last-unit test; replay the same movement twice, quantity unchanged. |
| DB-S2 | **High** | All tables; `00001:301` (`sku`), `:139` (`identity_card`) | Global identity | No `sync_id`; integer PKs collide; random `SKU-XXXXXXXX` is not a safe cross-device key; `identity_card` is optional. | Merges overwrite the wrong row; idempotent apply is impossible. | Add `sync_id` UUIDv7 `UNIQUE` per syncable table in the sync migration; keep local integer PKs for FKs. Do not use SKU as identity. | Two devices create a product offline; both sync; both rows survive, no overwrite. |
| DB-S3 | **High** | Timestamps only at `00001:136-137,484-485`; no `deleted_at`/outbox/version | Sync metadata | Missing `updated_at` on most tables, no monotonic sequence, no tombstones, no outbox. | Cannot order writes, propagate deletes, or retry; SYNC constraints 2–4 fail. | Dedicated sync migration: app-set `updated_at`, `deleted_at`, `version`, device sequence, and `sync_outbox` written in the business transaction. | Clock-skew, replay, and delete/restore tests from `SYNC.md:74-80`. |
| DB-S4 | **High** | `00001:490` (`sales.created_by_id`), `:348`, `:382`; SQLite `users` `:4-11` | Cross-dataset user identity | Business rows reference local integer user IDs. After sync, a sale’s author can map to a different person. | Wrong attribution/audit trail; possible privilege confusion. | Give users a global `sync_id`; map local FKs during apply; keep the local FK for display but never use it as the cross-device identity. | Two databases with different user ID assignments; sync a sale; the author is correct on both. |
| DB-S5 | Medium | `00001:445-455` (`sale_items`), `:230-239` (`order_items`), `:521-528` (`supplier_catalogs`) | Idempotency keys | No unique `(sale_id, product_id)`, `(order_id, product_id)`, `(product_id, provider_id)` (prior F8). | Replaying an outbox item duplicates lines or catalogs; double stock. | Add the unique indexes after a duplicate check, or give line rows client-generated `sync_id`s; upsert on the natural key. | Replay the same outbox item; row counts and stock unchanged. |
| DB-S6 | Medium | `docs/SYNC.md:37-38`; DB-2 | Money in sync | Sync payloads must be decimal strings; SQLite storage is undecided. | Float corruption if SQLite uses NUMERIC/REAL. | Decide storage (DB-2) and serialize decimal strings on the wire; never floats. | `typeof()` and exact decimal compare across the payload and both DBs. |
| DB-S7 | Low | `00001:266-272` (`price_history`) | Price history | `changed_at` exists, but there is no `sync_id`; price history is not currently used as a ledger. | Price changes may not merge or may duplicate. | Decide whether `price_history` is the price-change ledger; if yes, add sync metadata when sync lands. | Price change on two devices; history merges without duplicates. |
| DB-S8 | Low | `00001:412-418`; `docs/SYNC.md:46-48` | Auth table sync | Refresh tokens are local. A naive “copy all tables” sync would replicate them. | Token replay across devices. | Explicitly exclude `users`/`refresh_tokens` from sync or use an allowlist of syncable tables; never sync refresh tokens. | A sync run leaves `refresh_tokens` unchanged on both sides. |

---

## Schema changes

No change is proposed for the shipped migrations. The following is **Phase 3
or sync-phase DDL guidance**, not a request to apply it now.

### 1. Current auth fix (no DDL)

DB-3 is a code fix, not a schema change. Keep the writer fixed-width and relax
the reader:

```go
// internal/adapters/persistence/sqlite/values.go
func parseTimestamp(value string) (time.Time, error) {
    parsed, err := time.Parse(time.RFC3339Nano, value)
    if err != nil {
        return time.Time{}, fmt.Errorf("parse timestamp %q: %w", value, err)
    }
    return parsed.UTC(), nil
}
```

Do **not** change `formatTimestamp` to `RFC3339Nano`; variable-length
fractions break the lexicographic `expires_at > ?` comparison.

### 2. SQLite money (choose one before Phase 3)

Recommended: integer cents.

```sql
-- SQLite business tables, Phase 3.
-- Column names may differ from PostgreSQL; the adapter maps them.
price_cents        INTEGER NOT NULL CHECK (price_cents >= 0),
cost_cents         INTEGER NOT NULL CHECK (cost_cents > 0),
total_amount_cents INTEGER NOT NULL CHECK (total_amount_cents >= 0),
amount_paid_cents  INTEGER NOT NULL CHECK (amount_paid_cents >= 0),
unit_price_cents   INTEGER NOT NULL CHECK (unit_price_cents >= 0),
subtotal_cents     INTEGER NOT NULL CHECK (subtotal_cents >= 0)
-- Convert to/from shopspring/decimal in the SQLite adapter.
```

Acceptable only if you accept weaker constraints and never aggregate in SQL:
`price TEXT NOT NULL` with an adapter-enforced decimal format. Never
`NUMERIC(12,2)`/`REAL`.

### 3. SQLite timestamps for new columns

```sql
-- New SQLite columns only. Do not rebuild refresh_tokens just for this.
created_at TEXT NOT NULL CHECK (
    length(created_at) = 30
    AND created_at GLOB
      '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9].[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]Z'
),
updated_at TEXT
```

Prefer `TEXT` over `DATETIME` for new columns so the declared affinity no
longer looks numeric. Keep one `storeTimestamp`/`scanTimestamp` helper.

### 4. SQLite enums and length checks

```sql
-- Same DB labels as PostgreSQL; one Go mapping for both backends.
payment_status TEXT NOT NULL DEFAULT 'PENDING'
    CHECK (payment_status IN ('PAID', 'PENDING', 'PARTIAL')),
delivery_status TEXT NOT NULL DEFAULT 'PENDING'
    CHECK (delivery_status IN ('PENDING', 'PARTIAL', 'DELIVERED')),
order_status TEXT NOT NULL DEFAULT 'PENDING'
    CHECK (order_status IN ('PENDING', 'SENT', 'RECEIVED', 'CANCELLED')),
purchase_order_payment_status TEXT NOT NULL DEFAULT 'PENDING'
    CHECK (purchase_order_payment_status IN ('PENDING', 'PAID')),

abbreviation TEXT NOT NULL CHECK (length(abbreviation) <= 5),
sku          TEXT NOT NULL CHECK (length(sku) <= 100),
name         TEXT NOT NULL CHECK (length(name) <= 150),
size_value   TEXT         CHECK (size_value IS NULL OR length(size_value) <= 20)
```

Do not rely on `ORDER BY status`; use `CASE` or a sort column.

### 5. Unique natural keys (verify duplicates before adding)

```sql
-- PostgreSQL and SQLite, only after counting existing duplicates (prior F8).
CREATE UNIQUE INDEX IF NOT EXISTS ux_sale_items_sale_product
    ON sale_items (sale_id, product_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_order_items_order_product
    ON order_items (order_id, product_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_supplier_catalogs_product_provider
    ON supplier_catalogs (product_id, provider_id);
```

These serve the “one row per product per sale/order” invariant and idempotent
upserts during sync.

### 6. Sync-phase only (do not add now)

```sql
-- PostgreSQL. SQLite uses TEXT for identifiers/timestamps; see the next block.
ALTER TABLE products ADD COLUMN sync_id    uuid;
ALTER TABLE products ADD COLUMN updated_at timestamptz;
ALTER TABLE products ADD COLUMN deleted_at timestamptz;
ALTER TABLE products ADD COLUMN version    bigint NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX ux_products_sync_id ON products (sync_id);

CREATE TABLE sync_outbox (
    id           bigserial PRIMARY KEY,
    entity       text NOT NULL,
    sync_id      uuid NOT NULL,
    operation    text NOT NULL CHECK (operation IN ('insert','update','delete')),
    payload      text NOT NULL,               -- decimal strings, no floats
    attempts     integer NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL
);

CREATE TABLE stock_movements (
    id             bigserial PRIMARY KEY,
    product_id     bigint NOT NULL REFERENCES products(id),
    kind           text NOT NULL CHECK (kind IN
                     ('sale','sale_cancel','purchase_receipt','adjustment')),
    quantity_delta integer NOT NULL,
    source_type    text NOT NULL,
    source_id      bigint NOT NULL,
    created_at     timestamptz NOT NULL,
    sync_id        uuid UNIQUE
);
```

```sql
-- SQLite equivalent. Timestamps use the canonical 30-char TEXT format.
-- Do not use DATETIME/NUMERIC for new timestamp/identifier columns.
ALTER TABLE products ADD COLUMN sync_id    TEXT;
ALTER TABLE products ADD COLUMN updated_at TEXT;
ALTER TABLE products ADD COLUMN deleted_at TEXT;
ALTER TABLE products ADD COLUMN version    INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX ux_products_sync_id ON products (sync_id);

CREATE TABLE sync_outbox (
    id           INTEGER PRIMARY KEY,
    entity       TEXT NOT NULL,
    sync_id      TEXT NOT NULL,
    operation    TEXT NOT NULL CHECK (operation IN ('insert','update','delete')),
    payload      TEXT NOT NULL,
    attempts     INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL
);

CREATE TABLE stock_movements (
    id             INTEGER PRIMARY KEY,
    product_id     INTEGER NOT NULL REFERENCES products(id),
    kind           TEXT NOT NULL CHECK (kind IN
                     ('sale','sale_cancel','purchase_receipt','adjustment')),
    quantity_delta INTEGER NOT NULL,
    source_type    TEXT NOT NULL,
    source_id      INTEGER NOT NULL,
    created_at     TEXT NOT NULL,
    sync_id        TEXT UNIQUE
);
```

`SYNC.md:50-62` already says not to add these speculatively. This block is the
shape to use when the sync protocol is designed, not a Phase 3 task.

### 7. Indexes

No new index is justified for the current auth schema.

- PostgreSQL `refresh_tokens.user_id`: only if user deletion or revoke-all is
  exposed. It serves `ON DELETE CASCADE` from `users` and a future
  “revoke all tokens for user” query. Prior F7.
- SQLite `users.full_name`: do not add for `ILIKE '%term%'`; a btree cannot
  serve a leading wildcard. Measure FTS5 or a scan when offline user search
  exists.
- Future SQLite business indexes: mirror PG only for the queries that actually
  run offline, and name them. The known hot paths are `sale_items.sale_id`
  (sale detail/delete), `sale_items.product_id` (product history/top
  products), `order_items.order_id` (order detail), `order_items.product_id`
  (product order history), `sales.client_id` (client sales), `sales.created_by_id`
  (sales-by-user filter, prior F6), `purchase_orders.provider_id` (provider
  orders), and `supplier_catalogs.(product_id, provider_id)` (receive
  lookup, prior F4). Do not add status/date composite indexes without
  `EXPLAIN`.

---

## Safe migration plan

1. **Now — code-only timestamp fix (DB-3).**
   - Change `parseTimestamp` to accept `RFC3339Nano` and return UTC.
   - Add tests for zero and trailing-zero nanoseconds.
   - No schema change, no migration, no data rewrite. Freeze
     `00001_auth.sql`.
2. **Before Phase 3 — decide and document.**
   - Money representation in SQLite (integer cents or decimal TEXT).
   - Canonical timestamp representation for new columns.
   - User identity (`sync_id`/mapping) before `created_by_id` rows exist.
   - Stock ledger vs mutable stock (ADR), and the transaction/querier port.
   - Enum representation and status ordering.
   These are decisions, not migrations. Record them in `docs/DECISIONS.md`.
3. **PostgreSQL baseline (DB-6), only if a populated legacy DB is in scope.**
   - `pg_dump -Fc` the database.
   - Restore to a scratch database and diff against a goose-migrated empty
     database (exclude `alembic_version` and `goose_db_version`).
   - If the diff is empty, insert version 1 into `goose_db_version` on the
     real database (or use goose's baseline/stamp). Back up first.
   - Start with `AUTO_MIGRATE=true` and confirm `goose status` shows version 1.
   - If the diff is not empty, stop: do not “fix” it with a destructive import.
4. **SQLite backup before any schema change.**
   - Prefer a clean shutdown, then copy `inventory.sqlite`, `-wal`, and `-shm`
     together.
   - While running, use `VACUUM INTO '/backup/inventory-YYYYMMDD.sqlite'`.
   - Restore to a temporary path and run `PRAGMA integrity_check` and
     `SELECT count(*)` before trusting the backup.
5. **Phase 3 — one coherent SQLite business schema.**
   - Add `00002_business_schema.sql` (and only additional numbered files if
     needed) under `migrations/sqlite/`. Do not translate tables piecemeal per
     module (`docs/MIGRATION.md:69-71,127-134`).
   - Mirror PG order, nullability, uniques, checks, and FK actions; use
     integer cents/text for money; `TEXT` + CHECK for timestamps; `TEXT` +
     CHECK for enums; `INTEGER` 0/1 `NOT NULL` for booleans; length checks for
     `varchar(n)`; no extra cascades; explicit values for the five Python-side
     defaults.
   - Run migrations in a transaction; test a fresh file and a copy of a real
     file; run the same fixtures against PostgreSQL and SQLite.
   - Do not add sync columns in this migration; sync gets its own dedicated
     migration after the protocol is designed.
6. **Permissions and concurrency (DB-12, DB-20).**
   - Pre-create the DB file `0600` before `sql.Open`, or set umask `0077`;
     chmod an existing parent to `0700`; verify `-wal`/`-shm`.
   - Add an OS single-instance lock or a migration lock with retry; keep the
     busy timeout; use `BEGIN IMMEDIATE` for write transactions if concurrency
     is introduced.
7. **Rollback policy.**
   - Forward-only in release. Keep `Down` only for disposable dev databases.
   - Never edit a shipped migration. Every destructive change starts with a
     backup and a restore rehearsal.

### Verification commands (for the user to run)

These are intentionally not run here.

```bash
# Schema parity
psql "$DATABASE_URL" -c '\d users' -c '\d refresh_tokens'
sqlite3 "$SQLITE_PATH" '.schema users' '.schema refresh_tokens'
sqlite3 "$SQLITE_PATH" 'PRAGMA index_list(users); PRAGMA index_list(refresh_tokens);'
sqlite3 "$SQLITE_PATH" 'PRAGMA foreign_key_list(refresh_tokens);'

# Timestamp storage / comparison
sqlite3 "$SQLITE_PATH" "SELECT expires_at, typeof(expires_at) FROM refresh_tokens;"
sqlite3 "$SQLITE_PATH" "SELECT expires_at > '2026-01-01T00:00:00.000000000Z' FROM refresh_tokens;"

# Money (after Phase 3 tables exist)
sqlite3 "$SQLITE_PATH" "SELECT typeof(price_cents), SUM(price_cents) FROM products;"

# Integrity / restore
sqlite3 "$SQLITE_PATH" 'PRAGMA integrity_check; PRAGMA foreign_key_check;'
goose -dir migrations/sqlite status

# Contract tests both backends
INVENTORY_TEST_DATABASE_URL="$DATABASE_URL" go test ./internal/adapters/persistence/...
go test ./...
```

---

## Open questions

1. Will the desktop app ever attach to the **populated legacy PostgreSQL**
   database, or only to an empty database created by goose? This decides
   whether DB-6 is a release blocker or a documented non-goal.
2. Does Phase 3 ship **offline business writes** (customers, sales, stock), or
   is SQLite auth-only until sync exists? `AGENTS.md:54-55` and
   `docs/MIGRATION.md:127-134` imply full offline business schema in Phase 3,
   but that makes DB-2/DB-4/DB-5 immediate.
3. Money in SQLite: **integer cents** (recommended) or decimal TEXT? This must
   be an ADR, not an incidental choice in the first table.
4. Is there a stable **installation/dataset identifier** to bind JWTs to
   (DB-1), or will the app use a per-backend `SECRET_KEY`? Multi-station
   deployments (`docs/RELEASE.md:169-170`) make `iss`/`aud` mandatory if a
   secret is shared.
5. Are offline users provisioned independently in both databases, and how are
   password resets, deactivation, and role changes propagated? The current
   `cmd/seed` path is manual and can target the wrong dataset.
6. Is **stock a mutable scalar** in Phase 3, or does the movement ledger land
   at the same time? Retrofitting the ledger is a data migration, not a schema
   tweak.
7. Is **single-instance** guaranteed on Windows and macOS? If not, DB-20
   becomes a Medium operational finding.
8. What are the expected row counts for products, sales, and items? The index
   proposals in section 6.2 are intentionally gated on measurement.
9. What is the encryption-at-rest decision for Phase 3 PII/pricing
   (SQLCipher, OS-level full-disk, or explicit acceptance), and are backups
   covered by the same decision?
10. Is `AUTO_MIGRATE=false` a supported release mode? If yes, the missing
    `migrate` command is a release blocker, not just a dev inconvenience.

---

## Out of scope

- No code, SQL, migration, configuration, or test changes.
- No new index without a named query (section 7).
- No sync protocol, outbox implementation, encryption implementation, or
  conflict UI; `docs/SYNC.md` correctly defers those.
- No schema “cleanup” of legacy artifacts during the behavior port (prior
  F14/F15/F21).

---

## Resolution status (post-review)

Appended after re-inspection of the current code and tests. The findings above
are unchanged; this section only records what has since been closed.

### Resolved

| ID | Status | Evidence |
|---|---|---|
| DB-3 | **Resolved** | `migrations/sqlite/00001_auth.sql:17` now declares `expires_at TEXT NOT NULL` (no NUMERIC-affinity `DATETIME`). `sqlite/values.go:8-12` keeps the fixed-width 30-char UTC writer; `:14-20` parses with `time.RFC3339Nano` and normalizes to UTC. `values_test.go` covers zero, `123456780` (trailing zero), and `123456789` (full) nanoseconds; `refresh_tokens_test.go` drives the same three through `Create` → driver normalization → `FindValid`, expiry rejection, and `Rotate`. |
| DB-1 | **Resolved (SQLite↔Postgres)** | JWT `iss` is `inventory-desktop/<backend>` (`internal/service/auth.go:43`, wired from `stores.Backend` in `main.go:38` and `cmd/server/main.go:38`), and `Parse` requires it (`internal/auth/jwt.go:31-48`). Unit coverage in `auth_test.go:39-76`; end-to-end coverage in `httpapi/auth_issuer_test.go:25-79` proves a token minted for one backend is 401 on the other while its own token still authenticates. |
| DB-11 (partial) | **Partially resolved** | `GET /healthz` now returns `{"status","database"}` (`internal/httpapi/router.go:22-24`), covered by `health_test.go:14-58`. Persistent UI/authenticated banner and backend-aware `cmd/seed` remain open. |
| DB-12 (partial) | **Partially resolved** | `OpenSQLite` rejects symlinks before opening (`db/sqlite.go:18,45-57`), chmods an existing parent to `0700` (`:26-28`), and chmods DB/WAL/SHM via `secureSQLiteFiles` (`:59-65`). Tests: `db/sqlite_test.go:11-55` (existing `0755` dir tightened, symlink rejected) and `:57-94` (main + sidecar `0600`). Residual: chmod still runs only after `Ping`, and Windows ACLs are unchanged. |
| DB-19 (partial) | **Partially resolved** | Symlink paths are refused (`db/sqlite.go:45-57`); the DSN is still built with an unescaped `fmt.Sprintf` path (`:30`), and no absolute-path check was added. |

### Still open

| ID | Status | Gate |
|---|---|---|
| DB-2 | **Open** | SQLite money representation (integer cents vs decimal TEXT) is still undecided, and no business tables exist. Must be an ADR before the first money column. |
| DB-4 | **Open** | Ports still expose no transaction handle (`internal/ports/*.go`); no unit-of-work/`Querier`. Multi-repo atomicity and the single-connection nested-query deadlock remain unaddressed. |
| DB-5 | **Open** | No `FOR UPDATE` substitute: no `BEGIN IMMEDIATE` write path, no conditional stock `UPDATE ... WHERE stock_quantity >= ?`, no deterministic lock ordering. |
| DB-7/DB-8 | **Open** | The full offline business schema is not written; enums, length CHECKs, nullability, FK actions, and the five Python-side defaults still need the Phase 3 migration. |
| DB-S1/S2/S3/S5 | **Open** | No `stock_movements`, `sync_id`, `updated_at`, tombstones, version, or `sync_outbox`; line-item natural keys are not enforced. |
| DB-6 | **Open** | No goose baseline for a populated legacy PostgreSQL database; `alembic_version` still does not satisfy `goose_db_version`. |
| DB-9/DB-10/DB-20 | **Open** | Unique-violation mapping, the missing `migrate` command / `AUTO_MIGRATE=false` behavior, and multi-instance migration locking are unchanged. |
| DB-11 remainder | **Open** | Backend is not surfaced in the UI or an authenticated status field, and `cmd/seed` still follows `DB_MODE=auto`. |

### Verification already in place

- `go test ./internal/adapters/persistence/sqlite/...` — timestamp and refresh round-trip.
- `go test ./internal/auth/... ./internal/httpapi/...` — issuer binding and `/healthz`.
- `go test ./internal/db/... ./internal/bootstrap/...` — permissions and symlink rejection (POSIX only; Windows permission tests skip).

No code, SQL, migration, or configuration was changed here.
