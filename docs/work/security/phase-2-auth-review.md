# Security review — Phase 2 authentication and desktop boundary

**Scope:** Phase 2 Wails shell, loopback HTTP API, authentication flow, config
loader, seed command, and the React auth/bootstrap path. Static review of the
working tree; no commands were run.

**Verdict:** No Critical finding. One High (conditional) signing-secret
configuration finding. Five Medium findings. The implemented auth primitives
are sound; the main risks are configuration fail-open paths and the fixed
fallback API URL in the WebView.

## Summary

The Phase 2 auth slice implements the required primitives correctly:
loopback-only binding on an ephemeral port, HS256 with an explicit algorithm
check, 30-minute access tokens, 7-day refresh tokens hashed at rest with
SHA-256, transactional rotation, logout revocation, inactive-user rejection,
bcrypt default cost 12, parameterized SQL, a 10 MiB body cap, a normalized
error envelope, and panic recovery. The access-token flow is enforced by a
fail-closed middleware for every `/api/v1` path.

The weaknesses are not in the token math; they are in the edges:

- `SECRET_KEY` accepts any non-empty value, including the placeholder shipped
  in `.env.example`. If that placeholder is used, the HS256 key is public and
  an attacker can forge tokens for any user, including superusers.
- The WebView falls back to a fixed `127.0.0.1:8787` URL when the Wails
  binding is absent, empty, or throws. A local process on 8787 can then
  receive credentials and tokens.
- Config-file permissions are set on write but never verified on read.
- Dev CORS origins are compiled into release builds.
- The dev PostgreSQL container publishes port 5433 on all host interfaces with
  default credentials.
- The default DSN silently connects with a known local account when
  `DATABASE_URL` is unset.

The superuser surface (`/users`) is specified but not yet implemented, so
server-side `is_superuser` enforcement cannot be verified in this build and is
a merge gate before those routes land.

## 1. Loopback HTTP server

| Control | Evidence | Result |
|---|---|---|
| Bind address | `main.go:55` binds `127.0.0.1:0`; `cmd/server/main.go:55` binds `127.0.0.1:<PORT>` | Pass |
| Ephemeral port | `main.go:55` uses port `0`; `server.go:48-50` derives `BaseURL` from the listener | Pass |
| Public bind guard | No validation inside `httpapi.NewServer`; safety depends on callers (`server.go:20-45`, `main.go:55`) | F18 (Low) |
| CORS allowlist | Exact map, no wildcard: `middleware.go:32-39`, `middleware.go:90-97`. Includes `wails://wails` (Linux/macOS) and `http://wails.localhost` (Windows) | Pass |
| Dev origins in release | `http://localhost:5173`, `http://127.0.0.1:5173`, `:4173` are always allowed (`middleware.go:35-38`) | F4 (Medium) |
| Credentials header | `Access-Control-Allow-Credentials: true` is sent although `withCredentials: false` (`middleware.go:93`, `axios.ts:61`) | F4 (Low, part of F4) |
| OPTIONS handling | Any `OPTIONS` returns `204` before auth (`middleware.go:98-101`); the test asserts allowed and denied origins (`auth_integration_test.go:167-198`) | Pass, but it does not validate `Host`/`Origin` on non-preflight requests (F19) |
| Shutdown | Wails `OnShutdown` plus idempotent `app.shutdown` and a 10 s drain (`main.go:62-69`, `app.go:25-31`, `server.go:52-54`) | Pass |
| Timeouts | Read header 5 s, read 15 s, write 30 s, idle 60 s (`server.go:25-28`) | Pass |
| URL logging | Wails path does not log the URL; `cmd/server/main.go:61` does, but that binary is dev-only | Pass for release |

Notes:

- Wails v2.16 serves the app from `wails://wails/` on Linux/macOS and
  `http://wails.localhost/` on Windows. Both origins are in the allowlist.
  Confirm the actual header on each target OS during the release smoke test.
- A loopback API is reachable by every local process. The random port is
  obscurity, not an authentication control; bearer auth is the control. That
  matches `docs/SECURITY.md:44-51` and `docs/ARCHITECTURE.md:62-72`.

## 2. Authentication

| Control | Evidence | Result |
|---|---|---|
| Algorithm | HS256 is required and the header algorithm is pinned (`jwt.go:34-36`) | Pass |
| Claims | `sub` = user ID, `exp` = now + TTL (`jwt.go:22-29`); no `iat`, `jti`, `iss`, or `aud` | F17 (Low) |
| Access expiry | Default 30 min, positive int required (`config.go:19,83-100`); no upper bound | F17 (Low) |
| Refresh expiry | Default 7 days, positive int required (`config.go:20,87-100`); rotation resets the 7-day window | F13 (Low) |
| Secret generation | 32 bytes from `crypto/rand`, base64url (`config.go:189-195`) | Pass |
| Secret persistence | OS config dir `0700`, file `0600`, `Chmod` after append (`config.go:170-187`) | Pass on create; see F3 for existing files |
| Secret validation | Any non-empty `SECRET_KEY` is accepted (`config.go:72-81`); `.env.example:5` ships a known placeholder | **F1 (High, conditional)** |
| bcrypt cost | Default 12 (`config.go:21,91`); accepted range 4–31 (`config.go:101-103`); seed and tests use config (`cmd/seed/main.go:77,100`) | F11 (Low) |
| Password length | Seed enforces 8–128 runes and 72 bytes (`cmd/seed/main.go:35-43`); hash helper does not (`password.go:7-13`); contract says 8–128 characters (`docs/API-CONTRACT.md:74`) | F12 (Low) |
| Login rate limit | 5 per minute per `clientIP` (`auth.go:41,46-49,134-170`) | Pass, but see F8 |
| Rate-limit key | `clientIP` uses `RemoteAddr` only and never trusts `X-Forwarded-For` (`middleware.go:158-164`) | Pass; all loopback clients share one key (F8) |
| Inactive user on login | Verified after password check (`service/auth.go:56-61`) | Pass |
| Inactive user on every request | Checked in `AuthenticateAccessToken` (`service/auth.go:142-144`) | Pass |
| Refresh generation | 32 random bytes, base64url (`refresh.go:10-16`) | Pass |
| Refresh at rest | SHA-256 hex, unique index (`refresh.go:18-21`, `migrations/00001_initial_schema.sql:409-418,1020-1023`) | Pass; stronger than legacy |
| Refresh rotation | Old row revoked and new row inserted in one transaction (`repository/refresh_tokens.go:52-86`) | Pass |
| Refresh reuse handling | Replaying a rotated token returns 401, but the token family is not revoked (`service/auth.go:84-114`) | F13 (Low) |
| Logout | Revokes the presented refresh token idempotently (`service/auth.go:123-128`, `repository/refresh_tokens.go:88-98`) | Pass |
| Access token after logout | Remains valid until expiry; no denylist or user token version | Accepted risk (documented below) |
| Error messages | Same `detail` for unknown email and wrong password; inactive gets a distinct 401 by contract (`httpapi/auth.go:112-124`) | Pass; timing still leaks (F7) |
| Superuser routes | No `/users` routes exist yet; `UserFromContext` is present (`middleware.go:140-143`) but no superuser middleware | Open item, merge gate |

## 3. Input handling

| Control | Evidence | Result |
|---|---|---|
| Body cap | 10 MiB `MaxBytesReader` in `middleware.go:106-111` and again in `response.go:32-43`; `413` mapped in `httpapi/auth.go:126-132` | Pass |
| Form parsing | Login uses `r.ParseForm()` and `r.PostFormValue` (`httpapi/auth.go:51-56`), so query values do not become credentials | Pass |
| JSON decoding | `DisallowUnknownFields`, rejects trailing data (`response.go:32-43`) | Pass |
| Validation errors | Generic `422` with `VALIDATION_ERROR`; decoder details are discarded (`httpapi/auth.go:126-132`) | Pass |
| Error envelope | `{detail, code}`; `Status` is not serialized (`response.go:12-30`) | Pass |
| Information leakage | Auth failures return fixed strings; unexpected errors are logged and mapped to a generic 500 (`httpapi/auth.go:112-124`) | Pass |
| SQL injection | All queries use `$n` placeholders; only the static column list is concatenated (`repository/users.go:14,24-32`, `repository/refresh_tokens.go:23-98`) | Pass |
| Numeric/search/pagination bounds | Not implemented in this Phase 2 slice; auth inputs are only checked for non-empty (`httpapi/auth.go:57-60`) | Open item for Phase 3 |
| Login length bounds | No server-side max on `username` or `password` before `ParseForm`/bcrypt; the 10 MiB body cap is the only bound | F12 (Low) |
| Content-Type enforcement | `decodeJSON` does not require `application/json`; a simple cross-origin `text/plain` POST is accepted | F19 (Low) |

The login form is limited to 5 requests per minute per process (F8), so the
missing per-field length check is not a practical bcrypt DoS; bcrypt itself
truncates at 72 bytes. Add explicit field bounds when the user-management
handlers land.

## 4. Logging and errors

| Control | Evidence | Result |
|---|---|---|
| Access log fields | Request ID, method, path, status, duration only (`middleware.go:68-85`) | Pass |
| Query strings | Only `r.URL.Path` is logged, never `RawQuery` (`middleware.go:80`) | Pass |
| Headers/body | Never logged | Pass |
| Tokens/passwords | Not logged on success or auth failure; error logs contain the Go error only (`httpapi/auth.go:121`, `service/auth.go`) | Pass |
| Password hash serialization | `HashedPassword` and `TokenHash` are `json:"-"` (`model/user.go:9,17`) | Pass |
| Request ID | Generated with `crypto/rand`; client value accepted if ≤128 chars (`middleware.go:57-66`) | F15 (Low) |
| Panic recovery | Recovered, generic `500`, stack logged with request ID (`middleware.go:41-55`) | Pass |
| Panic after partial write | Recovery can attempt a second `WriteHeader`; response may be truncated | Low, not security-critical |
| DB errors | pgx redacts passwords in `ParseConfigError`; connection errors name user/host/database, not the password | Pass; add a log-redaction test |
| PII | No customer PII exists in this slice; seed prints the user email to stdout (`cmd/seed/main.go:92,120`) | Low |

The middleware order is `body limit → recover → request ID → logging → CORS →
auth → handler` (`router.go:24-31`). A panic bypasses the access-log line
because recovery is outside logging; the recovery log still carries the
request ID. This is an observability gap, not a security gap.

## 5. Desktop and WebView boundaries

| Control | Evidence | Result |
|---|---|---|
| Token storage | Access, refresh, and user cache in `localStorage` (`AuthContext.tsx:16-45,58-75`, `axios.ts:64-74,101-128`) | Accepted risk, documented in `docs/DECISIONS.md:120-132` and `docs/SECURITY.md:66-78` |
| XSS sinks | No `dangerouslySetInnerHTML`, `eval`, `new Function`, `innerHTML`, or `document.write` in `frontend/src` | Pass |
| Remote scripts | None in `index.html`; only the local module entry (`index.html:13`) | Pass |
| Remote styles/fonts | `@import` from `api.fontshare.com` and `fonts.googleapis.com` (`index.css:6-7`); preconnect hints (`index.html:8-9`) | **F14 (Low hardening)** |
| CSP | No CSP meta tag and no CSP header in Wails asset serving | F14 |
| API base URL binding | `GetAPIBaseURL` returns the listener URL (`app.go:19-23`, `main.go:62`); `bootstrap.ts:20-23` uses it | Pass |
| Fallback URL | On missing/failed binding the WebView uses `VITE_API_URL` or `http://127.0.0.1:8787/api/v1` (`bootstrap.ts:16-28`, `apiBase.ts:1`) | **F2 (Medium)** |
| URL validation | No scheme/host/path check before `setApiBaseUrl` (`apiBase.ts:9-11`) | F2 |
| Release devtools | `main.go:72-85` does not enable `Debug`; Wails v2.16 disables devtools unless built with the `devtools` tag | Pass; verify the release build |
| External navigation | No explicit navigation allowlist in `main.go`; no external links in the current UI | F14/OQ; verify on each OS |
| Context menu | Wails default context menu is off in production unless `EnableDefaultContextMenu` is set (it is not) | Pass |
| Multi-instance | No `SingleInstanceLock`; each instance gets its own port and API surface | Low, not a finding |

`localStorage` is the documented v1 choice. The review accepts it only because
there are currently no HTML injection sinks and no third-party scripts. F14 is
the mitigation gap: a CSP and self-hosted fonts are the cheap way to keep that
acceptance defensible. Any future rich-text, markdown, or `innerHTML` feature
must be a blocking security review item.

## 6. Config and secrets

| Control | Evidence | Result |
|---|---|---|
| Precedence | Env > OS config file > defaults (`config.go:58-70,117-120`) | Pass |
| Config location | `os.UserConfigDir()/inventory-desktop/config.env` (`config.go:134-140`) | Pass |
| Create permissions | Dir `0700`, file `0600`, `Chmod` after append (`config.go:170-187`) | Pass |
| Existing-file permissions | Not checked on read (`config.go:142-168`) | **F3 (Medium)** |
| Symlink/owner check | None | F3 (Low, part of F3) |
| Generated secret | 32 random bytes (`config.go:189-195`) | Pass |
| Placeholder secret | `.env.example:5` ships `SECRET_KEY=replace-with-a-random-32-byte-secret`; no placeholder rejection | **F1 (High, conditional)** |
| `.env` loading | Go does not load `.env`; `scripts/dev.sh:7-11` and `scripts/build.sh:7-11` source it | Documented mismatch; part of F1/F2 |
| Default DSN | `postgresql://inventory:inventory@localhost:5433/inventory_desktop` (`config.go:18`, `docs/ARCHITECTURE.md:120`) | **F6 (Medium)** |
| Dev DB exposure | Compose publishes `5433:5432` on all interfaces with `inventory/inventory` (`docker-compose.yml:5-9`) | **F5 (Medium)** |
| DB least privilege | The app uses the compose superuser account in dev; no app-limited role | F6 |
| DB TLS | No `sslmode` enforcement; pgx default is not `verify-full` | F6 (Low, part of F6) |
| Secrets in git | `.env`, `*.pem`, `*.key`, dumps ignored (`gitignore:30-40`); only `.env.example` is tracked | Pass |
| Test secrets | `playwright.config.ts:32` uses `e2e-test-secret`; tests use `test-secret`/cost 4 | Accepted test-only; do not reuse |
| Seed credentials | Flag/env input, hashed before storage (`cmd/seed/main.go:21-23,77,100`) | Pass; argv exposure is F9 |
| Seed reactivation | `existing.IsActive = true` on every update (`cmd/seed/main.go:106`) | F10 (Low) |
| Logs/errors | No secret or DSN logging found | Pass; verification step below |

## 7. Dependencies and release

| Control | Evidence | Result |
|---|---|---|
| Go pinning | Direct versions pinned in `go.mod:5-11`; `go.sum` present | Pass |
| Frontend pinning | `pnpm-lock.yaml` present; `package.json` uses caret ranges but the lockfile pins resolution | Pass |
| Dependency audit | Not run in this review (no shell commands) | OQ: run `govulncheck ./...`, `go list -m -u all`, `pnpm audit --audit-level=high` |
| Build env leakage | `scripts/build.sh:7-11` sources `.env`; any `VITE_*` value is embedded in the release bundle | F2 (Medium) |
| Release devtools | No debug build tag or `Debug` option in `main.go` | Pass; verify artifact |
| Release signing/checksums | Documented only (`docs/RELEASE.md:101-109`); not implemented in this repo | OQ |
| Backup/restore | Documented (`docs/RELEASE.md:89-99,127-152`); no evidence of a test | OQ |
| Artifact secrets | No `.env`/DSN/secret in source; `build/bin/` is ignored | Pass in source; inspect the built artifact |
| Dev server binary | `cmd/server` binds a fixed 8787 and logs the address (`cmd/server/main.go:51-61`); dev-only | Ensure it is not shipped as the release binary |

## 8. Findings

Severity follows `docs/SECURITY.md:160-167`. "Type" distinguishes a real
vulnerability from defense-in-depth.

| ID | Severity | Type | Location | Issue | Impact | Remediation | Verification |
|---|---|---|---|---|---|---|---|
| F1 | **High** (conditional) | Vulnerability | `internal/config/config.go:72-81`; `.env.example:5`; `scripts/dev.sh:7-11`; `scripts/build.sh:7-11` | Any non-empty `SECRET_KEY` is accepted, including the known placeholder. If the template is copied verbatim, the HS256 signing key is public. | Attacker forges an access JWT with `sub` = any user ID and full superuser rights; complete auth bypass. The generated-secret path is safe. | Reject the placeholder and secrets shorter than 32 bytes (prefer 43 base64url chars) in `config.Load`; leave `SECRET_KEY=` blank in `.env.example` with a generation command; add a startup warning/error. | `SECRET_KEY=replace-with-a-random-32-byte-secret go run ./cmd/seed ...` exits with a config error; unit test covers placeholder and 8-char secret. |
| F2 | Medium | Vulnerability (conditional) | `frontend/src/bootstrap.ts:16-28`; `frontend/src/lib/apiBase.ts:1`; `frontend/src/bootstrap.test.ts:31-43`; `scripts/build.sh:7-11` | On missing/empty/throwing Wails binding, the WebView falls back to a fixed `127.0.0.1:8787` or a build-time `VITE_API_URL`; the base URL is not validated. | A local process on 8787, or a release built with `VITE_API_URL`, receives login credentials and bearer/refresh tokens. Requires binding failure or a bad build env, but that is exactly the fallback path. | In production, fail closed if the binding is absent/invalid; validate `http://127.0.0.1:<ephemeral>/api/v1` and reject non-loopback hosts; do not bake `VITE_API_URL` into release builds; update the test that currently asserts fallback. | Build a release with `VITE_API_URL=https://evil.example`; launch with the binding stubbed to throw; app shows an error and makes no request to 8787/evil. |
| F3 | Medium | Vulnerability (conditional) | `internal/config/config.go:142-168,170-187` | The app sets `0600` when it creates/appends the config file but never checks the mode, owner, or file type when reading an existing file. | A `0644` config exposes `SECRET_KEY` and `DATABASE_URL` to other local users → JWT forgery and direct DB access. | `os.Lstat`, reject non-regular files/symlinks, require owner match, and reject or `chmod 0600` on `mode&0o077 != 0`; also `chmod 0700` an existing config dir. | `chmod 0644 ~/.config/inventory-desktop/config.env`; start the app; it refuses or fixes the mode. `stat -c '%a %U %n'` shows `600`. |
| F4 | Medium | Hardening | `internal/httpapi/middleware.go:32-39,90-97`; `frontend/src/lib/axios.ts:61` | Dev origins `localhost:5173`, `127.0.0.1:5173`, and `:4173` are compiled into every build and `Allow-Credentials: true` is sent unnecessarily. | A page served from a local dev port can make cross-origin requests and read API responses. The bearer token is not a cookie and is not attached, so token theft is not immediate; the surface is still unnecessary and becomes dangerous if cookies are added. | Gate dev origins behind a dev build flag; keep only the Wails origins in release. Drop `Access-Control-Allow-Credentials` for bearer auth. Reject non-empty disallowed `Origin` on non-preflight `/api/v1` requests. | Release build: `OPTIONS` with `Origin: http://localhost:5173` returns no `Access-Control-Allow-Origin`; WebView login still works. |
| F5 | Medium | Vulnerability (dev environment) | `docker-compose.yml:5-9` | PostgreSQL publishes `5433:5432` on all host interfaces with `inventory/inventory`. | Any host on the same LAN can reach the dev database, read customer/business data, and modify schema/data. | Bind `127.0.0.1:5433:5432`; take `POSTGRES_PASSWORD` from an ignored `.env`; never reuse this instance for production. | `docker compose up -d`; `ss -ltnp | grep 5433` shows `127.0.0.1:5433`; a remote `psql` fails. |
| F6 | Medium | Hardening | `internal/config/config.go:18,117-120`; `docker-compose.yml:5-7`; `docs/ARCHITECTURE.md:120` | With `DATABASE_URL` unset, the app silently connects to a known local DSN with a privileged role; no `sslmode` enforcement for remote hosts. | Production can fail open to an unintended/local DB; a local impostor on 5433 can receive queries and data; the app role has more DB privileges than needed. | Require `DATABASE_URL` in release/non-dev and fail startup; provision a least-privilege app role; require `sslmode=verify-full` for non-loopback hosts. | Unset `DATABASE_URL`, run the release binary → startup error. `\du` shows the app role has only DML/select on app tables. |
| F7 | Low | Vulnerability (low impact) | `internal/service/auth.go:48-61` | Unknown email returns before bcrypt; wrong password runs bcrypt. The response body is identical, but timing differs. | An attacker with local access can enumerate valid emails despite the 5/min limiter. | On `ErrNotFound`, compare against a fixed dummy bcrypt hash at the configured cost before returning `ErrInvalidCredentials`. | Timing test or benchmark: unknown-email and wrong-password latencies are within noise. |
| F8 | Low | Hardening | `internal/httpapi/auth.go:46-49,134-170`; `internal/httpapi/middleware.go:158-164` | The limiter is keyed only by `RemoteAddr`, which is always loopback, so 5 attempts/min is a global app budget; logout/refresh are unlimited; the map is never evicted. | A local process can exhaust the budget and block the operator from logging in; no per-account brute-force control; a future non-loopback bind would grow the map unbounded. | Add a per-username limit in addition to the IP limit; add `Retry-After`; evict expired windows; rate-limit logout/refresh by token or user. | Two usernames from `127.0.0.1` have independent budgets; the 429 includes `Retry-After`; a unit test covers eviction. |
| F9 | Low | Hardening | `cmd/seed/main.go:21-23`; `frontend/playwright.config.ts:25` | `-password` puts the seed password in argv, visible in `ps` and shell history. | A local user can read the bootstrap password while seed runs. | Remove `-password` or read from stdin with `golang.org/x/term.ReadPassword`; keep `SEED_PASSWORD`; update Playwright and docs. | `ps`/`/proc/<pid>/cmdline` never contains the password during seeding. |
| F10 | Low | Hardening | `cmd/seed/main.go:106` | Every seed update sets `IsActive = true`, even if the operator only changes a name. | A deliberately disabled account is silently re-enabled whenever seed runs. | Only change `IsActive` when an explicit `-active`/`-activate` flag is passed; otherwise keep the stored value. | Deactivate a user, run seed with only `-name`; the user remains inactive. |
| F11 | Low | Hardening | `internal/config/config.go:101-103` | `BCRYPT_ROUNDS=4` is accepted in production; only the default is 12. | A leaked database can be cracked much faster if the operator lowers the cost. | Require ≥12 unless an explicit test/dev flag is set; keep test config at 4. | `BCRYPT_ROUNDS=4` release start fails; test config still works. |
| F12 | Low | Hardening/compat | `cmd/seed/main.go:35-43`; `internal/auth/password.go:7-13`; `docs/API-CONTRACT.md:74` | Contract says 8–128 characters; bcrypt truncates/rejects at 72 bytes. Seed rejects >72 bytes, so the contract cannot be honored for 73–128-byte passwords. | Users can be created with a password the hash cannot represent, or valid long passwords are rejected inconsistently. | Decide on 8–72 bytes and update the contract/docs/frontend validators, or pre-hash (e.g., SHA-256) before bcrypt if 128 chars is required. Enforce the same rule at every creation/change boundary. | Create a user with a 73-byte password → 422; 72 bytes succeeds and verifies. |
| F13 | Low | Hardening | `internal/service/auth.go:79-121`; `internal/repository/refresh_tokens.go:52-86` | Rotation invalidates the presented token but does not detect reuse of a rotated token or revoke its family; expiry slides with each rotation. | A stolen refresh token used before the legitimate client keeps working after the victim rotates; a session can live indefinitely if refreshed. | Add `family_id`/`parent_id`; on reuse, revoke the whole family and return 401; add an absolute session expiry. | Integration test: login, rotate, replay the old token → 401 and the newest token is also revoked. |
| F14 | Low | Hardening | `frontend/src/index.css:6-7`; `frontend/index.html:8-9`; no CSP | The WebView loads remote font CSS from two third-party origins and has no CSP. | Third-party content is loaded into the token-bearing origin; without CSP, any future XSS executes freely and can read `localStorage` tokens. This violates `docs/SECURITY.md:120-125`. | Self-host the fonts; remove the `@import`/preconnect; add a CSP (meta or asset-server header) with `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src http://127.0.0.1:*; object-src 'none'; base-uri 'none'; frame-ancestors 'none'`. | Release build makes no request to `fontshare.com`/`fonts.googleapis.com`; DevTools shows no CSP violations; `grep` of `frontend/dist` finds no remote font URL. |
| F15 | Low | Hardening | `internal/httpapi/middleware.go:57-66` | A client-supplied `X-Request-Id` up to 128 characters is trusted and echoed to logs and the response. | Log spoofing/correlation abuse; `slog` quotes newlines, so log injection is limited. | Accept only `^[A-Za-z0-9-]{1,64}$`, otherwise generate; or always generate server-side. | Send `X-Request-Id: foo\nbar`; response/log use a generated ID. |
| F16 | Low | Hardening | `internal/httpapi/response.go:22-30` | Token-bearing auth responses do not set `Cache-Control: no-store`. | A WebView or intermediary could cache a login/refresh response containing tokens. | Set `Cache-Control: no-store` and `Pragma: no-cache` on login/refresh/logout responses. | `curl -i` login shows both headers. |
| F17 | Low | Hardening | `internal/auth/jwt.go:22-29`; `internal/config/config.go:95-100` | JWTs lack `iat`, `jti`, `iss`, and `aud`; TTL values have no upper bound. | No issuance time or revocation hook; an operator can issue year-long access tokens. | Add `iat` (and `jti` if revocation is planned), validate issuer/audience if multiple apps share a secret, clamp access ≤60 min and refresh ≤30 days. | Unit tests assert claims and reject out-of-range TTL config. |
| F18 | Low | Hardening | `internal/httpapi/server.go:20-45`; `main.go:55`; `cmd/server/main.go:55` | `NewServer` accepts any address; loopback binding is enforced only by callers. | A future caller could bind `0.0.0.0` and expose the API to the LAN. | Validate in `NewServer` that the host is loopback (`net.ParseIP(host).IsLoopback()`), or make the constructor take a port and bind `127.0.0.1` itself. | `NewServer("0.0.0.0:0", ...)` returns an error; existing tests pass. |
| F19 | Low | Hardening | `internal/httpapi/middleware.go:87-104`; `internal/httpapi/response.go:32-43` | No `Host` validation and no rejection of a disallowed `Origin` on non-preflight requests; JSON endpoints accept any content type. | DNS rebinding / cross-origin writes reach public endpoints; the browser cannot read the response, but state changes execute. Authenticated endpoints still require a bearer token. | Reject requests whose `Host` is not `127.0.0.1:<port>`/`localhost:<port>` (and `[::1]` if used); reject non-empty disallowed `Origin` for `/api/v1`; require `application/json` on JSON routes. | `curl -H 'Host: evil.example' http://127.0.0.1:PORT/healthz` → 421/403; disallowed `Origin` POST → 403. |
| F20 | Low | Hardening | `frontend/src/context/AuthContext.tsx:16-40` | The cached user object includes `id`, `email`, and names in `localStorage`. | PII at rest in WebView storage, readable by any XSS. Minor compared with the tokens already there. | Cache only `full_name` and `is_superuser` (as the legacy app does), or keep the user in memory and re-fetch. | `localStorage.getItem('user')` contains no email/id after login. |

### Accepted risks (not counted as findings)

- **`localStorage` token storage** (`AuthContext.tsx:43-45,60-61`;
  `docs/DECISIONS.md:120-132`). Accepted for v1. Mitigations are currently
  incomplete because F14 adds no CSP. OS keyring remains the documented
  upgrade path.
- **Access tokens survive logout** until their 30-minute expiry; logout revokes
  only the refresh token (`service/auth.go:123-128`). This is standard for
  short-lived JWTs. Do not add a denylist without a measured need.
- **No password-change/token-revocation endpoint yet.** When user management
  lands, a password change must revoke all refresh tokens for that user and
  ideally bump a token version so outstanding access tokens die sooner.

## Positive controls

- Loopback-only, ephemeral-port binding in the Wails entry point
  (`main.go:55`) with a listener-derived base URL (`server.go:48-50`).
- Graceful shutdown with a 10 s drain and an idempotent shutdown guard
  (`main.go:62-69`, `app.go:25-31`).
- Exact auth whitelist for login/refresh/logout/health; every other
  `/api/v1` path fails closed without a bearer token
  (`middleware.go:26-30,113-119`).
- HS256 algorithm pinning (`jwt.go:34-36`) and expiry validation via
  `golang-jwt/jwt/v5`.
- 32-byte `crypto/rand` refresh tokens, SHA-256 at rest, unique index, and
  transactional rotation (`refresh.go`, `repository/refresh_tokens.go:52-86`,
  `migrations/00001_initial_schema.sql:1020-1023`). This is stronger than the
  legacy API, which stores the raw refresh token.
- Inactive-user rejection on login, refresh, and every authenticated request
  (`service/auth.go:59-61,100-102,142-144`).
- Fixed error details for bad credentials and a generic 500 for unexpected
  errors (`httpapi/auth.go:112-132`).
- Exact-origin CORS allowlist with no wildcard (`middleware.go:32-39,90-97`).
- 10 MiB body cap, unknown-field rejection, and trailing-data rejection
  (`response.go:10,32-43`).
- Parameterized SQL only, including the refresh-token and user queries
  (`repository/users.go:24-65`, `repository/refresh_tokens.go:23-98`).
- No XSS sinks in `frontend/src`; React JSX escaping is the default.
- No token, password, authorization header, or request-body logging
  (`middleware.go:68-85`); password and token hashes are excluded from JSON
  (`model/user.go:9,17`).
- Panic recovery returns a generic 500 with the stack kept server-side
  (`middleware.go:41-55`).
- Config directory `0700` and file `0600` on creation, with an explicit
  `Chmod` (`config.go:170-187`).
- `.env`, keys, certificates, and database dumps are gitignored
  (`.gitignore:30-40`); only `.env.example` is tracked.
- Dependencies are pinned in `go.mod`/`go.sum` and `pnpm-lock.yaml`.
- Wails v2.16 production builds do not enable devtools unless built with the
  `devtools` tag; this project does not set `Debug` (`main.go:72-85`).

## Open questions and handoffs

1. **Superuser enforcement (merge gate).** `/users` routes are specified
   (`docs/API-CONTRACT.md:67-79`) but not implemented. Before they land,
   backend must add a server-side `is_superuser` check (e.g., a
   `requireSuperuser` middleware using `UserFromContext`) and tests for
   `403` with a regular user's token. `frontend/src/components/SuperuserRoute.tsx`
   is UI-only and must never be the authorization control.
2. **Dependency audits.** Please run and share:
   `govulncheck ./...`, `go list -m -u all`, and
   `cd frontend && pnpm audit --audit-level=high`. This review could not run
   commands.
3. **Config permissions on the real machine.** Please run
   `stat -c '%a %U %n' ~/.config/inventory-desktop/config.env` and
   `stat -c '%a %U %n' ~/.config/inventory-desktop`. Expect `600` and `700`.
4. **Docker exposure.** Please run `ss -ltnp | grep 5433` after
   `docker compose up -d`; it must show `127.0.0.1:5433`, not `0.0.0.0:5433`.
5. **Release environment.** Confirm the release build does not set
   `VITE_API_URL` (check `scripts/build.sh`'s `.env` sourcing) and that
   `frontend/dist` contains no remote API URL. Also confirm the shipped
   binary is the Wails build, not `cmd/server`.
6. **CSP support.** Decide whether to inject CSP through an
   `index.html` meta tag (works today) or a Wails asset-server header
   (requires a small Go change). Self-host fonts either way.
7. **Backup/restore.** `docs/RELEASE.md:89-99,127-152` requires a tested
   restore. No test evidence exists in this phase.
8. **Inactive-user message parity.** The legacy service returns the generic
   bad-credentials message for inactive users; this API returns
   `"Inactive user"` per `docs/API-CONTRACT.md:44-46`. Confirm the explicit
   status is intentional (it reveals account state to someone who already
   knows the password).
9. **`.env` semantics.** Go does not read `.env`; the shell wrappers do. If
   `.env` is meant to be a runtime config source, document that only the
   wrappers load it; if not, remove the advice to copy `.env.example` to
   `.env` or add an explicit loader. This is also the delivery path for F1.
10. **Token cleanup.** Plan a periodic delete of expired/revoked
    `refresh_tokens` rows; the table otherwise grows without bound. Not a
    security finding, but it affects the rotated-token lookup index over time.

## Health score

| Area | Score | Rationale |
|---|---:|---|
| Auth | 8/10 | Strong primitives, rotation, revocation, inactive checks. Deductions: secret validation (F1), timing enumeration (F7), global limiter (F8), reuse-family gap (F13). |
| Input | 8/10 | Body cap, strict JSON, parameterized SQL, generic validation errors. Deductions: no field bounds on login, no content-type check, no Host/Origin rejection. |
| API | 8/10 | Fail-closed auth middleware, exact CORS allowlist, normalized errors, panic recovery. Deductions: dev origins in release (F4), no loopback constructor guard (F18), no superuser routes yet. |
| Storage | 6/10 | Tokens hashed at rest and config created `0600`. Deductions: config read permissions not verified (F3), known placeholder secret (F1), PII in `localStorage` (F20), default DSN (F6). |
| Dependencies | 7/10 | Pinned modules and lockfile, current-looking stack. Audits not run; release signing/checksums still process-only. |
| Desktop | 6/10 | Correct loopback/ephemeral binding and shutdown; XSS sinks absent. Deductions: fixed 8787 fallback (F2), remote fonts/no CSP (F14), no navigation allowlist, no multi-instance lock. |
| **Overall** | **7/10** | Safe to continue Phase 2 development. Do not ship a release until F1 is fixed and F2/F3/F4/F5/F6 have an owner and a verification result. F1 is a full auth bypass if the documented template is used verbatim. |

**Recommended fix order:** F1 → F2 → F3 → F5/F6 → F4 → F7/F8 →
F11–F20. F1 and F2 are the only paths that can yield tokens or a signing key
to an attacker; the rest are defense-in-depth.

**Handoff:** F1, F3, F6, F7, F8, F11, F12, F13, F15, F16, F17, F18, F19
→ backend. F2, F4 (frontend half), F14, F20 → frontend. F4 (release
origins), F5, and the audit/release checks → devops. F9/F10 → backend (seed).
Re-test after the fix and attach the verification command output to the
finding.
