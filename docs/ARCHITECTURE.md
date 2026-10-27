# Architecture

## Context

Inventory Desktop packages the existing GestiónStock web UI and replaces the
hosted FastAPI service with a Go service that runs inside the desktop app.
PostgreSQL remains the primary database; a local SQLite database is used when
PostgreSQL is unavailable.

Goals:

- Reuse the React application and its data-fetching patterns.
- Preserve the `/api/v1` contract used by the current frontend.
- Run without a remote API server.
- Keep the backend on loopback and the database external to the UI process.
- Support standalone/offline operation through an automatic SQLite fallback.

Non-goals:

- Cross-device synchronization (future work, not implemented).
- Multi-tenant hosting of the desktop app.
- Rewriting the UI from scratch.

## Runtime model

```text
Wails process
├── Go main goroutine
│   ├── load config
│   ├── bootstrap stores (PostgreSQL or SQLite)
│   ├── run backend-specific goose migrations
│   ├── start HTTP server on 127.0.0.1:<ephemeral>
│   └── run Wails event loop
├── HTTP handler goroutines
└── WebView process
    └── React app → axios → http://127.0.0.1:<port>/api/v1
```

There is exactly one Go process and one React app per running instance. The
PostgreSQL server is a separate process, local or remote. SQLite runs
in-process.

## Startup sequence

1. Wails starts `main.go`.
2. Config loads from, in order: environment variables, the OS config file,
   built-in defaults.
3. `DB_MODE` selects the backend:
   - `postgres`: connect to PostgreSQL; failure is fatal.
   - `sqlite`: open the local database directly.
   - `auto` (default): probe PostgreSQL with a 3-second timeout. On failure,
     log a warning and open SQLite.
4. Backend-specific `goose` migrations run when `AUTO_MIGRATE=true` (default).
   A failed migration stops startup and leaves the database unchanged.
5. The HTTP server listens on `127.0.0.1:0`; the OS assigns a free port.
6. The server URL is stored for the `GetAPIBaseURL` binding.
7. Wails opens the window and serves the embedded frontend.
8. React bootstraps the API base URL through the binding, then renders.

Shutdown closes the HTTP server, waits for in-flight requests, and closes the
active store. Wails window close is the trigger.

## API boundary

- Prefix: `/api/v1`.
- Transport: HTTP/JSON over loopback.
- Authentication: `Authorization: Bearer <access_token>`.
- Public endpoints: login, refresh, logout, and `GET /healthz`.
- `GET /healthz` returns `{"status":"ok","database":"postgres|sqlite"}`; it
  exposes the active backend and no business data.
- Access tokens carry `iss=inventory-desktop/<backend>`, `iat`, and a random
  `jti`. Verification requires the expected issuer and HS256, so tokens do not
  cross the PostgreSQL and SQLite datasets.
- The server never binds `0.0.0.0`.
- CORS always allows the Wails webview origin. The Vite dev origin and the
  Playwright preview origin (`:5173`, `:4173`) are allowed only when dev mode
  is enabled (`APP_ENV=development` or `ALLOW_DEV_ORIGINS=true`).
- The API is the only way React reads or writes business data. Wails bindings
  are limited to the base URL and window lifecycle.

A loopback server is reachable by other local processes. The threat model
therefore relies on bearer authentication, a random port, and not exposing
the port in logs. See [SECURITY.md](SECURITY.md).

## Frontend bootstrap

The legacy app reads `import.meta.env.VITE_API_URL` at module load time. The
desktop app cannot use a build-time port, so bootstrap changes:

```text
main.tsx
  └── await GetAPIBaseURL()            # Wails binding
        └── setApiBaseUrl(url)         # axios.defaults.baseURL
              └── render(<App />)
```

- In the Wails runtime, `GetAPIBaseURL()` is the source.
- In browser dev, if the binding is unavailable, use `VITE_API_URL`, then
  `http://127.0.0.1:8787/api/v1`.
- The API base URL must be set before the first request. No component may
  import axios and fire a request during module evaluation.

Routing uses `HashRouter` in the desktop build. The Wails asset server does
not need SPA history fallback, and deep links keep working after reloads.
Browser dev may keep `BrowserRouter` behind a compile-time switch; tests must
cover the desktop router.

## Database

Persistence sits behind ports/adapters. `internal/domain` holds entities and
domain errors, `internal/ports` holds consumer-facing repository interfaces,
`internal/service` depends only on those interfaces, and backend adapters live
under `internal/adapters/persistence/{postgres,sqlite}`. See
[DECISIONS.md](DECISIONS.md) ADR-0011.

PostgreSQL:

- PostgreSQL 17, driver `pgx/v5` with `pgxpool`.
- Migrations in `migrations/postgres/`, embedded in the binary.
- Schema is a faithful translation of the legacy Alembic schema.
- Every write that spans rows uses a transaction.
- Stock mutations lock product rows (`SELECT ... FOR UPDATE`) before changing
  quantities.
- Money is `NUMERIC(12,2)`; Go reads it into `shopspring/decimal`.

SQLite:

- Driver `modernc.org/sqlite` (pure Go, no CGO).
- Opened through `database/sql` with foreign keys, a 5-second busy timeout,
  and WAL enabled. The parent directory is always tightened to `0700`, a
  symlink at the database path is rejected, and the database file plus any
  existing `-wal`/`-shm` sidecars are best-effort set to `0600`.
- Migrations in `migrations/sqlite/`, embedded in the binary.
- Only the auth tables exist today. The remaining business tables are Phase 3
  work; full-schema parity is not yet available offline.
- Booleans are stored as `0`/`1`; timestamps are stored as fixed-width UTC
  text so lexical comparisons stay correct, and parsing accepts RFC 3339
  with variable fractional seconds.

The default SQLite location is
`<OS config dir>/inventory-desktop/data/inventory.sqlite`; `SQLITE_PATH`
overrides it. The development environment provides PostgreSQL through
`docker-compose.yml`; the production installer documents the DSN requirement.
Bundling a PostgreSQL server is deferred.

Future sync is **not implemented**. Supporting it later will require stable
sync identifiers, reliable timestamps, tombstones, and an outbox for local
changes. See [SYNC.md](SYNC.md) for the constraints that future work must
respect.

## Configuration

Precedence: environment > OS config file > defaults.

| Key | Default | Notes |
|---|---|---|
| `DB_MODE` | `auto` | `auto`, `postgres`, or `sqlite` |
| `DATABASE_URL` | `postgresql://inventory:inventory@localhost:5433/inventory_desktop` | Dev default matching compose, which exposes PostgreSQL 17 on port `5433`; required in production |
| `SQLITE_PATH` | `<OS config dir>/inventory-desktop/data/inventory.sqlite` | Local database used by `sqlite` mode and by `auto` fallback |
| `SECRET_KEY` | generated on first run | At least 32 bytes; known placeholders are rejected. Persisted with owner-only permissions |
| `ACCESS_TOKEN_MINUTES` | `30` | `ACCESS_TOKEN_EXPIRE_MINUTES` is also accepted |
| `REFRESH_TOKEN_DAYS` | `7` | `REFRESH_TOKEN_EXPIRE_DAYS` is also accepted |
| `BCRYPT_ROUNDS` | `12` | Lower only in tests |
| `AUTO_MIGRATE` | `true` | Run goose at startup |
| `APP_ENV` | unset | `development` enables dev CORS origins |
| `ALLOW_DEV_ORIGINS` | `false` | Enable Vite/Playwright CORS origins explicitly |
| `LOG_LEVEL` | `info` | `slog` level |
| `VITE_API_URL` | unset | Browser dev only |

The config file lives in the OS config directory, never in the repository.
Secrets are never logged.

## Target folder map

```text
main.go                  Wails entry, server lifecycle
app.go                   Wails application bindings
wails.json               Wails project config
internal/
  httpapi/               router, handlers, middleware (inbound adapter)
  service/               business rules and transactions
  ports/                 repository interfaces consumed by services
  domain/                entities and domain errors
  adapters/
    persistence/
      postgres/          pgx repository implementations
      sqlite/            database/sql repository implementations
  auth/                  JWT, bcrypt, refresh tokens
  config/                env and file config
  db/                    backend-specific open and goose runners
  bootstrap/             backend selection and store wiring
migrations/
  postgres/              PostgreSQL goose SQL files
  sqlite/                SQLite goose SQL files
frontend/                copied React application
```

Dependency direction: `httpapi → service → ports ← adapters → db`. The domain
package is shared by all layers. `service` never imports pgx, SQLite, or
concrete adapters. `httpapi` never issues SQL.

## Failure modes

| Failure | Behavior |
|---|---|
| PostgreSQL unreachable (`DB_MODE=postgres`) | Startup aborts; app does not open |
| PostgreSQL unreachable (`DB_MODE=auto`) | Warning logged; SQLite is opened |
| SQLite open or migration failure | Startup aborts |
| Migration failure | Startup aborts; database left at the last successful version |
| Port unavailable | Retry with a new ephemeral port |
| Frontend bootstrap failure | Error screen; no unauthenticated requests are fired |
| Expired access token | Frontend refresh flow; logout on refresh failure |
| Refresh token reuse | Token rotation invalidates the old token |

## Decisions

Architecture decisions and their rationale live in
[DECISIONS.md](DECISIONS.md).
