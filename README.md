# Inventory Desktop

Desktop inventory management for a Colombian hardware store (ferretería).
The application is a Wails v2 shell with an embedded React frontend and a Go
REST API backed by PostgreSQL.

## Status

**Phase 1 — documentation, rules, and agents.** No application code exists
yet. Phase 2 starts the Wails scaffold, the Go API, and the frontend copy.

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
- PostgreSQL is required; the development database runs in Docker.

## Target stack

| Layer | Technology |
|---|---|
| Shell | Wails v2 / Go 1.26 |
| API | `net/http`, `pgx/v5`, `goose` |
| Database | PostgreSQL 17 |
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
```

Phase 2 adds `main.go`, `app.go`, `wails.json`, `internal/`, `migrations/`,
and `frontend/`.

## Prerequisites

- Go 1.26+
- Wails CLI 2.16+
- Node 22+ and pnpm 12+
- Docker (development PostgreSQL)
- PostgreSQL 17 (installed database or server for production)

Wails also needs the platform webview dependencies. On Linux that is
WebKitGTK; on Windows it is WebView2; on macOS it is the system WebKit.

## Planned commands

These commands become available in Phase 2:

```bash
# Development
wails dev

# Migrations (inside the Go app or via the goose CLI)
go run ./cmd/migrate up

# Go checks
go test ./...
go vet ./...

# Frontend checks
cd frontend
pnpm install
pnpm lint
pnpm test
pnpm build

# Desktop build
wails build
```

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

## Legacy references

- `../inventory-api` — FastAPI, SQLAlchemy, Alembic, PostgreSQL.
- `../inventory-app` — React 19, Vite, TanStack Query, CSS Modules.

Both are read-only. Changes belong in this repository.
