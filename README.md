# Inventory Desktop

Desktop inventory management for a Colombian hardware store (ferretería).
The application is a Wails v2 shell with an embedded React frontend and a Go
REST API backed by PostgreSQL.

## Status

**Phase 2 — walking skeleton + auth + hexagonal refactor.** Implemented:
Wails shell, embedded React login/home, loopback Go API, PostgreSQL and SQLite
persistence behind ports/adapters, JWT login, refresh rotation, logout, and
the seed command. Inventory, products, clients, providers, sales, orders,
dashboard, and user management are Phase 3. SQLite currently contains only
the auth tables; full offline business data is Phase 3 work.

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

## Target stack

| Layer | Technology |
|---|---|
| Shell | Wails v2 / Go 1.26 |
| API | `net/http`, GORM, `goose` |
| Database | PostgreSQL 17 primary; GORM outbound adapters; SQLite offline fallback |
| UI | React 19, TypeScript 6, Vite 8 |
| Data | TanStack Query v5, axios |
| Forms | react-hook-form, zod |
| Tests | Go `testing`, Vitest, Playwright |

## Repository map

```text
AGENTS.md                  rules for every agent
opencode.json              opencode config and legacy references
.opencode/agents/          specialized agents
docs/                      architecture, domain, API, role docs
docs/work/                 non-developer reports and specifications
internal/domain/           entities and domain errors
internal/ports/inbound/    use-case interfaces
internal/ports/outbound/   repository and security interfaces
internal/application/      use cases
internal/adapters/inbound/httpapi/ inbound HTTP adapter
internal/adapters/outbound/persistence/gormstore/ GORM outbound adapter
internal/adapters/outbound/security/ JWT, bcrypt, refresh tokens
```

Implemented in Phase 2: `main.go`, `app.go`, `main_bindings.go`,
`wails.json`, `internal/`, `migrations/`, `cmd/`, `frontend/`, and the
development `docker-compose.yml`.

## Prerequisites

- Go 1.26+
- Wails CLI 2.16+
- Node 22+ and pnpm 12+
- Docker (development PostgreSQL 17 on `127.0.0.1:5433`)
- PostgreSQL 17 (recommended primary; optional for auth-only offline mode)

Wails also needs the platform webview dependencies. On Linux that is
WebKitGTK 4.1 (`libwebkit2gtk-4.1-dev`); the Makefile passes
`-tags webkit2_41`. On Windows it is WebView2; on macOS it is the system
WebKit.

## Commands

```bash
# Development database (PostgreSQL 17 on 127.0.0.1:5433)
make db-up
make db-down

# Seed the first superuser
SEED_EMAIL=admin@example.com SEED_PASSWORD='change-me-now' make seed

# Run the desktop app (Linux passes -tags webkit2_41)
make dev

# Force the offline/local backend
DB_MODE=sqlite make dev

# Checks
make test
make lint
go test -race ./...
INVENTORY_TEST_DATABASE_URL='postgresql://inventory:inventory@localhost:5433/inventory_desktop_test' \
  go test -v ./internal/adapters/inbound/httpapi
cd frontend && pnpm test:e2e

# Production build
make build
```

`make dev` and `make build` use `-tags webkit2_41` on Linux. The browser
fallback API base is `http://127.0.0.1:8787/api/v1`; run
`go run ./cmd/server` for headless dev or Playwright.

## Using the agents

`manager` is the default agent. It frames work and delegates to the
specialists. Developer roles (`frontend`, `backend`, `devops`, `qa`) are the
only roles allowed to write code or commit. Non-developer roles write only
Markdown under `docs/work/`.

After changing `opencode.json` or any file in `.opencode/agents/`, restart
opencode so the configuration reloads.

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
| [docs/MIGRATION.md](docs/MIGRATION.md) | FastAPI → Go migration plan |
| [docs/SYNC.md](docs/SYNC.md) | Future offline/sync design constraints |

## Legacy references

- `../inventory-api` — FastAPI, SQLAlchemy, Alembic, PostgreSQL.
- `../inventory-app` — React 19, Vite, TanStack Query, CSS Modules.

Both are read-only. Changes belong in this repository.
