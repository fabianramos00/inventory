# AGENTS.md — Inventory Desktop

This file is the root rulebook for every agent working in this repository.
Read it before touching anything.

## Project

**Inventory Desktop** is a Wails v2 desktop application for a hardware store
(ferretería). It is based on two legacy web projects that are **read-only
references**:

- `../inventory-api` — FastAPI + PostgreSQL API (the contract source of truth).
- `../inventory-app` — React 19 + Vite web client (the UI source of truth).

The desktop app embeds a React frontend and runs a Go REST API on loopback.
The Go API mirrors the legacy FastAPI `/api/v1` contract. PostgreSQL remains
the database.

Current phase: **Phase 1 — documentation, rules, and agents.** No application
code exists yet. Do not scaffold code until the user starts Phase 2.

## Stack

| Layer | Technology |
|---|---|
| Desktop shell | Wails v2 (Go) |
| Backend | Go 1.26, standard library `net/http` |
| Database | PostgreSQL 17, `pgx/v5`, `goose` migrations |
| Auth | JWT access + refresh tokens, bcrypt |
| Money | `shopspring/decimal` in Go, `NUMERIC(12,2)` in PostgreSQL |
| Frontend | React 19, TypeScript 6, Vite 8, pnpm |
| Data fetching | TanStack Query v5, axios |
| Forms | react-hook-form, zod |
| Styling | CSS Modules and design tokens |
| Testing | Go `testing`, Vitest, Playwright |

## Architecture invariants

- Wails embeds the frontend; the Go backend runs an HTTP server bound to
  `127.0.0.1` on a dynamic port.
- The React app calls the Go API through axios, not through Wails bindings.
- Only the base URL is delivered through a Wails binding (`GetAPIBaseURL`).
  In browser dev mode, `VITE_API_URL` is the fallback.
- The API prefix is `/api/v1` and must match `docs/API-CONTRACT.md`.
- PostgreSQL is the only persistence layer. No SQLite, no browser storage of
  business data.
- Migrations are versioned SQL files run by `goose`.
- The loopback server must never bind to `0.0.0.0`.
- Tokens are sent as `Authorization: Bearer <access_token>`.

## Locale, money, and time

- Locale: `es-CO`.
- Currency: Colombian peso (COP), displayed as `$ 1.234,56`.
- Dates displayed as `DD/MM/YYYY`.
- Timestamps stored in UTC and serialized as RFC 3339.
- Dashboard day grouping uses `America/Bogota`.
- All UI copy is Spanish. Code, comments, commits, and docs are English.

## Repository layout

Current:

```text
AGENTS.md
README.md
opencode.json
.opencode/agents/
docs/
```

Target after Phase 2:

```text
main.go, app.go, wails.json
internal/
  httpapi/       # routing, handlers, middleware
  service/       # business rules and transactions
  repository/    # SQL and persistence
  model/         # domain structs and DB mapping
  auth/          # JWT, passwords, refresh tokens
  config/        # env and app config
  db/            # pool, goose runner
migrations/
frontend/        # copied React app
docs/
```

## Roles and permissions

Developer roles are the only roles allowed to write code or commit:
`frontend`, `backend`, `devops`, `qa`.

| Role | Edits | Commits | Scope |
|---|---|---|---|
| `manager` | Markdown only | No | `docs/work/manager/**` |
| `product` | Markdown only | No | `docs/work/product/**` |
| `ux-designer` | Markdown only | No | `docs/work/ux/**` |
| `frontend` | Code + docs | Yes | `frontend/**` |
| `backend` | Code + docs | Yes | `main.go`, `app.go`, `internal/**`, `migrations/**`, `go.mod`, `go.sum` |
| `database` | Markdown only | No | `docs/work/database/**` |
| `qa` | Tests only | Yes | `frontend/**/*.test.*`, `frontend/e2e/**`, `internal/**/*_test.go`, `tests/**`, `testdata/**` |
| `security` | Markdown only | No | `docs/work/security/**` |
| `devops` | Build/CI + docs | Yes | `wails.json`, `build/**`, `.github/**`, `scripts/**`, `Makefile` |

Non-developer agents produce reports and specifications under their
`docs/work/<role>/` folder. They never modify application code, migrations,
build scripts, or configuration.

Cross-boundary changes require the owning agent to make the change. If a
backend change needs a frontend adjustment, backend reports the required
change to `frontend`; it does not edit `frontend/`.

## Commit policy

- **No agent commits unless the user explicitly asks.**
- Only `frontend`, `backend`, `devops`, and `qa` may commit.
- `qa` commits test code only.
- `devops` commits build, CI, packaging, and release changes only.
- `git push` is always denied.
- Commit only files inside the agent's owned scope.
- Before committing, run the relevant checks: formatter, linter, tests, build.
- Never commit secrets, `.env` files, tokens, database dumps, or credentials.
- Commit message format: `type(scope): imperative summary`
  (`feat(frontend): add product filters`, `fix(api): reject oversell`).
- Do not amend, rebase, force-push, or create empty commits.

## Agent context requirement

Every file in `.opencode/agents/` must contain role-specific context, not a
generic job description. At minimum:

1. Project context: stack, phase, locale, and legacy references.
2. Owned paths and explicit read-only areas.
3. Required reading with exact file paths.
4. Domain context relevant to the role (business rules, schema, threat
   model, or design system as applicable).
5. Technical context relevant to the role (commands, patterns, known quirks,
   or build constraints).
6. Workflow and handoffs.
7. Permission and commit rules.
8. A concrete output format.

If an agent lacks the context it needs, stop and update its definition before
working.

## Read order

Every agent reads only what its role needs:

1. `AGENTS.md` (this file).
2. `docs/ARCHITECTURE.md` for system boundaries.
3. `docs/DOMAIN.md` for business rules.
4. `docs/API-CONTRACT.md` for endpoint behavior.
5. Its role document: `docs/FRONTEND.md`, `docs/BACKEND.md`,
   `docs/TESTING.md`, `docs/SECURITY.md`, or `docs/RELEASE.md`.
6. Legacy source files relevant to the task.

Do not bulk-read `node_modules`, `.venv`, build output, or the entire legacy
repositories.

## Workflow

1. `manager` frames the task, identifies affected modules, and delegates.
2. `product` defines requirements and acceptance criteria (when behavior
   changes).
3. `ux-designer` produces the interaction and visual spec (when UI changes).
4. `backend` changes the Go API and database access.
5. `frontend` changes the React app.
6. `qa` verifies behavior, adds tests, and checks API parity.
7. `security` reviews auth, data handling, and desktop boundaries.
8. `devops` updates build, packaging, and release steps.
9. `manager` confirms the Definition of Done.

Database schema changes are proposed by `database` and implemented by
`backend`. Security findings are implemented by the owning developer.

## Definition of Done

A task is done only when:

- Behavior matches `docs/API-CONTRACT.md` and `docs/DOMAIN.md`.
- Money uses decimal arithmetic end to end; no float arithmetic in Go.
- Inputs are validated at the trust boundary.
- Errors use the documented shape and do not leak internals.
- Accessibility basics are preserved (labels, focus, keyboard, contrast).
- Tests cover the changed behavior.
- Formatting, linting, tests, and build pass.
- Relevant docs are updated in the same change.

## Engineering rules

- Prefer the standard library and installed dependencies over new packages.
- No dependency without a documented reason in `docs/DECISIONS.md`.
- No speculative abstractions, factories, or configuration for a single use.
- No comments unless they explain why, not what.
- Delete dead code instead of leaving it for later.
- Keep the shortest working diff that solves the root cause.
- Never simplify away validation, security, data-loss protection, or
  accessibility.
- Mark deliberate shortcuts with a `ponytail:` comment naming the ceiling and
  the upgrade path.

## Testing rules

- Go: table-driven unit tests for services; `httptest` for handlers;
  integration tests against a real PostgreSQL database.
- Frontend: Vitest for hooks, utilities, and components; Playwright for
  critical flows.
- API parity: run the same request fixture against FastAPI and Go and compare
  status, shape, and values. Differences must be documented.
- New non-trivial logic leaves at least one runnable check behind.

## Security rules

- Bind the API to loopback only and use a random port.
- Require bearer auth on every endpoint except login, refresh, logout, and
  health.
- Never log passwords, tokens, secrets, or full request bodies.
- Validate all input server-side; never trust the frontend.
- Keep secrets out of source control; use environment variables or the OS
  config directory.
- Treat `localStorage` token storage as a known risk and document mitigations.
- Follow `docs/SECURITY.md` for audits and severity reporting.

## Documentation rules

- `docs/` is the source of truth for this project.
- If code and docs disagree, stop and resolve the conflict explicitly.
- Update the affected docs in the same change as the code.
- Do not duplicate large sections; link to the owning document.
- Non-developer deliverables go under `docs/work/<role>/`.
