# Release

## Prerequisites

| Tool | Version |
|---|---|
| Go | 1.26+ |
| Wails CLI | 2.16+ |
| Node | 22+ |
| pnpm | 12+ |
| Platform webview deps | WebView2 / WebKitGTK 4.1 / system WebKit |
| PostgreSQL | 17 (primary; SQLite fallback available) |

PostgreSQL is the primary database. The installer does not bundle it. With
`DB_MODE=auto` (default), the app falls back to local SQLite when PostgreSQL
is unreachable. `DB_MODE=postgres` fails hard; `DB_MODE=sqlite` forces the
local backend. SQLite contains the auth tables and the full business schema.
All business modules are available offline, including catalog, products,
clients, providers, supplier catalogs, sales, purchase orders, and the
dashboard.

On Linux, Wails must be built with `-tags webkit2_41`. Ubuntu 26.04 ships
WebKitGTK 4.1 (`libwebkit2gtk-4.1-dev`) only, so the 4.0 ABI is unavailable.
The development database is PostgreSQL 17 from `docker-compose.yml`, exposed
on port `5433`.

## Phase status

Phase 3 ships the walking skeleton, PostgreSQL and SQLite persistence behind
ports/adapters, authentication, the seed command, and every business module:
catalog, products, clients, providers, supplier catalogs, sales, purchase
orders, and the dashboard. The full SQLite business schema exists, so the
release supports both the PostgreSQL primary and the offline SQLite backend.

## Versioning

- Semantic versioning: `MAJOR.MINOR.PATCH`.
- The version is stored in `wails.json` and the Go build info.
- Database schema version is tracked by goose, independent of the app version.
- Every release notes the app version, schema version, and minimum PostgreSQL
  version.

## Build

```bash
# Development database
docker compose up -d --wait postgres

# Install frontend dependencies
cd frontend && pnpm install --frozen-lockfile && cd ..

# Checks
go test ./...
go vet ./...
gofmt -l .   # must be empty
cd frontend && pnpm lint && pnpm test && pnpm build && cd ..

# Build the desktop app (Linux; see the WebKitGTK note above)
wails build -tags webkit2_41
```

`wails build` does not require a running database. The app itself starts with
`DB_MODE=auto` and local SQLite if PostgreSQL is unavailable. Wails compiles a
temporary binary with the `bindings` build tag to generate `frontend/wailsjs`;
that entry point skips config loading, database connection, and migrations. The
database is required only when the built app runs, or for seed and parity
tests.

The `Makefile` wraps the same steps: `make dev`, `make build`, `make test`,
`make lint`, `make db-up`, `make db-down`, and `make seed`. To seed the offline
dataset, run `DB_MODE=sqlite SEED_EMAIL=... SEED_PASSWORD=... make seed`.
Local SQLite accounts are separate from PostgreSQL accounts until sync exists. The `scripts/dev.sh`
and `scripts/build.sh` wrappers source `.env` when it exists and call the
matching make target.

Platform builds:

```bash
wails build -platform linux/amd64 -tags webkit2_41
wails build -platform windows/amd64
wails build -platform darwin/universal
```

Artifacts land in `build/bin/`. Do not commit them.

Optional packaging:

- Windows: NSIS installer via the Wails build flag.
- macOS: `.app` bundle, signed and notarized.
- Linux: binary plus a `.desktop` file and icon; `.deb` or AppImage if the
  release process adds it.

## Configuration at install time

The app needs:

- `DATABASE_URL` for the store database.
- A writable OS config directory.
- A generated `SECRET_KEY` if one is not supplied.

Never ship a `.env` file, a DSN, or a secret in the installer. The first-run
screen or a documented pre-seed step supplies the DSN. A wrong DSN produces a
readable error and does not open the main window.

## Migrations

- Migrations run at startup when `AUTO_MIGRATE=true`.
- A failed migration aborts startup.
- Before a release that includes migrations:
  1. Back up the database.
  2. Restore the backup into a staging database.
  3. Run the new build against staging.
  4. Verify login, dashboard, inventory, sales, and orders.
- Forward-only: a downgrade requires a tested restore, not a hand-written
  down migration.

## Signing

| Platform | Requirement |
|---|---|
| Windows | Authenticode certificate; timestamp the signature |
| macOS | Developer ID, hardened runtime, notarization |
| Linux | Detached signature or published checksum |

Publish SHA-256 checksums with every artifact.

## CI

`.github/workflows/ci.yml` runs on every push and pull request. It pins Go
1.26, Node 24, and pnpm 12, and starts a PostgreSQL 17 service on port 5433
with the `inventory` and `inventory_desktop_test` databases. The Go steps run
with `INVENTORY_TEST_DATABASE_URL` pointed at the test database.

The workflow runs, in order:

1. `go test ./... -count=1`
2. `go test -race ./... -count=1`
3. `go vet ./...`
4. `gofmt -l internal cmd main.go app.go` (must be empty)
5. `govulncheck ./...`
6. `pnpm install --frozen-lockfile`
7. `pnpm lint`
8. `pnpm test`
9. `pnpm build`
10. `pnpm audit --audit-level=high`

A red step blocks the release. Do not skip tests to ship. `scripts/verify.sh`
(also `make verify`) runs the same local checks, adding `git diff --check`;
the PostgreSQL pass is skipped unless `INVENTORY_TEST_DATABASE_URL` is set.

Wails platform builds and the FastAPI-vs-Go contract parity suite when
`LEGACY_API_URL` is available remain release-time steps, not part of this
first workflow. Playwright E2E is a documented follow-up: add it once the
service/database setup is reliable in CI.

```bash
scripts/verify.sh
```

## Local release verification

Session 3 verification, 2026-10-29 (Linux amd64, WebKitGTK 4.1):

- Wails build: `pnpm install --frozen-lockfile && pnpm build` followed by
  `wails build -tags webkit2_41` succeeded and produced
  `build/bin/inventory-desktop`. The binary stays ignored and is not committed.
- Backup/restore: `pg_dump` of `inventory_desktop_test` from the
  `inventory-postgres-1` container restored into a throwaway
  `inventory_desktop_restore_test` database. The restored database had the
  same 16 public tables and matching representative rows (`users`,
  `products`, `goose_db_version`); the restore database was dropped afterwards.
- Full local suite: `scripts/verify.sh` with `INVENTORY_TEST_DATABASE_URL`
  pointed at `inventory_desktop_test` passed gofmt, `go vet`, Go unit tests,
  PostgreSQL tests, Vitest (197 tests), ESLint (warnings only), the frontend
  build, and `git diff --check`.

Blocked until credentials or other OS hosts are available:

- Windows and macOS Wails builds and installer packaging.
- Authenticode signing, Developer ID signing, and notarization.
- Published SHA-256 checksums and the manual smoke test on Windows and macOS.

## Release checklist

- [ ] Version bumped in `wails.json`.
- [ ] Docs updated, including `API-CONTRACT.md` and `DECISIONS.md`.
- [ ] Full Go and frontend test suites pass.
- [ ] Contract parity matrix shows 67/67 passed or documented differences.
- [ ] Database backup taken and restore tested.
- [ ] Migrations applied to a staging copy.
- [ ] Wails smoke test: launch, login, create sale, restart, data persists.
- [ ] Manual smoke test on each target OS.
- [ ] Installer and binary signed.
- [ ] SHA-256 checksums published.
- [ ] Release notes list user-visible changes, schema changes, and known
      issues.
- [ ] Security checklist in [SECURITY.md](SECURITY.md) has no open Critical
      or High findings.

## Rollback

1. Stop the app.
2. Restore the pre-release database backup.
3. Reinstall the previous signed build.
4. Confirm login and dashboard data.
5. Record the incident and the failed version in the release notes.

Never roll back only the binary if the schema changed.

## Known release constraints

- PostgreSQL must be provisioned separately.
- Linux builds require WebKitGTK 4.1 and the `webkit2_41` build tag. Distros
  that only provide WebKitGTK 4.0 are not supported by the current build.
- Development PostgreSQL runs on port `5433` and must not be reused for
  production data.
- There is no auto-update mechanism yet.
- SQLite is not encrypted at rest; the UI shows a persistent offline banner
  whenever the app runs on the local backend.
- Multi-station use requires a shared PostgreSQL server and a shared
  `SECRET_KEY`.
