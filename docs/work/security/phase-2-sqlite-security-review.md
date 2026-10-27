# Security review — Phase 2 SQLite fallback and local data at rest

**Scope:** `DB_MODE=auto`, `internal/db/sqlite.go`, the SQLite repositories,
`migrations/sqlite/00001_auth.sql`, offline provisioning through `cmd/seed`,
and `docs/SYNC.md`. Static review; no shell commands were run. This report
supplements `phase-2-auth-review.md` and does not re-count its F1–F20.

**Verdict:** No Critical or currently exploitable High finding in the current
auth-only SQLite slice. Five Medium findings (conditional, latent, or a Phase 3
gate) and three Low/hardening findings. The auth cryptography and SQL access
are sound. The risks are fallback transparency, filesystem handling,
cross-dataset token identity, and the Phase 3 decision to store business data
unencrypted.

## Summary

The SQLite fallback gets the basics right: DB file `0600`, parent directory
created `0700`, foreign keys and WAL enabled, bcrypt default 12, refresh
tokens hashed with SHA-256 and rotated transactionally, parameterized SQL.
SQLite currently contains only `users` and `refresh_tokens`; no customer PII,
costs, prices, or stock is stored locally yet.

The gaps are at the edges:

- `auto` is the default, the only fallback warning is on stderr, and no API or
  UI reports the active backend. A local process can force the probe to fail.
- The `0700` directory control is only applied on creation; an existing
  `data/` directory is not tightened, the DB is chmodded after `Ping`, and
  `-wal`/`-shm` are never checked.
- `SQLITE_PATH` is not validated for symlinks, regular files, ownership, or
  relative paths, and is interpolated into the DSN without escaping.
- Access tokens contain only `sub` and `exp`. SQLite and Postgres user IDs
  both start at 1, so a token minted offline can authenticate as a different
  Postgres user by ID. No protected routes exist yet, so this is latent.
- Phase 3 will put PII and pricing in the same unencrypted file. The docs
  acknowledge this but do not decide encryption, backup, or retention.

`DB_MODE=postgres` is a correct fail-hard control, but it is not the default
and is not required by the release docs.

## 1. Automatic fallback

`bootstrap.Open` switches on `DB_MODE` (`internal/bootstrap/database.go:27-45`).
`postgres` returns the connection error to `main.go`, which logs and exits
(`:29-30,47-53`; `main.go:28-32`). `sqlite` opens the file directly (`:31-32`).
`auto` probes Postgres for 3 seconds (`:18,34-36`), logs a warning, and opens
SQLite on any error (`:40-41`). The default is `auto`
(`internal/config/config.go:136-139`).

**Can an attacker force fallback?** Yes. A local process can bind
`127.0.0.1:5433` when Postgres is not listening and stall the probe until the
3-second timeout. The same fake listener can speak enough Postgres to be
trusted as the primary DB, receiving the DSN credentials and returning
attacker data (same trust gap as F6 in the auth review). Resource exhaustion,
network blackholing of a remote DSN, or stopping the local service also work.
The probe is `pgxpool.NewWithConfig` plus `Ping`
(`internal/db/postgres.go:15-28`); there is no TLS or server identity check.

**Impact:** the app runs on a separate auth dataset. With no local user, login
is impossible (DoS). With local users, it is a different identity set; the
local `SECRET_KEY` is unchanged (`internal/config/config.go:75-89`), so an
offline access token remains valid when Postgres returns (SQL-4). In Phase 3,
business writes would be stranded locally; there is no sync or migration
(`docs/SYNC.md:7-15`). A missing/corrupt local DB aborts startup
(`internal/db/sqlite.go:29-36,40-48`; `docs/ARCHITECTURE.md:200-201`).

**Is the warning sufficient?** No. `cfg.Logger()` writes to `os.Stderr`
(`internal/config/config.go:46-48`), invisible in a GUI release. `Stores.Backend`
is set (`internal/bootstrap/database.go:24,70,90`) but never read. `GET /healthz`
returns only `{"status":"ok"}` (`internal/httpapi/router.go:15-17`). There is
no UI banner, no status field, and no operator-visible event.

**Is `DB_MODE=postgres` adequate?** Technically yes: it never falls back and
`main.go` exits non-zero without a window. But it is opt-in. `docs/RELEASE.md:83-93`
does not require it, and `docs/RELEASE.md:168` incorrectly says “There is no
offline mode.” Recommended: make `postgres` the release default, or require an
explicit `ALLOW_OFFLINE_FALLBACK=true`; expose the backend in an authenticated
status endpoint and a persistent UI banner; persist the last backend and
require confirmation when it changes; emit a stable `db.fallback` event.

**Verification:** stop Postgres; `DB_MODE=postgres` exits 1 with no window;
`DB_MODE=auto` shows an offline banner and reports `"backend":"sqlite"`. A
fake listener on 5433 produces the same banner and never receives credentials.

## 2. SQLite file handling

**Path selection.** Default is
`<OS config dir>/inventory-desktop/data/inventory.sqlite`
(`internal/config/config.go:201-207`). `SQLITE_PATH` overrides it
(`:146-152`). There is no check that the path is absolute, cleaned, inside the
config tree, a regular file, owned by the current user, or not a symlink.
`filepath.Dir("inventory.sqlite")` is `"."`, so a relative override lands in
the process CWD.

**Parent directory `0700`.** `os.MkdirAll(dir, 0o700)` only affects missing
directories (`internal/db/sqlite.go:18`). An existing `data/` directory with
`0755`/`0777` is left as-is. The config loader does tighten an existing
directory (`internal/config/config.go:224-232`); `OpenSQLite` does not. The
config tests cover file/dir modes (`internal/config/config_test.go:90-97,136-143`);
there is no SQLite equivalent. A broad directory makes the DB and sidecars
reachable by other local OS users.

**File `0600`.** `os.Chmod(path, 0o600)` runs after `Ping`
(`internal/db/sqlite.go:29,33`). If the file is new, `Ping` creates it under
the process umask (typically `0644`), leaving a window before the chmod. If
`Ping` fails, the chmod never runs and a pre-existing mode remains. `os.Chmod`
follows symlinks. On Windows, `os.Chmod` only toggles the read-only attribute;
it does not set an ACL, so `0600` is a Unix-only claim (SQL-7).

**Symlink/path traversal.** `OpenSQLite` lacks the config loader’s `Lstat`,
symlink, and regular-file checks (`internal/config/config.go:210-222`). A
pre-created symlink at `SQLITE_PATH` can make the app open and chmod an
arbitrary file. This requires write access to the path (same user, a
world-writable directory, or a broad installer). If the app runs elevated, it
becomes an arbitrary-file chmod/corruption primitive.

**DSN injection.** The DSN is built with `fmt.Sprintf("file:%s?...", path)`
(`internal/db/sqlite.go:22`). A path containing `?`, `#`, `%`, or `&` changes
the URI. The `modernc.org/sqlite` driver interprets `mode`, `cache`,
`immutable`, and `_pragma`; `_pragma` values run as SQL text, and the driver
added `StrictPragmas` because a non-constant DSN can carry a second statement.
`x?mode=memory` can force an in-memory DB (data loss); a crafted `_pragma` can
run extra SQL before the chmod. Low risk while `SQLITE_PATH` is local config,
but a real injection point and a correctness bug for filenames containing `?`.

**Ownership.** No owner check. An unprivileged app fails closed if `Chmod`
fails on a foreign-owned file; a privileged app uses and chmods the file.
Check `Stat_t.Uid` against the current EUID and reject non-regular files.

**WAL/`-shm`.** WAL is enabled, so SQLite creates `-wal` and `-shm` beside the
DB. SQLite normally gives them the DB file’s mode, and the `0700` directory is
the real control, but the code does not create, chmod, or verify them. A broad
directory or a stale sidecar exposes them. After an unclean shutdown, `-wal`
persists recent page images (password hashes, refresh-token hashes) until the
next clean open/checkpoint. A backup that copies only `inventory.sqlite` and
not `-wal` can lose committed transactions or be inconsistent; SQLite treats
the WAL as persistent state.

**Recommendation:** pre-create the DB with `O_CREATE|O_EXCL|O_RDWR`, `0600`,
before `sql.Open` (or set umask `0077`); `Lstat` and reject symlinks and
non-regular files; verify the parent is a directory and chmod it to `0700` if
broad; chmod `-wal`/`-shm` if present; build the DSN with `net/url`; use
`fchmod` on the opened descriptor on Unix.

**Verification:** `chmod 0755 ~/.config/inventory-desktop/data`; start the app;
`stat -c '%a %n'` shows `700` for the dir and `600` for all three files. A
second OS user cannot read them. A symlinked `SQLITE_PATH` is refused.
`SQLITE_PATH='x?mode=memory'` does not create an in-memory DB.

## 3. Local data at rest

**Current content.** `users` stores `full_name`, `email`, and bcrypt
`hashed_password` (`migrations/sqlite/00001_auth.sql:4-11`); the seed uses
`cfg.BcryptRounds`, default 12 (`internal/config/config.go:21`,
`cmd/seed/main.go:69,92`). `refresh_tokens` stores only a SHA-256 hash of a
32-byte random token (`:13-19`; `internal/auth/refresh.go:10-20`), with a
unique index. A stolen DB does not yield usable refresh tokens. No customer
PII, costs, prices, or stock exists yet (`docs/ARCHITECTURE.md:127-129`,
`docs/MIGRATION.md:69-71`).

**Encryption.** None. The file is plaintext SQLite; this is acknowledged in
`docs/SYNC.md:64-70` and `AGENTS.md:243-244`. Full-disk encryption protects a
stolen disk but not a live same-user process, a copied backup, or a roaming
profile. Any process running as the same OS user can read the DB and the JWT
secret in `config.env`, regardless of `0600`/`0700`. That limitation should be
stated in the threat model.

**Backup exposure.** The DB and `config.env` share the `inventory-desktop`
tree. A directory backup captures both user hashes and the signing secret.
Risks: unencrypted USB/network/cloud copies, helpdesk copies, copying only the
DB without the WAL. `docs/RELEASE.md:95-105` requires a backup/restore
rehearsal but describes Postgres, not SQLite; `docs/SECURITY.md:101-108` says
backups are encrypted/access-controlled but does not name the SQLite files.
Document a SQLite procedure (stop the app or use the backup API, checkpoint
the WAL, copy all three files, encrypt, restrict, test restore) and add it to
the release checklist.

`PRAGMA secure_delete=ON` or a periodic `VACUUM` reduces recoverable deleted
rows (old password hashes, revoked token hashes). Hardening only.

**Multi-user OS accounts.** The default location is per-OS-user. Two staff on
one OS account share one DB and secret; two OS accounts get two separate DBs
and user sets, neither of which is the Postgres dataset. There is no sync or
migration. On Windows, `os.UserConfigDir()` is `%AppData%` (roaming); a roaming
profile can place the DB and secret on a server, export the hashes, and break
SQLite WAL (WAL is unsupported on network filesystems). See SQL-7.

## 4. Separate auth datasets

**Local vs Postgres.** The tables match in shape but not identity. SQLite
`users.id` is `AUTOINCREMENT` (`migrations/sqlite/00001_auth.sql:5`); Postgres
uses an integer sequence
(`migrations/postgres/00001_initial_schema.sql:555-575`). Both start at 1.
There is no `sync_id`, mapping table, or email reconciliation. `docs/SYNC.md:14-15`
correctly says not to assume the same user exists in both; the token format
does not enforce it.

**Offline provisioning and seed.** `cmd/seed` is the only path. It calls
`bootstrap.Open` (`cmd/seed/main.go:51`), so it honors `DB_MODE` and can
silently seed SQLite under `auto` when Postgres is down. `-superuser` creates a
local superuser (`:23,78,106`). The backend is logged by the bootstrap
(`internal/bootstrap/database.go:85`), but the final line only says
`created user <id> (<email>)` (`cmd/seed/main.go:84,112`), so the operator may
not notice which dataset was written. Every update forces `IsActive = true`
(`:98`), re-enabling disabled users (prior F10). The password can be passed via
`-password` (argv/`ps`) or `SEED_PASSWORD` (`:21-23`; prior F9). No in-app
user management, password change, disable, or local-user deletion exists.

**Refresh-token isolation.** Hashed tokens live only in the active backend
(`internal/adapters/persistence/sqlite/refresh_tokens.go:24-105`). A token
from SQLite is not found in Postgres, so refresh fails closed with 401
(`internal/service/auth.go:84-88`), and vice versa. Gaps: logout revokes only
the active DB, so a Postgres token issued before an offline logout remains
valid for up to 7 days; expired/revoked rows are never purged (growth only).

**Access-token isolation (SQL-4).** Claims are only `sub` and `exp`
(`internal/auth/jwt.go:22-29`); there is no `iss`, `aud`, backend, or
installation ID. `AuthenticateAccessToken` loads `sub` from the active store
(`internal/service/auth.go:130-145`). Both backends number users from 1. A
token minted offline for local user 1 is accepted by Postgres as Postgres user
1 if the same `SECRET_KEY` is used. The secret is per installation by default
(`docs/DECISIONS.md:135-148`), but `docs/RELEASE.md:169-170` requires a shared
secret for multi-station use, so tokens can cross stations whose IDs collide.
There are no protected business routes yet (`internal/httpapi/router.go:15-23`),
so this is latent, but it is a merge gate for the first protected route.

**Recommendation:** add and validate `iss` (installation/backend) and `aud`
(API) claims, or use a per-backend secret, or make `sub` a globally unique
`sync_id` UUID. Force re-login and clear tokens on a backend switch. If
multi-station shares a secret, `iss`/`aud` are mandatory.

**Verification:** mint an offline token for local user 1; restart against
Postgres where ID 1 is a different/superuser account; a protected endpoint
returns 401, not the other user’s data.

## 5. Future sync (`docs/SYNC.md`)

`docs/SYNC.md` is explicit that sync is not implemented and that an offline
session is a separate standalone dataset (`:7-15`). Its constraints are sound:
stable `sync_id` (UUIDv7), UTC `updated_at` plus a per-device sequence,
tombstones, an outbox written in the same transaction as the business write,
idempotent apply, stock as an append-only ledger rather than last-write-wins,
money as decimal text, per-entity conflict policy, device identity/cursor/
revocation, local offline auth, and “do not sync refresh tokens” (`:17-48`).
Its security section requires device authentication, TLS, certificate
validation, no PII payload logging, and acknowledges that a stolen device
exposes local data and cached tokens (`:64-70`). Encryption at rest, sync
endpoints, device registration, and conflict UI are correctly out of scope
(`:82-88`).

Gaps to close before implementation:

- Bind access tokens to the installation/backend (`iss`/`aud`); `SYNC.md` does
  not address the SQL-4 token confusion.
- Reconcile “sessions are per installation” (`:14-15`) with the shared
  `SECRET_KEY` requirement for multi-station (`docs/RELEASE.md:169-170`).
  Either share a secret and enforce `iss`/`aud`, or use per-install secrets
  and a separate sync credential.
- Specify device key storage (OS keyring/TPM), rotation, and revocation; TLS
  alone does not identify a device.
- Consider certificate/public-key pinning for a self-hosted sync server, and
  forbid plaintext fallback.
- The outbox is a durable copy of PII in an unencrypted DB; decide whether it
  is encrypted, minimized, and retention-limited.
- Add a rule for user/role changes and password resets across devices; never
  sync refresh tokens.
- Fix the docs contradiction: `docs/RELEASE.md:168` says “There is no offline
  mode,” while ADR-0012 and the code implement one.

## 6. Dependency risk (`modernc.org/sqlite`)

`go.mod` pins `modernc.org/sqlite v1.60.1` and `modernc.org/libc v1.78.0`
(`go.mod:11,52`). The driver is pure Go, no CGO. Upstream documents v1.60.1 as
bundling SQLite 3.53.4, which includes the 3.51.3 WAL-reset fix and the 3.53.4
journal-rollback fix; that is a positive.

Risk: v1.60.1’s own `go.mod` requires `modernc.org/libc v1.77.1`, and upstream
explicitly says downstream must pin the exact libc version it declares. The
project pins v1.78.0, an untested combination for a transpiled,
memory-safety-sensitive dependency. Pin v1.77.1 or move to a sqlite release
whose `go.mod` pins v1.78.0. Verify with
`go list -m modernc.org/sqlite modernc.org/libc`.

The driver parses the SQLite file, WAL, and SQL, so it is attack surface if an
attacker can replace the DB file (same-user process, backup restore, future
sync, removable media). Pure Go avoids native memory-safety bugs in the
wrapper, but the transpiled core and libc layer still have pointer arithmetic;
recent changelogs include fixed crashes, leaks, double-frees, and a TOCTOU.
Keep current and track modernc releases, not only upstream SQLite CVEs.

SQL injection is not an issue in the adapters: all statements use
placeholders, and the only concatenation is the static `userColumns`
(`internal/adapters/persistence/sqlite/users.go:13,26,31`;
`refresh_tokens.go:25-28,40-44,69-74,82-86,97-100`).

No `govulncheck`/`go list -m -u all` output exists in the repo. Please run and
share `govulncheck ./...`, `go list -m -u all`,
`go list -m modernc.org/sqlite modernc.org/libc`, and `go test ./...`.

## 7. Findings

### Real vulnerabilities and security gaps

| ID | Severity | Type | Location | Issue | Impact | Remediation | Verification |
|---|---|---|---|---|---|---|---|
| SQL-1 | **Medium** (conditional) | Vulnerability — integrity/availability | `internal/bootstrap/database.go:27-45`; `internal/config/config.go:136-139`; `internal/httpapi/router.go:15-17`; `main.go:28-32` | `auto` is the default; a local listener/stall or network disruption forces SQLite; the only warning is stderr and `Stores.Backend` is never surfaced. | Silent switch to a separate auth dataset; login DoS if unseeded; Phase 3 data divergence; offline access token later replayed against Postgres (SQL-4). | Default release to `postgres` or require explicit offline opt-in; surface backend in an authenticated status and a persistent UI banner; persist last backend; emit `db.fallback`. | Stop Postgres: `postgres` exits 1, no window; `auto` shows banner and status `backend=sqlite`; fake listener on 5433 never receives credentials. |
| SQL-2 | **Medium** (conditional) | Vulnerability — local credential exposure | `internal/db/sqlite.go:16-37`; `docs/ARCHITECTURE.md:121-126` | `MkdirAll` does not tighten an existing dir; DB chmod happens after `Ping`; `-wal`/`-shm` are not checked; symlinks are followed. | If `data/` is `0755` or the DB pre-exists broadly, other local users can read bcrypt hashes, refresh hashes, and WAL pages; failed `Ping` leaves the file at its old mode. | Stat/chmod the dir to `0700`; pre-create the DB `0600` before `sql.Open` or set umask `0077`; `Lstat` and reject symlinks/non-regular files; chmod sidecars. | `chmod 0755` the data dir; app start yields `700`/`600`; second OS user cannot read DB/WAL; failed open does not leave `0644`. |
| SQL-4 | **Medium** (latent High) | Vulnerability — auth boundary | `internal/auth/jwt.go:22-29`; `internal/service/auth.go:130-145`; `migrations/sqlite/00001_auth.sql:5`; `docs/RELEASE.md:169-170` | JWT has only `sub` and `exp`; SQLite/Postgres IDs both start at 1; multi-station shares a secret. | An offline access token can authenticate as a different Postgres user, potentially a superuser, for up to 30 minutes. No protected routes yet, so latent. | Add/validate `iss` and `aud`, use a per-backend secret, or use a global UUID `sub`; force re-login on backend switch. | Offline token for user 1 against Postgres user 1 (different account) must return 401. |
| SQL-7 | **Medium** (conditional) | Vulnerability — platform gap | `internal/db/sqlite.go:33`; `internal/config/config.go:193-207`; `docs/ARCHITECTURE.md:125-126` | On Windows `os.Chmod` does not set an ACL, and `os.UserConfigDir()` is roaming `%AppData%`; WAL is unsupported on network filesystems. | The documented `0600` owner-only control is not delivered on Windows; a roaming profile can copy the DB/secret to a server and corrupt/lock the WAL DB. | Store the DB under `%LocalAppData%`/`os.UserCacheDir`; apply an explicit ACL or document the installer requirement; detect network paths and fail/warn. | On Windows, `icacls` shows only the owner; DB path is under `%LocalAppData%`; no DB in the roaming share. |

### Hardening and Phase 3 gates

| ID | Severity | Type | Location | Issue | Impact | Remediation | Verification |
|---|---|---|---|---|---|---|---|
| SQL-3 | Low | Hardening | `internal/config/config.go:146-152,201-207`; `internal/db/sqlite.go:16-23,33` | `SQLITE_PATH` is not checked for absolute/clean path, symlink, regular file, or owner; the path is interpolated into the DSN unescaped. | A symlink can target an arbitrary file for chmod/corruption; a relative path lands in CWD; `?mode=memory`/`_pragma` can change the DB or execute SQL (requires local path control). | Require an absolute clean path; `Lstat`/`O_NOFOLLOW`; owner check; build the DSN with `net/url`; use `StrictPragmas`; pre-create `0600`. | Symlink path refused; relative path refused; `x?mode=memory` does not create an in-memory DB. |
| SQL-5 | Low | Hardening — operational | `cmd/seed/main.go:19-24,44-56,58-112`; `docs/SYNC.md:14-15` | Seed follows `auto`, can silently write SQLite; final line omits the backend; updates force `IsActive=true`; no local user lifecycle. | Operator seeds the wrong dataset or creates a local admin with a reused password; disabled users are re-enabled; offline and Postgres identities diverge. | Require `--backend` or refuse `auto` for seed; print backend in the success line; add `--active/--inactive`; document offline provisioning; plan identity mapping. | Seed with Postgres down and `auto` refuses or requires `--backend sqlite`; name-only seed leaves a disabled user disabled. |
| SQL-6 | **Medium** | Hardening — Phase 3 gate | `docs/ARCHITECTURE.md:121-136`; `docs/MIGRATION.md:69-71,127-132`; `docs/SYNC.md:64-70`; `docs/RELEASE.md:95-105` | Phase 3 plans customer PII, costs, prices, and stock in the unencrypted SQLite file; no SQLite backup/restore procedure or encryption decision. | A stolen disk, backup, roaming copy, or same-user process exposes PII and pricing; copying only the DB can lose WAL commits. | Decide encryption at rest (SQLCipher/OS-level), encrypt backups, use a non-roaming dir, document and test a WAL-aware backup; add SQLite file/sidecar items to `docs/SECURITY.md`. | Backup is unreadable without the key; restore rehearsal succeeds; `-wal`/`-shm` are absent or `600` after a clean shutdown. |
| SQL-8 | Low | Hardening — dependency | `go.mod:11,52`; upstream `modernc.org/sqlite` v1.60.1 `go.mod` | Project pins `modernc.org/libc v1.78.0`; sqlite v1.60.1 requires v1.77.1; no `govulncheck` output. | Untested libc/sqlite combination in a transpiled memory-safety-sensitive dependency; unknown advisories. | Pin libc v1.77.1 or upgrade sqlite to a release pinning v1.78.0; add `govulncheck` to CI; track modernc releases. | `go list -m` shows the matching pair; `govulncheck ./...` is clean; SQLite integration tests pass. |

## Positive controls

- Loopback-only binding and ephemeral port are enforced in `httpapi.NewServer`
  (`internal/httpapi/server.go:42-48`) and used by `main.go:42`.
- Auth middleware fails closed for every `/api/v1` path except login, refresh,
  logout, and health (`internal/httpapi/middleware.go:26-30,115-140`).
- Refresh tokens: 32 random bytes, SHA-256 at rest, unique index, transactional
  rotation, logout revocation (`internal/auth/refresh.go:10-20`;
  `refresh_tokens.go:24-105`; `migrations/sqlite/00001_auth.sql:13-19`).
- Bcrypt default cost 12; seed enforces 8–128 characters and 72 bytes
  (`internal/config/config.go:21`; `cmd/seed/main.go:35-43`).
- Parameterized SQL only in both SQLite adapters; the only concatenation is
  the static column list (`users.go:13,26,31`; `refresh_tokens.go:25-105`).
- SQLite: foreign keys, 5 s busy timeout, WAL, one connection to avoid
  `SQLITE_BUSY` (`internal/db/sqlite.go:22-28`).
- DB file `0600` and parent `0700` on creation (`internal/db/sqlite.go:18,33`);
  config dir `0700`/file `0600` with symlink and regular-file checks
  (`internal/config/config.go:210-235,261-281`).
- JWT algorithm is pinned to HS256 and expiry is validated
  (`internal/auth/jwt.go:31-46`).
- Secret is generated from 32 random bytes; known placeholders and secrets
  under 32 bytes are rejected (`internal/config/config.go:179-191,283-289`).
- SQLite currently has no business data, so the blast radius is auth-only.
- `modernc.org/sqlite` v1.60.1 bundles SQLite 3.53.4, past the 3.51.3
  WAL-reset fix and the 3.53.4 journal-rollback fix.
- `docs/SYNC.md` correctly rejects last-write-wins for stock and requires
  outbox, tombstones, idempotent apply, device auth, TLS, and no token sync.

## Accepted risks and hardening suggestions

- **Accepted:** `localStorage` token storage (ADR-0009); access tokens survive
  logout until expiry; refresh rotation does not revoke a token family (prior
  F13); same-user processes can read `config.env`, the DB, and the WAL.
- **Hardening:** `PRAGMA secure_delete=ON` or periodic `VACUUM`; `_defensive=1`
  in the DSN if the file may be tampered with (not a sandbox for hostile
  files); process umask `0077`; periodic purge of expired/revoked refresh
  tokens; avoid logging the SQLite path in release; add SQLite-specific tests
  (the current tests only cover the config file and dir modes).
- **Prior findings still relevant:** F3 (config file read permissions), F6
  (default DSN/no TLS), F9 (seed argv), F10 (seed reactivation), F11
  (`BCRYPT_ROUNDS=4` allowed in production).

## Open questions and handoffs

1. Is production one OS user per station or a shared OS account? This changes
   SQL-5/SQL-7 from conditional to likely.
2. Is `DB_MODE=postgres` set in the installer/production? If not, SQL-1 is a
   release gate.
3. Is `SECRET_KEY` shared across stations? If yes, SQL-4 is urgent; if not,
   how do multi-station sessions work (`docs/RELEASE.md:169-170`)?
4. What SQLite version is linked in the artifact? Confirm with
   `SELECT sqlite_version();` (expected 3.53.4 for v1.60.1).
5. Is `modernc.org/libc` v1.78.0 intentional? Run
   `go list -m modernc.org/sqlite modernc.org/libc`.
6. On Windows, is the profile roaming? Check `%AppData%` redirection and
   `icacls` on the DB directory.
7. Is there a tested SQLite backup/restore procedure that includes the WAL?
8. Run and share: `govulncheck ./...`, `go list -m -u all`,
   `go list -m modernc.org/sqlite modernc.org/libc`, `go test ./...`,
   `cd frontend && pnpm audit --audit-level=high`.
9. Confirm the release binary has no console (so the stderr fallback warning
   is invisible) and that `docs/RELEASE.md:168` is corrected.

**Handoffs:** SQL-1 → backend, frontend (banner), devops (release default).
SQL-2/SQL-3 → backend. SQL-4 → backend + docs. SQL-5 → backend + manager/devops.
SQL-6 → manager/product/backend/devops/database (Phase 3 gate). SQL-7 →
backend/devops (Windows packaging). SQL-8 → backend/devops (pin + CI audit).

## Health score

| Area | Score | Rationale |
|---|---:|---|
| Auth | 7/10 | Strong hashing/rotation and fail-closed middleware; SQL-4 cross-dataset token confusion; seed lifecycle. |
| Input | 8/10 | All SQL parameterized; SQL-3 path/DSN handling. |
| API | 7/10 | Good loopback/auth boundary; no backend signal, `healthz` is status-only, prior CORS/guard items. |
| Storage | 5/10 | `0600`/`0700` on create, hashed tokens; existing dir, sidecars, Windows ACL, no encryption, no SQLite backup. |
| Dependencies | 7/10 | Pinned and recent SQLite 3.53.4; libc mismatch; no audit output. |
| Desktop | 6/10 | Correct loopback/ephemeral port; fallback UX, roaming profiles, multi-user silos. |
| **Overall** | **6/10** | Safe for auth-only Phase 2 offline use. Do not ship Phase 3 business data in SQLite until SQL-1, SQL-2, SQL-4, SQL-6, and SQL-7 have owners and verification results. |

**Recommended fix order:** SQL-1 → SQL-4 → SQL-2 → SQL-7 → SQL-3 → SQL-5 →
SQL-8 → SQL-6. SQL-1 and SQL-4 are the paths that can silently put the
operator on the wrong dataset or the wrong identity; the rest are conditional
exposure or hardening.

## Resolution status (post-review)

Appended after the review. The original findings above are unchanged; this
section records status against the current code and tests. Static review, no
commands run.

| ID | Status | Current control | Evidence |
|---|---|---|---|
| SQL-1 | **Resolved** (visibility; accepted residual) | `/healthz` reports the active backend; `auto` fallback still logs a warning. | `main.go:42` passes `stores.Backend`; `internal/httpapi/router.go:14-24`; `internal/bootstrap/database.go:40`; `internal/httpapi/health_test.go:14-59`. |
| SQL-2 | **Resolved** (reported exposures) | Parent directory is always `chmod 0700`, including when it already exists; symlink database paths are rejected before open; DB plus any `-wal`/`-shm` are best-effort `0600` after open. | `internal/db/sqlite.go:18-28,41,45-65`; `internal/db/sqlite_test.go:11-94`; `internal/bootstrap/database_test.go:95-124`. |
| SQL-3 | **Open** | `SQLITE_PATH` still lacks absolute/clean/owner/regular-file validation, and the DSN is still built by interpolation. | `internal/config/config.go:146-152,201-207`; `internal/db/sqlite.go:30`. |
| SQL-4 | **Resolved** | Access tokens now carry `iss`, `iat`, and a random `jti`; parsing requires the expected issuer and HS256. The issuer is `inventory-desktop/<backend>` built from the active store. | `internal/auth/jwt.go:25-58`; `internal/service/auth.go:39-44`; `main.go:34-42`; `internal/auth/auth_test.go:39-76`; `internal/httpapi/auth_issuer_test.go:25-79`. |
| SQL-5 | **Open** | Seed still follows `auto`, omits the backend from the final line, and forces `IsActive=true` on update. | `cmd/seed/main.go:19-24,44-56,58-112`. |
| SQL-6 | **Open** | Local data at rest is still unencrypted; no SQLite-specific backup/restore procedure or encryption decision. | `docs/ARCHITECTURE.md:126-138`; `docs/SYNC.md:64-70`. |
| SQL-7 | **Open** | Windows `os.Chmod` still does not set an ACL; roaming `%AppData%` and network/WAL behavior are unresolved. | `internal/db/sqlite.go:26-28,59-65`; `internal/config/config.go:193-207`. |
| SQL-8 | **Open** | Dependency audit output is still absent; libc pin still differs from the sqlite module's declared version. | `go.mod:11,52`; no `govulncheck`/`go list -m -u all` evidence in the repo. |

**SQL-2 residuals (tracked, not re-opened):** the driver still creates the
database during `Ping` before the post-open `0600` chmod
(`internal/db/sqlite.go:37,41`); there is no pre-create with
`O_CREATE|O_EXCL`, no owner check, and no non-regular-file check. DSN/path
validation remains SQL-3. These are bounded by the `0700` directory and the
new symlink rejection.

**SQL-4 residuals:** the issuer binding prevents SQLite/PostgreSQL token
crossover, including shared-secret multi-station use, but tokens minted before
this change are rejected after deployment and users must re-login. There is
still no explicit forced re-login on a backend switch; a backend switch
changes the expected issuer, so old access tokens fail closed.

### Remaining accepted risks / open items

- **`DB_MODE=auto` default:** `internal/config/config.go:136-138` still
  defaults to `auto`; release does not require `postgres` or an explicit
  offline opt-in. This is accepted for Phase 2, with `/healthz` as the
  visibility control and the stderr warning as the fallback signal. No
  persistent UI banner or `db.fallback` event.
- **Local data not encrypted:** SQLite remains plaintext at rest. Same-user
  processes can read the DB, WAL, and `config.env` regardless of `0600`/`0700`.
  This remains the SQL-6 Phase 3 gate.
- **Windows ACL limitation:** `0600`/`0700` are Unix permissions only.
  `os.Chmod` does not set a Windows ACL, and `%AppData%` is roaming. SQL-7 is
  open.
- **Separate auth datasets:** SQLite and PostgreSQL still have independent
  users with overlapping IDs. Issuer binding prevents token crossover, but
  local users must be provisioned per backend and there is no sync or mapping
  (SQL-5/SYNC).
- **Future sync security:** `docs/SYNC.md` remains unimplemented. Device key
  storage, rotation/revocation, sync endpoint authentication, outbox PII
  handling, and encryption at rest are still open.
- **Dependency audit pending:** SQL-8 stays open until `govulncheck ./...`,
  `go list -m -u all`, and `pnpm audit` output is reviewed; the libc pin should
  match the `modernc.org/sqlite` module or be deliberately justified.

The health scores above are not re-scored here; unresolved SQL-3, SQL-5,
SQL-6, SQL-7, SQL-8 and the residuals above still bound the overall score.
