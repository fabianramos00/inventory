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
Standalone use is covered by the auth-only SQLite fallback in ADR-0012;
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

**Status:** Accepted

**Context:** The desktop app must remain usable when PostgreSQL is not
reachable, and a future sync feature needs a local source of truth. Shipping
CGO or an embedded PostgreSQL server adds packaging and operational cost.

**Decision:** Add `DB_MODE` with `auto` (default), `postgres`, and `sqlite`.
In `auto`, probe PostgreSQL with a 3-second timeout; on failure log a warning
and open SQLite through `modernc.org/sqlite` (pure Go). SQLite enables foreign
keys, a busy timeout, and WAL, stores data under the OS config directory, and
currently contains only the auth tables. PostgreSQL remains the full-feature
backend.

**Consequences:** Standalone installs can authenticate and persist sessions
without PostgreSQL. Business modules are not available offline until the full
SQLite schema and parity tests land in Phase 3. Automatic fallback can hide a
misconfigured DSN, so the warning is logged and `DB_MODE=postgres` is
available for deployments that must fail hard. Sync is not implemented; it
will require stable sync IDs, reliable timestamps, tombstones, and an outbox.

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
