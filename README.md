# Inventory Desktop

Desktop inventory management for a Colombian hardware store (ferretería).
The application is a Wails v2 shell with an embedded React frontend and a Go
REST API backed by PostgreSQL.

## Why desktop

The legacy web app is split into a React client (`../inventory-app`) and a
FastAPI service (`../inventory-api`). This project keeps the React UI and
replaces the hosted API with a Go service that runs on the same machine as
the desktop app. The result is a single installable application for the
store counter, with no dependency on a remote web server.

## Architecture

```text
┌────────────────────────────── Wails desktop process ──────────────────────────────┐
│                                                                                   │
│  React 19 (WebView)  ── HTTP/JSON ──►  Go API  127.0.0.1:<random>  ──►  PostgreSQL │
│         │                                │                                        │
│         └──── GetAPIBaseURL() ◄──────────┘                                        │
│                                                                                   │
└───────────────────────────────────────────────────────────────────────────────────┘
```

- The API mirrors the legacy FastAPI `/api/v1` contract.
- The React app is reused and keeps axios, TanStack Query, and existing tests.
- The API binds to loopback only; the port is chosen at startup.
- PostgreSQL is the primary database. `make dev` and `make seed` target the
  PostgreSQL 17 the app uses by default; the optional in-repo compose
  PostgreSQL 17 is started with `make db-up` (see
  [Database ports](#database-ports)).
- `DB_MODE=auto` falls back to a local SQLite database when PostgreSQL is
  unreachable. `DB_MODE=postgres` fails hard instead, and `DB_MODE=sqlite`
  forces offline mode.

## Prerequisites

- Go 1.26+
- Wails CLI 2.16+
- Node 22+ and pnpm 12+
- PostgreSQL 17 running on `127.0.0.1:5432` for `make dev`/`make seed`
- Docker for the optional compose PostgreSQL 17 (`make db-up`)

Wails also needs the platform webview dependencies. On Linux that is
WebKitGTK 4.1 (`libwebkit2gtk-4.1-dev`); the Makefile passes
`-tags webkit2_41`. On Windows it is WebView2; on macOS it is the system
WebKit.

## Database ports

- `5432` is the PostgreSQL 17 the app uses by default.
- `5433` belongs only to the optional in-repo container started by `make db-up`.
  Override it with `POSTGRES_HOST_PORT` (default `5433`).
- CI publishes its own container on `5433`; that port is CI-only and unrelated
  to both of the above.

## Quickstart

`make dev`, `make seed`, and `make seed-catalogs` use the PostgreSQL 17 the app
targets by default (`DB_MODE=postgres`, database `inventory_desktop`, user
`inventory`). The DSN is the app's built-in default; set `DATABASE_URL` to
change it. The app uses the non-superuser project role `inventory` on
`inventory_desktop`; the admin role `postgres`/`postgres` is only for
administration. On first start an init SQL script creates the role and
database. `make dev` does not start Docker. The local cluster stores data in
the fresh `postgresql_postgres_data_pg17` volume; the previous PG16 volume was
deleted.

```bash
SEED_EMAIL=admin@example.com SEED_PASSWORD='change-me-now' make seed
make seed-catalogs
make dev
```

`make seed-catalogs` loads the default categories, brands, materials, and measurement units; it is idempotent and safe to re-run.

The optional compose PostgreSQL 17 remains available:

```bash
make db-up
DB_MODE=postgres \
DATABASE_URL="postgresql://inventory:inventory@localhost:${POSTGRES_HOST_PORT:-5433}/inventory_desktop" \
SEED_EMAIL=admin@example.com SEED_PASSWORD='change-me-now' make seed
make db-down
```

`make build` produces a release binary. `make dev`, `make build`,
`make seed`, and `make seed-catalogs` source `.env` when present; `make dev`
and `make build` use `-tags webkit2_41` on Linux. See
[docs/RELEASE.md](docs/RELEASE.md) for platform builds, signing, migrations,
and release checks.

## Configuration

`make dev`, `make seed`, `make seed-catalogs`, and `make build` source `.env`
when it exists. For those targets the precedence is `.env` > process
environment > built-in default. Inside the app it is environment > OS config
file (`<OS config dir>/inventory-desktop/config.env`) > built-in defaults.
Empty values are ignored, so a blank `DATABASE_URL` keeps the built-in
default.

Integration suites read `INVENTORY_TEST_DATABASE_URL` and skip the PostgreSQL
cases when it is unset.

## Documentation

| Document | Purpose |
|---|---|
| [AGENTS.md](AGENTS.md) | Rules, roles, boundaries, Definition of Done |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Runtime, process model, boundaries |
| [docs/DOMAIN.md](docs/DOMAIN.md) | Entities, statuses, business invariants |
| [docs/API-CONTRACT.md](docs/API-CONTRACT.md) | Endpoint contract and parity rules |
| [docs/DECISIONS.md](docs/DECISIONS.md) | Architecture decisions |
| [docs/FRONTEND.md](docs/FRONTEND.md) | React conventions |
| [docs/BACKEND.md](docs/BACKEND.md) | Go conventions |
| [docs/TESTING.md](docs/TESTING.md) | Test strategy and parity checks |
| [docs/SECURITY.md](docs/SECURITY.md) | Threat model and security rules |
| [docs/RELEASE.md](docs/RELEASE.md) | Build, packaging, release |
| [docs/DESIGN-SYSTEM.md](docs/DESIGN-SYSTEM.md) | Visual system |
| [docs/SYNC.md](docs/SYNC.md) | Future offline/sync design constraints |

## Legacy references

- `../inventory-api` — FastAPI, SQLAlchemy, Alembic, PostgreSQL.
- `../inventory-app` — React 19, Vite, TanStack Query, CSS Modules.

Both are read-only. Changes belong in this repository.
