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

## ADR-0003 — PostgreSQL remains the database

**Status:** Accepted

**Context:** The legacy schema, Alembic migrations, dashboard queries, and
business rules are built on PostgreSQL. SQLite would require translating the
schema and query semantics.

**Decision:** Keep PostgreSQL 17 and the existing schema. The desktop app
connects with a configurable `DATABASE_URL`.

**Consequences:** The installer requires a reachable PostgreSQL instance. The
development environment provides one through Docker. Offline use is not
supported in this phase. Bundling a server is deferred.

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
