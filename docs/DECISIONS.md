# Decisions

Architecture decision records for Inventory Desktop. Add a new entry when a
decision changes the stack, contract, or security posture.

## ADR-0001 — Wails v2 as the desktop shell

**Status:** Accepted

**Context:** The app is a React + Go desktop application. The environment
already has Go 1.26 and Wails 2.16 installed. Alternatives were Electron with
a Go sidecar and Tauri with a Rust backend.

**Decision:** Use Wails v2. It embeds the React build and runs the Go backend
in the same process.

**Consequences:** One language for the backend, one build pipeline, and a
native webview per OS. The UI is limited to the platform webview rather than
a bundled Chromium.

## ADR-0002 — Local REST API instead of Wails bindings for data

**Status:** Accepted

**Context:** The legacy React app calls a REST API through axios. Wails
bindings would require replacing the data layer, auth interceptors, and many
tests.

**Decision:** The Go backend exposes the legacy `/api/v1` contract on
`127.0.0.1` with an ephemeral port. Only the base URL is exposed through a
Wails binding.

**Consequences:** The React data layer is reused. Loopback HTTP introduces a
local attack surface, addressed by bearer auth, a random port, and
loopback-only binding. CORS is restricted to the webview origin.

## ADR-0003 — PostgreSQL remains the primary database

**Status:** Accepted; extended by ADR-0012

**Context:** The legacy schema, Alembic migrations, dashboard queries, and
business rules are built on PostgreSQL. SQLite would require translating the
schema and query semantics.

**Decision:** Keep PostgreSQL 17 and the existing schema as the primary
database. The desktop app connects with a configurable `DATABASE_URL`.

**Consequences:** The installer requires a reachable PostgreSQL instance for
full functionality. The development environment provides one through Docker.
Standalone use is covered by the local SQLite fallback in ADR-0012;
bundling a PostgreSQL server is deferred.

## ADR-0004 — Full Go port with contract parity

**Status:** Accepted

**Context:** Running the FastAPI service beside the desktop app would keep a
Python runtime, a second process, and a remote dependency. The user chose a
full rewrite.

**Decision:** Reimplement all 67 legacy endpoints in Go, preserving the
contract. The FastAPI app remains the reference for behavior and fixtures.

**Consequences:** The port is large and must be sequenced by module. Contract
tests are mandatory. Any intentional behavioral change is documented.

## ADR-0005 — Root Wails layout

**Status:** Accepted

**Context:** A separate `apps/desktop` and `apps/server` layout adds
configuration for a server that does not exist. The product is a single
desktop app.

**Decision:** Use the conventional Wails layout at the repository root:
`main.go`, `app.go`, `wails.json`, `internal/`, `migrations/`, `frontend/`.

**Consequences:** Tooling works without custom paths. If a hosted server is
added later, the Go packages can move without changing the domain code.

## ADR-0006 — HashRouter in the desktop build

**Status:** Accepted

**Context:** The legacy app uses `BrowserRouter`. The Wails asset server does
not provide a guaranteed SPA history fallback for every platform and scheme.

**Decision:** Use `HashRouter` in the desktop build. Browser development may
use `BrowserRouter` behind a build-time switch.

**Consequences:** Routes survive reloads and deep links in the webview.
Playwright tests must use hash URLs.

## ADR-0007 — Normalized error envelope

**Status:** Accepted

**Context:** FastAPI returns `422` validation errors with an array-shaped
`detail`, while the frontend error helper only handles a string. Other
FastAPI errors return a string `detail`.

**Decision:** Every Go error returns `{ "detail": string, "code"?: string }`.
Validation errors add an optional `errors` array but keep `detail` readable.

**Consequences:** The frontend has one error path. Contract tests treat this
as a documented difference from FastAPI.

## ADR-0008 — Colombia locale and COP

**Status:** Accepted

**Context:** The legacy UI formats with `es-PE` (Peruvian Sol), while the
backend groups dashboard days with `America/Bogota` and the business context
is Colombia. The two cannot both be correct.

**Decision:** `es-CO`, COP, `$ 1.234,56`, and `America/Bogota`.

**Consequences:** Frontend formatters and display tests change. The API is
unaffected because money is numeric and time is UTC.

## ADR-0009 — Token storage in WebView localStorage for v1

**Status:** Accepted, with risk

**Context:** The legacy app stores access and refresh tokens in
`localStorage`. Moving to OS keyring storage requires new Go bindings,
migration of existing sessions, and additional UI states.

**Decision:** Keep the legacy storage model for v1. Document the XSS risk and
require a security review of every new HTML injection point.

**Consequences:** The change is small and testable. The security agent tracks
OS keyring migration as a hardening item.

## ADR-0010 — Generate and persist the JWT secret on first run

**Status:** Accepted

**Context:** A desktop install cannot rely on a `.env` file. A hardcoded
secret would be public.

**Decision:** If `SECRET_KEY` is unset, generate 32 random bytes on first run
and store them in the OS config directory with owner-only permissions. An
explicit environment value overrides the stored value.

**Consequences:** Tokens are valid only for the installation that issued
them. A shared database used by several counters requires a shared secret
configured explicitly.

## ADR-0011 — Hexagonal ports/adapters for the auth slice

**Status:** Accepted

**Context:** The first auth slice coupled services directly to pgx
repositories. The remaining modules will target PostgreSQL and may need a
local fallback. The user asked for the architecture to be settled before
porting them.

**Decision:** Put entities and domain errors in `internal/domain`. Define
use-case interfaces in `internal/ports/inbound` and repository/security
interfaces in `internal/ports/outbound`. Keep use cases in
`internal/application`; they implement the inbound port and depend only on
domain plus outbound ports. Implement persistence in
`internal/adapters/outbound/persistence/{postgres,sqlite}` and JWT, bcrypt, and
refresh-token crypto in `internal/adapters/outbound/security`. The inbound HTTP
adapter lives in `internal/adapters/inbound/httpapi`, depends on the inbound
port, and owns its HTTP DTOs.

**Consequences:** Application services compile without pgx, SQLite,
`database/sql`, concrete security implementations, or any concrete adapter.
Backends and security implementations can be swapped by bootstrap wiring and
tested independently. Domain entities carry no HTTP tags. New modules must
follow the same port/adapter direction; adapter-specific types do not leak into
services.

## ADR-0012 — Automatic PostgreSQL to SQLite fallback

**Status:** Accepted; extended by ADR-0021

**Context:** The desktop app must remain usable when PostgreSQL is not
reachable, and a future sync feature needs a local source of truth. Shipping
CGO or an embedded PostgreSQL server adds packaging and operational cost.

**Decision:** Add `DB_MODE` with `auto` (default), `postgres`, and `sqlite`.
In `auto`, probe PostgreSQL with a 3-second timeout; on failure log a warning
and open SQLite through `modernc.org/sqlite` (pure Go). SQLite enables foreign
keys, a busy timeout, and WAL, stores data under the OS config directory, and
since Phase 3 contains the auth tables and the full business schema.
PostgreSQL remains the full-feature backend.

**Consequences:** Standalone installs can authenticate, persist sessions, and
run the business modules offline. The full SQLite schema and parity tests
landed in Phase 3. Automatic fallback can hide a misconfigured DSN, so the
warning is logged, the UI shows a persistent offline banner, and
`DB_MODE=postgres` is available for deployments that must fail hard. Sync is
not implemented; it will require stable sync IDs, reliable timestamps,
tombstones, and an outbox.

## ADR-0013 — Bind access tokens to the active backend

**Status:** Accepted

**Context:** PostgreSQL and SQLite user IDs both start at 1. With a shared
`SECRET_KEY`, a token minted while the app was running on SQLite could be
accepted by a PostgreSQL-backed process as a different user with the same ID.

**Decision:** Access tokens carry `iss` = `inventory-desktop/<backend>`, plus
`iat` and a random `jti`. Token parsing requires the expected issuer and
HS256. The backend name is passed to the auth service during bootstrap.

**Consequences:** Tokens cannot cross between the SQLite and PostgreSQL
datasets, even when user IDs collide. Tokens issued before this change are
rejected. A future multi-backend design with shared identities must replace
the simple backend issuer with a stable installation/tenant identifier.

## ADR-0014 — GORM limited to outbound persistence adapters

**Status:** Accepted

**Context:** The remaining business modules are mostly CRUD over the legacy
schema. Hand-written SQL for every table is slow to port and easy to drift, but
letting an ORM into services or handlers would break the ports/adapters
direction and make the PostgreSQL/SQLite contract harder to guarantee.

**Decision:** Add `gorm.io/gorm`, `gorm.io/driver/postgres`, and the pure-Go
`github.com/glebarez/sqlite` driver. Both backends are implemented by one
outbound adapter, `internal/adapters/outbound/persistence/gormstore`, behind
the existing `internal/ports/outbound` interfaces. GORM does not appear in
domain, ports, application, or inbound HTTP; `internal/db` owns the open
helpers, and `internal/bootstrap` may hold the opaque `*gorm.DB` handle during
wiring. Migrations stay on goose SQL and run against the `*sql.DB` that GORM
exposes; GORM AutoMigrate is forbidden. Raw SQL remains allowed inside the adapter for dashboard
aggregates, stock locking, and complex statements.

**Consequences:** CRUD ports share one implementation and one contract suite
across PostgreSQL and SQLite. The PostgreSQL driver reuses `pgx/v5`. SQLite
keeps its pragmas, permission hardening, and fixed-width UTC timestamp text
through `gormstore.Timestamp`. GORM is configured with a silent logger so SQL
values and token material never reach logs. Queries that need precise locking
or aggregation may still be written by hand without introducing a second
persistence abstraction.

## ADR-0015 — Inventory search treats SQL wildcards literally

**Status:** Accepted

**Context:** Legacy FastAPI built catalog search as `ILIKE '%' || term || '%'`,
so `%` and `_` in the term acted as SQL wildcards. The SQLite Go-side filter
folds case in Go over a full-table scan, which cannot reproduce `ILIKE`
wildcard semantics. Backend parity requires the same observable behavior on
PostgreSQL and SQLite. Product search has the same constraint and additionally
matches both `name` and `sku`.

**Decision:** Inventory search performs literal substring matching on both
backends. PostgreSQL uses `strpos(lower(name), lower(?)) > 0` (plus `sku` for
products); SQLite uses Go case folding. `%` and `_` are ordinary characters.

**Consequences:** Results are consistent across backends, but `%` and `_` no
longer act as wildcards, a documented deviation from the legacy API.

## ADR-0016 — SQLite money is stored as integer cents

**Status:** Accepted

**Context:** PostgreSQL stores money as `NUMERIC(12,2)` and Go reads it as a
decimal. SQLite has no exact decimal type; storing `REAL` would lose precision
and break the money invariant. The Phase 3 SQLite schema already declares the
money columns as `INTEGER`.

**Decision:** Keep the legacy column names unchanged. SQLite stores money as
integer cents. The GORM adapter normalizes at the boundary: reads convert
integer cents with `decimal.New(cents, -2)`; writes round to two decimals and
use `Shift(2).IntPart()`. PostgreSQL keeps `NUMERIC(12,2)`; the adapter reads it
as text with `decimal.NewFromString` and writes `StringFixed(2)`. Aggregates
such as `SUM(stock_quantity * price)` stay integer on SQLite until the same
conversion.

**Consequences:** Money never passes through `float64`, and sums are exact on
both backends. Every future money column and aggregate must use the same
normalization. Raw SQL written against SQLite must remember that values are
cents, not currency units.

## ADR-0017 — Reject unknown JSON fields at the HTTP boundary

**Status:** Accepted

**Context:** Legacy Pydantic ignores unknown fields. Go already uses
`DisallowUnknownFields`, which caught the real `min_stock_quantity` drift
between clients and the API.

**Decision:** Reject unknown fields on JSON bodies with `422 VALIDATION_ERROR`.

**Consequences:** The Go API is stricter than legacy, catching client/API drift
early. Clients must send exact field names. The difference is documented in
[MIGRATION.md](MIGRATION.md#known-legacy-quirks).

## ADR-0018 — UnitOfWork and StockRepository for stock mutations

**Status:** Accepted

**Context:** Sale creation, item mutations, deletion, and future order
receiving must update stock and business rows atomically. Services own those
transactions, but the application layer cannot import GORM or open a
`*gorm.DB` transaction directly. Repository methods must also not start nested
transactions when called from a service boundary.

**Decision:** Add `outbound.UnitOfWork` (`WithinTx`) plus `outbound.TxRepos`
and a narrow `outbound.StockRepository`. `WithinTx` wraps `db.Transaction` in
`gormstore` and hands the callback transaction-bound sale, product, client,
and stock stores. `StockRepository.LockForUpdate` selects product quantities
in ascending id order, using `FOR UPDATE` on PostgreSQL and a plain ordered
select on SQLite. `ApplyDelta` is a conditional update that returns
`domain.ErrInsufficientStock` when the new quantity would be negative. Sale
services use the read repository for list, get, and stats, and the unit of
work for every mutation.

**Consequences:** Sales, item changes, and deletion commit or roll back as one
unit without leaking GORM into the application layer. PostgreSQL serializes
competing stock writers with row locks; SQLite relies on its single
connection and transaction. Future order receiving uses the same boundary.
Helpers that call repositories with a transaction-bound `*gorm.DB` must not
open another transaction.

## ADR-0019 — Sales edge cases follow DOMAIN over legacy code

**Status:** Accepted

**Context:** The legacy API accepts duplicate product rows in a sale, allows
`delivered_quantity` above `quantity` on some paths, and marks a zero total with
zero paid as `paid` because `0 >= 0`. `docs/DOMAIN.md` requires one item per
product, delivered quantity bounded by quantity, and a zero-total sale to be
`pending`.

**Decision:** Follow DOMAIN where it conflicts with legacy behavior. Go rejects
duplicate products and `delivered_quantity > quantity` with `422`, and derives
`pending` for a zero total with zero paid.

**Consequences:** These are documented divergences in
[MIGRATION.md](MIGRATION.md#known-legacy-quirks). Clients must rely on DOMAIN,
not the legacy implementation, for sale validation and status.

## ADR-0021 — SQLite offline mode is visible and unencrypted in v1

**Status:** Accepted

**Context:** SQLite now holds business data, including client PII and product
pricing, not just auth rows. The database file is not encrypted at rest.
`DB_MODE=auto` can silently fall back from PostgreSQL to SQLite, so an operator
could be working on the local device without noticing.

**Decision:** Keep `DB_MODE=auto` as the default for v1. The frontend shows a
persistent banner on the login screen and on every authenticated screen
whenever the active backend is SQLite; the backend already exposes it through
`GET /healthz` (`database=sqlite`). SQLite permissions stay owner-only
(`0600` file, `0700` directory). Cross-device sync remains out of scope.
Encryption at rest is deferred.

**Consequences:** Operators must protect the device and any backups of the
SQLite file. The fallback is no longer silent because the UI surfaces it.
Encryption at rest and sync each require a future ADR before implementation.
