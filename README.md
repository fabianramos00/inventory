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
- PostgreSQL is the primary database; the development one runs in Docker.
- `DB_MODE=auto` falls back to a local SQLite database when PostgreSQL is
  unreachable. `DB_MODE=postgres` and `DB_MODE=sqlite` force a backend.

## Prerequisites

- Go 1.26+
- Wails CLI 2.16+
- Node 22+ and pnpm 12+
- Docker (development PostgreSQL 17 on `127.0.0.1:5433`)
- PostgreSQL 17 (recommended primary; optional when running on local SQLite)

Wails also needs the platform webview dependencies. On Linux that is
WebKitGTK 4.1 (`libwebkit2gtk-4.1-dev`); the Makefile passes
`-tags webkit2_41`. On Windows it is WebView2; on macOS it is the system
WebKit.

## Quickstart

```bash
make db-up
SEED_EMAIL=admin@example.com SEED_PASSWORD='change-me-now' make seed
make seed-catalogs
make dev
```

`make seed-catalogs` loads the default categories, brands, materials, and measurement units; it is idempotent and safe to re-run.

`make build` produces a release binary. `make dev` and `make build` use
`-tags webkit2_41` on Linux and source `.env` when present. See
[docs/RELEASE.md](docs/RELEASE.md) for platform builds, signing, migrations,
and release checks.

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
