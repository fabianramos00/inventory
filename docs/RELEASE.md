# Release

## Prerequisites

| Tool | Version |
|---|---|
| Go | 1.26+ |
| Wails CLI | 2.16+ |
| Node | 22+ |
| pnpm | 12+ |
| Platform webview deps | WebView2 / WebKitGTK / system WebKit |
| PostgreSQL | 17 |

The target machine needs a reachable PostgreSQL 17 instance. The installer
does not bundle PostgreSQL.

## Versioning

- Semantic versioning: `MAJOR.MINOR.PATCH`.
- The version is stored in `wails.json` and the Go build info.
- Database schema version is tracked by goose, independent of the app version.
- Every release notes the app version, schema version, and minimum PostgreSQL
  version.

## Build

```bash
# Install frontend dependencies
cd frontend && pnpm install --frozen-lockfile && cd ..

# Checks
go test ./...
go vet ./...
cd frontend && pnpm lint && pnpm test && pnpm build && cd ..

# Build the desktop app
wails build
```

Platform builds:

```bash
wails build -platform linux/amd64
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

The pipeline must run, in order:

1. `go test ./...`
2. `go vet ./...`
3. `gofmt -l .` (must be empty)
4. `pnpm install --frozen-lockfile`
5. `pnpm lint`
6. `pnpm test`
7. `pnpm build`
8. `wails build` for the target platform
9. Contract parity suite when `LEGACY_API_URL` is available

A red step blocks the release. Do not skip tests to ship.

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
- There is no auto-update mechanism yet.
- There is no offline mode.
- Multi-station use requires a shared PostgreSQL server and a shared
  `SECRET_KEY`.
