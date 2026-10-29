# Security review — Phase 3 implementation (desktop Go API surface)

**Scope:** the implemented Phase 3 surface — loopback server, auth middleware,
all 67 routes, superuser Users routes, JWT/refresh lifecycle, CORS and body
limits, input validation, SQL access, logging/errors, config and SQLite
storage, and the React session flow.

**Method:** static review only. No shell commands were run (role denies shell),
no code, tests, migrations, or configuration were modified. Phase 2 findings are
carried in this audit's tables; the original Phase 2 review files were removed
during documentation cleanup.

**Date:** 2026-10-29.

## Tool results (manager-run)

The manager ran the checks this review could not run (the security role denies
shell access). These results are folded into the verdict and findings below.

| Check | Command | Result |
|---|---|---|
| Go vulnerability scan | `govulncheck ./...` | **Still not run locally — not installed** on this machine. A CI job now installs pinned `govulncheck@v1.9.0` and runs it (`.github/workflows/ci.yml:65-68`), but CI has **not run on GitHub yet**. The Go graph remains **unverified until CI is green**. |
| Frontend dependency audit (Session 3) | `pnpm audit --audit-level=high` and full `pnpm audit` in `frontend/` | **0 known vulnerabilities**. The 48 advisories (24 high) from the earlier run are resolved by the upgrade; see P3-8. |
| Config permissions | `stat -c '%a %U %n' ~/.config/inventory-desktop/config.env ~/.config/inventory-desktop` | `600 fabian ~/.config/inventory-desktop/config.env` and `700 fabian ~/.config/inventory-desktop`. Matches the `0600`/`0700` control. |
| Tracked secret scan | `git ls-files \| grep -E '(^\|/)\.env$\|\.key$\|\.pem$\|\.sqlite$'` | No output. No tracked `.env`, `.key`, `.pem`, or `.sqlite` files. |

## Summary

The Phase 3 API is in materially better shape than the Phase 2 review found.
The signing-secret hole (F1), the base-URL fallback that could deliver
credentials to `127.0.0.1:8787` (F2), the ungated dev CORS origins (F4), the
public compose port (F5), the loopback constructor guard (F18), cacheable token
responses (F16) and unbound JWTs (F17) are all fixed and have tests. The auth
boundary is fail-closed and I could not find an auth bypass, SQL injection, or
leaked-secret path.

**Session 3 closed the three Phase 3 release gates.** The High dependency
finding (P3-8) is resolved: the frontend packages are upgraded, there is no
remaining high advisory, and both `pnpm audit --audit-level=high` and the full
`pnpm audit` report no known vulnerabilities. Password reset now revokes every
refresh token for the user (P3-1) and the residual stateless access-token
window (≤30 minutes) is documented. The unencrypted-SQLite finding (P3-2) is
resolved **by decision and mitigation**, not by encryption: ADR-0021 accepts
unencrypted local storage for v1, a persistent non-dismissable banner is shown
before login and on every screen, permissions remain `0600`/`0700`, sync is out
of scope, and encryption is deferred. **Residual risk remains by design.**

Also fixed in Session 3: the dev server reads `DevMode` from config and logs
its address only in dev (P3-4); JWT parsing now requires `exp` (P3-6); brand
`logo_url` accepts only `http`/`https` (P3-7). A CI workflow was added with a
pinned `govulncheck@v1.9.0`, `go test`, `go test -race`, and
`pnpm audit --audit-level=high` (P3-5, **Mitigated** — CI has not run on
GitHub yet).

**No open Critical or High finding remains.** The carried Medium issues
(`F6` default DSN, `SQL-7` Windows permissions) and the Low defense-in-depth
items below remain open. The Go dependency scan is still unverified locally
because `govulncheck` is not installed; the CI job is the verification path.
None of the auth/data findings is remotely exploitable from the network; they
are data-protection failures under the documented threat model (stolen disk /
local process / silent divergence / malicious dependency).

## 1. Verdict

**No open Critical or High finding.** No auth bypass, no SQL injection, no
committed secret, no token or PII in logs, no wildcard CORS, and no
unauthenticated business endpoint were found. `P3-1`, `P3-4`, `P3-6`, `P3-7`,
and `P3-8` are resolved; `P3-2` is resolved by decision and mitigation
(ADR-0021) with residual risk accepted; `P3-5` is mitigated by a CI workflow
that has not yet run on GitHub. Config permissions (`0600`/`0700`) and the
tracked-secret check passed.

**Release is conditional, not blocked by a security finding.** Remaining gates:

1. **Run the new CI workflow on GitHub** and confirm `govulncheck ./...`,
   `go test`, `go test -race`, and `pnpm audit --audit-level=high` are green.
   The Go dependency graph is unverified until then.
2. **Windows/macOS packaging, signing, notarization, and checksums** are not
   produced or verified.
3. **Per-OS smoke tests** of the release artifact are not run (devtools closed,
   external navigation blocked, loopback-only bind, SQLite banner).

The carried Medium items (`F6` default DSN, `SQL-7` Windows permissions) and
the open Lows (`P3-3`, `F7`, `F8`, `F13`, `F14`, `F15`, `F19`, `F20`) should be
scheduled; they do not re-open the release gate on their own.

## 2. Findings

Severity follows `docs/SECURITY.md:192-200`. "Type" separates a vulnerability
from a hardening suggestion.

### New in Phase 3

| ID | Severity | Status | Type | Location | Issue | Impact | Remediation | Owner |
|---|---|---|---|---|---|---|---|---|
| P3-1 | Medium | **Resolved** | Vulnerability (session lifecycle) | `internal/application/users.go:110-116`; `internal/adapters/outbound/persistence/gormstore/refresh_tokens.go:98-105`; `internal/application/users_test.go:364`; `internal/application/auth_test.go:395` | `UserService.Update` now calls `RefreshTokenRepository.RevokeAllForUser` whenever a non-empty password is set, so every refresh token for that user is revoked. `Refresh` after a reset fails. Access tokens are stateless and remain valid until expiry. | The attacker's refresh token no longer survives a password reset. **Residual:** an already-issued access token works for at most 30 minutes. That window is accepted and documented in `docs/SECURITY.md:63-66`. | Implemented `RevokeAllForUser` (`UPDATE refresh_tokens SET revoked = true WHERE user_id = ? AND revoked = false`) and the call after `users.Update` succeeds. Tests: `TestUserUpdatePasswordRevokesRefreshTokens`, `TestUserUpdateWithoutPasswordKeepsRefreshTokens`, `TestUserUpdatePasswordRevocationFailurePropagates`, `TestPasswordResetRevokesRefreshToken`. | backend |
| P3-2 | Medium | **Resolved by decision/mitigation** | Vulnerability (data at rest, Phase 3 gate) | `frontend/src/components/OfflineDatabaseBanner/OfflineDatabaseBanner.tsx:6-35`; `frontend/src/App.tsx:26`; `docs/DECISIONS.md:342-360`; `docs/SECURITY.md:119-139`; `internal/db/sqlite.go` | SQLite still stores client PII, costs, prices, and stock without encryption. ADR-0021 explicitly accepts this for v1. The React app now calls `GET /healthz` and renders a persistent, non-dismissable banner (`role="status"`) before login and on every authenticated screen when `database=sqlite`. File/dir permissions stay `0600`/`0700`; sync is out of scope; encryption is deferred. | **Residual risk remains by design:** a stolen disk, backup, or roaming profile exposes local PII and margins. Mitigated by visibility, OS account separation, full-disk encryption, and owner-only permissions. `DB_MODE=auto` remains the v1 default per ADR-0021. | ADR-0021 is the accepted decision. Encryption at rest and sync each require a future ADR before implementation. Backups must be protected and taken with the app stopped or via SQLite's backup API (`docs/SECURITY.md:138-139`). | frontend + manager (done) |
| P3-3 | Low | **Open** | Hardening (destructive side effect) | `internal/db/sqlite.go:24-32` | The parent directory of `SQLITE_PATH` is unconditionally `MkdirAll`'d and `Chmod`'d to `0700`. `dir` is not `Lstat`'d, so a symlinked or shared parent (for example `SQLITE_PATH=/tmp/inv.sqlite`) gets its permissions changed. The path is also interpolated into the DSN at `sqlite.go:32` without escaping, so a `?` in the path can add connection parameters. | A configuration mistake silently tightens an unrelated directory (`/tmp`, a shared work folder), breaking other users/processes. A symlinked parent redirects the chmod/DB, and a crafted path can set `_pragma`/`mode` options. Not remotely reachable. | Only create/tighten a directory the app owns (under `os.UserConfigDir()`); require an absolute, cleaned `SQLITE_PATH`; `Lstat` each path component and refuse symlinks, non-regular files, or a directory not owned by the current user; build the DSN with `net/url` instead of `fmt.Sprintf`. | backend |
| P3-4 | Low | **Resolved** | Hardening (release hygiene) | `cmd/server/main.go:63,79-81` | `cmd/server` now passes `DevMode: cfg.DevMode` and logs the listen address only when `cfg.DevMode` is true. | The dev binary no longer unconditionally allows Vite/Playwright origins or advertises the port. Keep it out of release artifacts. | Fixed in Session 3. Release artifacts remain the Wails build (`Makefile:30-31`), not `cmd/server`. | backend + devops (done) |
| P3-5 | Low | **Mitigated** | Hardening (supply chain) | `.github/workflows/ci.yml:53-88` | A CI workflow now runs `go test`, `go test -race`, `go vet`, `gofmt`, a pinned `govulncheck@v1.9.0`, `pnpm lint`, `pnpm test`, `pnpm build`, and `pnpm audit --audit-level=high`. | The scan is enforced in CI, but **CI has not run on GitHub yet**. Local `govulncheck` is still not installed, so the Go graph is unverified until the first green run. | Push the workflow and confirm all jobs pass. A seeded known-vulnerable dependency should fail `govulncheck`; the audit gate fails on High. | devops |
| P3-6 | Low | **Resolved** | Hardening | `internal/adapters/outbound/security/jwt.go:46-54`; `internal/adapters/outbound/security/security_test.go:51` | `Parse` now passes `jwt.WithExpirationRequired()`; a token without `exp` is rejected. | A forged non-expiring token is no longer accepted even if `SECRET_KEY` leaks. Defense-in-depth only. | Fixed in Session 3; test asserts a token without `exp` is rejected. | backend (done) |
| P3-7 | Low | **Resolved** | Hardening (latent XSS sink) | `internal/application/catalog.go:242-253`; `internal/application/catalog_test.go:130-133` | Brand `logo_url` now parses as a URL and requires a non-empty host with an `http` or `https` scheme. `javascript:`, `data:`, `ftp:`, and relative URLs are rejected with `422`. | A future link/image render of `logo_url` can no longer execute stored script through a dangerous scheme. | Fixed in Session 3; tests cover `javascript:alert(1)` → validation error and `data:`/`ftp:`/relative rejection. | backend (done) |
| P3-8 | **High** | **Resolved** | Vulnerability (vulnerable dependency) | `frontend/package.json:17,23,25,45`; `frontend/pnpm-lock.yaml`; `docs/SECURITY.md:158-164` | The direct packages are upgraded: `axios ^1.21.0` (`>=1.20.0`), `react-router-dom ^7.18.5` (`>=7.18.2`), `zod ^4.6.7` (`>=4.6.6`), and `vite ^8.3.5` (`>=8.1.6`). Both `pnpm audit --audit-level=high` and the full `pnpm audit` report **no known vulnerabilities**; the frontend tests and build pass. | The High advisories that shipped in the token-bearing WebView bundle are gone. No known Critical/High dependency advisory remains. | Upgraded dependencies and re-resolved transitive packages in Session 3, then ran the audit and the test/build suite. Keep CI's `pnpm audit` gate to catch regressions. | frontend + devops (done) |

### Phase 2 findings carried into Phase 3

| Prior ID | Severity | Status | Evidence |
|---|---|---|---|
| F1 placeholder secret | High (conditional) | **Resolved** | Placeholder list + 32-byte minimum in `internal/config/config.go:179-191`; `.env.example:12` now ships `SECRET_KEY=` blank. |
| F2 base-URL fallback | Medium | **Resolved** | `frontend/src/bootstrap.ts:42-64` fails closed in production and validates loopback scheme/host/port/path. |
| F3 config read permissions | Medium | **Resolved (mode), residual owner check** | `internal/config/config.go:209-235` rejects symlinks/non-regular files, tightens dir `0700` and file `0600`. No owner check; `_ = os.Chmod(...)` at line 234 ignores failure. |
| F4 CORS credentials/dev origins | Medium | **Resolved** | `internal/adapters/inbound/httpapi/middleware.go:90-99`: dev origins gated by `DevMode`, no `Access-Control-Allow-Credentials`. |
| F5 compose port exposure | Medium | **Resolved** | `docker-compose.yml:9` binds `127.0.0.1:5433`. |
| F6 default DSN | Medium | **Open** | `internal/config/config.go:18`; `DB_MODE` default `auto` at `config.go:136-139`. |
| F7 email timing enumeration | Low | **Open** | `internal/application/auth.go:44-47` returns before bcrypt on unknown email. |
| F8 login limiter scope | Low | **Open** | `internal/adapters/inbound/httpapi/auth.go:126-162`; key is `clientIP` (`middleware.go:160-166`), always loopback → one global 5/min budget; successful logins count; no eviction; no `Retry-After`. |
| F9 seed password in argv | Low | **Open** | `cmd/seed/main.go:21`. |
| F10 seed re-enables users | Low | **Open** | `cmd/seed/main.go:90-96` always sets `IsActive: true`. |
| F11 bcrypt cost 4 allowed | Low | **Open** | `internal/config/config.go:109-111` accepts 4–31 in any environment. |
| F12 72-byte password | Low | **Partially fixed** | `internal/application/users.go:133-141` enforces 8–128 chars and ≤72 bytes; `docs/API-CONTRACT.md:74` still says only "8–128 characters". |
| F13 refresh reuse/family | Low | **Open** | `internal/application/auth.go:99-109`; `internal/adapters/outbound/persistence/gormstore/refresh_tokens.go:53-86` revokes only the presented token; expiry slides. |
| F14 remote fonts / no CSP | Low | **Open** | `frontend/src/index.css:6-7`, `frontend/index.html:8-9`; no `Content-Security-Policy` anywhere. |
| F15 trusted `X-Request-Id` | Low | **Open** | `internal/adapters/inbound/httpapi/middleware.go:62-67`. |
| F16 token responses cacheable | Low | **Resolved** | `Cache-Control: no-store` + `Pragma` on login/refresh/logout (`internal/adapters/inbound/httpapi/auth.go:40-41,73-74,95-96`). |
| F17 JWT claims | Low | **Resolved** | `iss`, `iat`, `jti` in `internal/adapters/outbound/security/jwt.go:35-41`; `WithIssuer` + HS256 pinning + required `exp` at `:51-53`. No TTL clamp yet. |
| F18 loopback constructor | Low | **Resolved** | `internal/adapters/inbound/httpapi/server.go:21-28` rejects any non-loopback host. |
| F19 Host/Origin/content-type | Low | **Open** | `internal/adapters/inbound/httpapi/middleware.go:90-105`; `internal/adapters/inbound/httpapi/response.go:32-43`. |
| F20 user PII in `localStorage` | Low | **Open** | `frontend/src/context/AuthContext.tsx:16-40,58-66`. |
| SQL-3 `SQLITE_PATH` validation | Low | **Open** | `internal/db/sqlite.go:24-32`; `internal/config/config.go:146-152` (see P3-3). |
| SQL-6 unencrypted local PII | Medium | **Resolved by decision/mitigation** | See P3-2. ADR-0021 (`docs/DECISIONS.md:342-360`) accepts unencrypted local storage for v1; banner, `0600`/`0700`, and `docs/SECURITY.md:119-139` document the residual risk. |
| SQL-7 Windows permissions/roaming | Medium | **Open** | `internal/db/sqlite.go:28,68-74`; `internal/config/config.go:193-207`. |

## 3. Session 3 verification

Re-audit of the fixes delivered after the original Phase 3 review. The security
role did not run shell commands; the evidence below was produced by the
implementing developers and the manager and is recorded verbatim.

| ID | Result | Evidence and residual risk |
|---|---|---|
| P3-1 | **Resolved** | Password reset revokes all refresh tokens for the user (`internal/application/users.go:110-116`, `gormstore/refresh_tokens.go:98-105`). Tests added: `TestUserUpdatePasswordRevokesRefreshTokens` (`internal/application/users_test.go:364`), `TestUserUpdateWithoutPasswordKeepsRefreshTokens`, `TestUserUpdatePasswordRevocationFailurePropagates`, and `TestPasswordResetRevokesRefreshToken` (`internal/application/auth_test.go:395`). Residual: stateless access tokens remain valid for **≤30 minutes**; documented in `docs/SECURITY.md:63-66`. |
| P3-8 | **Resolved** | Frontend dependencies upgraded (`frontend/package.json:17,23,25,45`: `axios ^1.21.0`, `react-router-dom ^7.18.5`, `zod ^4.6.7`, `vite ^8.3.5`). `pnpm audit --audit-level=high` and the full `pnpm audit` now report **no known vulnerabilities**. Frontend tests and build pass. |
| P3-2 | **Resolved by decision/mitigation** | Persistent, non-dismissable SQLite banner renders before login and on all screens (`frontend/src/components/OfflineDatabaseBanner/OfflineDatabaseBanner.tsx:6-35`, `frontend/src/App.tsx:26`), backed by `/healthz`. ADR-0021 accepts unencrypted local storage for v1 (`docs/DECISIONS.md:342-360`). Permissions remain `0600` file / `0700` directory. Sync is out of scope; encryption is deferred. **Residual risk remains by design** for a stolen disk/backup or roaming profile. |
| P3-4 | **Resolved** | `cmd/server` uses `DevMode: cfg.DevMode` and logs the listen address only in dev (`cmd/server/main.go:63,79-81`). |
| P3-6 | **Resolved** | JWT parsing requires `exp` via `jwt.WithExpirationRequired()` (`internal/adapters/outbound/security/jwt.go:53`); test in `security_test.go:51`. |
| P3-7 | **Resolved** | Brand `logo_url` accepts only `http`/`https` with a host (`internal/application/catalog.go:242-253`); tests reject `javascript:`, `data:`, `ftp:`, and relative URLs (`catalog_test.go:130-133`). |
| P3-5 | **Mitigated** | CI workflow added (`.github/workflows/ci.yml`): pinned `govulncheck@v1.9.0` (`:65-68`), `go test`, `go test -race`, and `pnpm audit --audit-level=high` (`:86-88`). **CI has not run on GitHub yet**, so the gates are not verified in practice. |
| P3-3, SQL-7, F6, F7, F8, F13, F14, F15, F19, F20 | **Open** | Unchanged from the findings and open-questions sections. |

**Go dependency scan:** still **not run locally** because `govulncheck` is not
installed on this machine. The CI workflow is the verification path (`go
install ...@v1.9.0` then `govulncheck ./...`); it must pass on GitHub before the
Go graph is considered verified.

**Local release evidence:**

- Linux Wails build: **passed**.
- Backup/restore into a disposable PostgreSQL database: **passed**.
- `scripts/verify.sh` with PostgreSQL: **passed**.
- Windows/macOS packaging, signing, notarization, checksums, and per-OS smoke
  tests: **blocked / not run**.

## 4. Controls verified

### 4.1 Loopback binding and port

- `main.go:65` constructs the server with `127.0.0.1:0`; `NewServer`
  independently rejects a non-loopback host
  (`internal/adapters/inbound/httpapi/server.go:21-28`), so a future caller
  cannot bind `0.0.0.0`.
- The base URL is derived from the live listener
  (`server.go:64-66`), not from config, so it always carries the ephemeral port.
- The API URL is not logged in the release path. `cmd/server` reads
  `DevMode` from config and logs the address only when dev mode is true
  (`cmd/server/main.go:63,79-81`, P3-4 resolved).
- Timeouts: read header 5 s, read 15 s, write 30 s, idle 60 s
  (`server.go:33-36`). Graceful shutdown with a 10 s drain and an idempotent
  guard (`main.go:77-85`, `app.go:32-38`).

### 4.2 Auth middleware and all 67 endpoints

Route enumeration from `router.go` matches `docs/API-CONTRACT.md`: 3 auth + 5
users + 18 catalog + 6 products + 5 clients + 5 providers + 2 supplier
catalogs + 9 purchase orders + 4 dashboard + 10 sales = **67**, plus
`GET /healthz`.

- `authMiddleware` is the only gate and wraps the whole mux
  (`router.go:128-135`). It requires `Bearer` on every `/api/v1` path except
  the exact `publicPaths` map (`middleware.go:26-30,115-139`).
- The whitelist is an exact map lookup after trimming one trailing slash; there
  is no prefix or `strings.HasPrefix` bypass. A path that does not start with
  `/api/v1` is passed through only to the catch-all 404 handler
  (`router.go:124-126`) and matches no business route. `ServeMux` cleans
  `..`/`//` paths to a redirect, so no traversal reaches a handler
  unauthenticated.
- `AuthenticateAccessToken` re-loads the user from the database on every
  request and rejects `!IsActive` (`internal/application/auth.go:125-140`), so
  a deleted or deactivated user loses access immediately even with a valid
  token.
- `requireSuperuser` reads the DB-backed user from the request context, not a
  token claim, and is applied to exactly the five `/users` routes
  (`router.go:58-62`, `users.go:159-168`). Every other route accepts any active
  user, matching `docs/BACKEND.md:310-311`. `TestUsersRequireSuperuser`
  covers all five methods (`users_test.go:192-220`).
- Each module has an auth test (unauthenticated `401`, garbage token `401`,
  active non-superuser not `403`): `client_integration_test.go:78,292-303`,
  `product_integration_test.go:148,502-529`, `sale_test.go:263,348-358`,
  `purchase_order_test.go:238,311-319`, `dashboard_test.go:73-86`,
  `catalog_test.go:162-172`, `provider_test.go:174`, `auth_integration_test.go:166-191`.

### 4.3 JWT, refresh rotation, issuer binding, logout

- HS256 is pinned and the expected issuer is required
  (`internal/adapters/outbound/security/jwt.go:46-53`).
- The issuer is `inventory-desktop/<backend>` built from the active store
  (`main.go:36`), so a token minted for the SQLite dataset is rejected by
  PostgreSQL and vice versa. `auth_issuer_test.go:79-88` asserts both
  directions.
- Access token carries `iss`, `sub`, `iat`, `exp`, `jti` (`jwt.go:35-41`).
  Default 30 min (`docs/ARCHITECTURE.md:179`).
- Refresh token: 32 `crypto/rand` bytes, base64url (`security/refresh.go:20-26`),
  stored only as a hex SHA-256 (`refresh.go:28-31`), unique-indexed
  (`migrations/postgres/00003_identity.sql:212-215`).
- Rotation is a single transaction that revokes the old row and inserts the new
  one, using `UPDATE ... RETURNING` so a concurrent replay cannot both succeed
  (`gormstore/refresh_tokens.go:53-86`).
- Logout revokes the presented refresh token (`application/auth.go:118-123`),
  and is called by the UI before clearing storage
  (`frontend/src/components/layout/Sidebar.tsx:51-52`,
  `frontend/src/pages/Home/Home.tsx:13-14`).
- A password change via `PUT /users/{id}` revokes **all** refresh tokens for
  that user (`internal/application/users.go:110-116`,
  `gormstore/refresh_tokens.go:98-105`). Access tokens remain valid until
  expiry (≤30 min), documented in `docs/SECURITY.md:63-66`.
- Refresh and access both re-check `IsActive`
  (`application/auth.go:95-97,137-139`).

### 4.4 CORS and body limits

- Exact-origin allowlist: `wails://wails` and `http://wails.localhost` always;
  `localhost`/`127.0.0.1` on `:5173`/`:4173` only when `DevMode`
  (`middleware.go:32-42,90-99`). `DevMode` comes from
  `APP_ENV=development` or `ALLOW_DEV_ORIGINS=true`
  (`config.go:118-122`), and `scripts/build.sh:13-14` unsets both for release.
- No wildcard, no `Access-Control-Allow-Credentials`, `Vary: Origin` set.
- Body limit 10 MiB enforced twice: global middleware
  (`middleware.go:108-113`) and `decodeJSON` (`response.go:32-43`). Oversize
  bodies map to `413 BODY_TOO_LARGE`.
- JSON decoding rejects unknown fields and trailing data
  (`response.go:34-42`), per ADR-0017.

### 4.5 Input validation

- Pagination: `skip >= 0`, `limit` 1–200, default 100, `search` ≤256 runes
  (`httpapi/users.go:170-193`), used by every list handler.
- Dashboard `limit` 1–100 and rejects empty/non-numeric
  (`httpapi/dashboard.go:95-108`).
- Enum filters (`payment_status`, `delivery_status`, `payment_method`,
  `stock_status`, order statuses) are closed switches; invalid values are `422`
  (`sale.go:498-522`, `purchase_order.go:393-408`).
- Dates parsed with `time.Parse(time.RFC3339)` and normalized to UTC
  (`sale.go:481-491`).
- Product money: positive, ≤`9999999999.99`, rounded half away from zero
  (`application/product.go:320-330`); stock/threshold non-negative
  (`product.go:138-146,232-248`).
- Password: 8–128 runes and ≤72 bytes (`application/users.go:133-141`); email
  normalized with `net/mail` (`users.go:124-131`).
- Brand `logo_url`: ≤255 chars and an absolute `http`/`https` URL with a host;
  `javascript:`, `data:`, `ftp:`, and relative values are rejected
  (`application/catalog.go:242-253`).
- Client-claimed totals, statuses, `created_by_id`, and stock are ignored or
  rejected; `created_by_id` on sale create is overwritten from the token
  (`httpapi/sale.go:28,220`). Client `created_by_id` on providers uses the
  context user.
- Duplicate/validation details are author-written; no DB error is echoed
  (`sale.go:401-424`, `purchase_order.go:338-358`, `catalog.go:280-294`,
  `users.go:142-157`).

### 4.6 SQL injection

- Every value is a bound parameter. Dynamic SQL only appends **static**
  condition strings and placeholder lists:
  `gormstore/stock.go:33-46` (placeholder count), `supplier_catalog.go:39-58`,
  `product.go:40-53`, `sale.go:169-185`, `purchase_order.go:46-61`.
- Search comparisons use `strpos(lower(col), lower(?)) > 0` with the term bound
  (`catalog.go:187`, `client.go:118`, `provider.go:102`); SQLite uses Go-side
  case folding. `%`/`_` literal behavior is documented (ADR-0015).
- User search uses a bound `LIKE` (`users.go:40-43`).
- No `fmt.Sprintf` interpolates a value into SQL; the one use builds `?,?,?`
  placeholders from a slice length.
- GORM logger is `Silent` on both backends (`db/postgres.go:18`,
  `db/sqlite.go:34`), so statement values are not logged.

### 4.7 Logs

- Request logging records `request_id`, `method`, `path`, `status`, `duration`
  only (`middleware.go:71-88`). Query strings, headers, and bodies are not
  logged, so `?search=` PII and `Authorization` never appear.
- Password hashes and tokens have no JSON tags (`domain/user.go`), and response
  DTOs are built explicitly (`httpapi/user_dto.go:5-21`).
- Error logs carry the error and request ID, not the request body
  (`auth.go:104,121`, handler `write*Error` functions).
- Panic recovery keeps the stack server-side and returns a generic `500`
  (`middleware.go:44-58`).

### 4.8 Secrets and configuration

- `SECRET_KEY` is generated from 32 `crypto/rand` bytes on first run and
  appended to `<UserConfigDir>/inventory-desktop/config.env`
  (`config.go:75-89,261-289`); a supplied value must be ≥32 bytes and is not a
  known placeholder (`config.go:179-191`).
- Directory `0700`, file `0600` on create and on read of an existing file
  (`config.go:224-235,261-281`). Symlink/non-regular config files are rejected.
- `.env`, `*.key`, `*.pem`, `*.sqlite`, dumps are gitignored
  (`.gitignore:31-45`). The only tracked env file is `.env.example`, and its
  `SECRET_KEY` is blank with a generation hint (`.env.example:8-12`).
- Repo-wide grep for `BEGIN ... PRIVATE KEY`, `ghp_`, and `AKIA` found nothing.
  The `postgresql://inventory:inventory@...` strings are the documented
  local-dev compose credentials (`.env.example:5`, `docs/ARCHITECTURE.md:176`,
  `internal/config/config.go:18`, `internal/config/config_test.go`), not
  production secrets. Dev-only; see F6.
- The release build does not require `VITE_API_URL`; bootstrap fails closed if
  the binding is absent in a production bundle (`bootstrap.ts:45-63`).

### 4.9 SQLite permissions and offline fallback

- Parent directory always tightened to `0700`, including when it already
  existed (`db/sqlite.go:24-30`).
- A symlink at the database path is rejected before open
  (`db/sqlite.go:20-23,54-66`).
- DB and any existing `-wal`/`-shm` are best-effort `0600` after `Ping`
  (`db/sqlite.go:46-51,68-74`).
- Foreign keys, 5 s busy timeout, WAL, single connection
  (`db/sqlite.go:32-45`).
- `DB_MODE=postgres` fails startup on connection error
  (`bootstrap/database.go:42-43,66-76`); `auto` logs a warning and falls back
  (`:46-60`). ADR-0021 keeps `auto` as the v1 default.
- The fallback is now visible: `OfflineDatabaseBanner` calls `/healthz` and
  renders a persistent, non-dismissable `role="status"` banner before login and
  on every screen when `database=sqlite`
  (`frontend/src/components/OfflineDatabaseBanner/OfflineDatabaseBanner.tsx:6-35`,
  mounted at `frontend/src/App.tsx:26`; tests in `frontend/src/App.test.tsx`).
- The risk is documented and accepted: unencrypted at rest, separate dataset,
  issuer prevents token crossover, backup with app stopped
  (`docs/SECURITY.md:119-139`, `docs/DECISIONS.md:342-360`).
- Residual gaps are carried in SQL-3 (P3-3) and SQL-7. P3-2 is closed by
  ADR-0021; encryption remains deferred.

### 4.10 Frontend session behavior

- Access/refresh tokens and a cached user live in `localStorage`
  (`AuthContext.tsx:43-45,58-75`) — accepted risk, ADR-0009, documented in
  `docs/SECURITY.md:75-87`.
- Base URL is set before the first render; a bootstrap failure renders an error
  and fires no request (`main.tsx:32-39`).
- Refresh is single-flight with a queue; on failure tokens are cleared and the
  app navigates to login (`lib/axios.ts:96-132`); login/refresh `401`s are not
  retried (`axios.ts:92-94`).
- No `dangerouslySetInnerHTML`, `innerHTML`, `eval`, `new Function`, or string
  timers in `frontend/src` (grep), and no external links (`href="http..."`).
- Manual logout calls the API first, then clears storage
  (`Sidebar.tsx:51-52`, `Home.tsx:13-14`); the token-expired listener clears
  locally, which is correct because the refresh token is already invalid.

## 5. Open questions and blockers

1. **Go dependency scan (still unverified locally).** `govulncheck` is not
   installed on this machine, so the Go graph has not been scanned locally. The
   new CI job installs pinned `govulncheck@v1.9.0` and runs `govulncheck ./...`
   (`.github/workflows/ci.yml:65-68`), but it has not run on GitHub yet.
   Confirm the first green run and share the output. Pinned versions reviewed:
   `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto`,
   `github.com/wailsapp/wails/v2`, `github.com/glebarez/sqlite`,
   `modernc.org/sqlite`, `gorm.io/gorm` (`go.mod`). Frontend direct versions
   after the Session 3 upgrade: `axios ^1.21.0`, `react ^19.2.7`,
   `react-router-dom ^7.18.5`, `vite ^8.3.5`, `zod ^4.6.7`
   (`frontend/package.json`).
2. **Config permissions — verified.** Manager output: `600 fabian
   ~/.config/inventory-desktop/config.env`, `700 fabian
   ~/.config/inventory-desktop`. No longer open.
3. **Tracked-secret check — verified.** Manager output: `git ls-files` found
   no tracked `.env`, `.key`, `.pem`, or `.sqlite` files. No longer open.
4. **Is `cmd/server` ever shipped? — Resolved.** It now reads `DevMode` from
   config and logs the address only in dev (`cmd/server/main.go:63,79-81`).
   Release artifacts remain the Wails build (`Makefile:30-31`), not this binary.
5. **Offline gate decision (P3-2). — Resolved by ADR-0021.** v1 keeps
   `DB_MODE=auto` and accepts unencrypted local storage with a persistent
   banner, `0600`/`0700` permissions, and sync out of scope. Encryption at rest
   is deferred and needs a future ADR (`docs/DECISIONS.md:342-360`). Residual
   risk is accepted, not eliminated.
6. **Should a password reset also invalidate access tokens? — Decided.**
   Refresh tokens are revoked (P3-1). Stateless access tokens remain valid for
   at most 30 minutes; this window is accepted and documented in
   `docs/SECURITY.md:63-66`. A token-version/`iat`-floor mechanism is not
   implemented.
7. **Last-superuser lockout.** `PUT /users/{id}` and `DELETE /users/{id}` allow
   a superuser to demote/delete themselves, leaving zero superusers
   (`internal/application/users.go:98-112`). Not a vulnerability; recovery
   depends on `cmd/seed`. Confirm the intended operator procedure.
8. **Backup/restore evidence. — Partially resolved.** Session 3 restored a
   backup into a disposable PostgreSQL database and it passed (see Session 3
   verification). Per-OS SQLite backup/restore with WAL sidecars is still not
   evidenced; confirm `VACUUM INTO`/the backup API and `-wal`/`-shm` handling
   on Windows and macOS.
9. **Devtools and navigation in the release artifact.** The code has no
   devtools option and `wails build` uses the production tag
   (`app.go:40-53`, `Makefile:30-31`), but there is no configured external
   navigation handler. Linux packaging succeeded in Session 3; the devtools and
   navigation checks in the built artifact remain blocked until per-OS smoke
   tests run.
10. **`.env` semantics.** Go does not read `.env`; only `scripts/*.sh` source
    it. Keep that explicit so operators do not expect a runtime `.env` on a
    double-click launch.

## 6. Dependency and secret scan

| Check | Result |
|---|---|
| Go modules pinned | Yes, direct versions in `go.mod:5-15`, `go.sum` present. |
| Frontend lockfile | `pnpm-lock.yaml` present; `package.json` uses caret ranges resolved by the lock. |
| `govulncheck` | **Not run locally — not installed.** CI job added with pinned `govulncheck@v1.9.0` (`.github/workflows/ci.yml:65-68`); **not yet run on GitHub**. Go graph unverified until then. |
| `pnpm audit --audit-level=high` and full `pnpm audit` | **0 known vulnerabilities** after the Session 3 upgrade (P3-8 resolved). |
| CI enforcement | `.github/workflows/ci.yml:53-88`: `go test`, `go test -race`, `go vet`, `gofmt`, `govulncheck` pinned to `v1.9.0`, `pnpm lint`/`test`/`build`, `pnpm audit --audit-level=high`. **Has not run on GitHub yet** (P3-5 mitigated). |
| Committed secrets | None. `git ls-files` found no `.env`, `.key`, `.pem`, or `.sqlite`; grep found no private keys or cloud tokens. `.env`/keys/certs/DB files ignored; only `.env.example` tracked, with a blank `SECRET_KEY`. |
| Config permissions | Verified: `600 fabian config.env`, `700 fabian` config directory. |
| Test-only secrets | `frontend/playwright.config.ts:30` uses `inventory:inventory@..._test`; test fixture only, must not be reused. |
| Release devtools | Production build path (`Makefile:30-31`). Linux artifact built; per-OS artifact inspection is still blocked (open question 9). |
| Signing/checksums | Documented only; not implemented. Windows/macOS packaging, signing, notarization, and checksums remain blocked (devops). |

## 7. Health score

| Area | Score | Rationale |
|---|---:|---|
| Auth | 9/10 | Fail-closed gate, DB-backed superuser check, issuer binding, hashed rotating refresh tokens, inactive-user rejection, password reset revokes all refresh tokens (P3-1), `exp` required (P3-6). Deductions: F7 timing, F8 limiter scope, F13 reuse detection. Residual access-token window ≤30 min is accepted and documented. |
| Input | 9/10 | 10 MiB cap, strict JSON, bounded pagination/search, closed enums, non-negative/limited money, parameterized SQL only, `logo_url` scheme allowlist (P3-7). Deduction: F19 content-type/Host. |
| API | 9/10 | All 67 routes behind auth except the four documented; exact CORS; loopback-only constructor; normalized errors; panic recovery; dev server derives `DevMode` (P3-4). Deductions: F6 default DSN, F19. |
| Storage | 7/10 | Config `0600`/`0700`, SQLite dir/sidecars tightened, refresh hashes at rest, persistent non-dismissable SQLite banner, ADR-0021 decision (P3-2/SQL-6). Deductions: unencrypted local PII remains by design, SQL-7 Windows ACLs, SQL-3 path validation (P3-3). |
| Dependencies | 7/10 | Frontend audit is clean and the packages are upgraded (P3-8 resolved); CI added with pinned `govulncheck@v1.9.0` and audit gates (P3-5 mitigated). Deductions: `govulncheck` not run locally, CI not yet run on GitHub, signing not implemented. |
| Desktop | 7/10 | Loopback guard, ephemeral port, fail-closed bootstrap, no XSS sinks, hash router, Linux build passed. Deductions: remote font CSS and no CSP (F14), navigation/devtools unverified in a release artifact, Windows/macOS packaging untested. |
| **Overall** | **8/10** | No open Critical/High. P3-1, P3-4, P3-6, P3-7, and P3-8 are resolved; P3-2 is resolved by decision/mitigation with accepted residual risk; P3-5 is mitigated by CI. Release is conditional on the first green CI run, Windows/macOS packaging/signing, and final per-OS smoke tests. |

**Remaining fix order:** P3-3 (SQLite path) → F6 (default DSN) → SQL-7 (Windows
permissions/roaming) → F8/F13/F14/F15/F19 → F7/F20. Release gates are separate:
first green CI run, Windows/macOS packaging + signing/notarization + checksums,
and per-OS smoke tests.

## 8. Handoffs

Completed and removed: P3-1/P3-4/P3-6/P3-7 backend handoff, P3-2 banner
handoff, P3-8 frontend upgrade handoff, and the P3-2 decision handoff. Remaining
work:

```text
HANDOFF
Target: backend
Reason: `SQLITE_PATH` still chmods an arbitrary parent and interpolates the path into the DSN (P3-3).
Files: internal/db/sqlite.go, internal/config/config.go
Change: Only create/tighten a directory the app owns under
        `os.UserConfigDir()`; require an absolute, cleaned `SQLITE_PATH`;
        `Lstat` each path component and refuse symlinks/non-regular files/dirs
        not owned by the current user; build the DSN with `net/url`.
Acceptance: `SQLITE_PATH=/tmp/x.sqlite` fails without touching /tmp; a symlinked
            parent is rejected; a `?` in the path cannot add DSN parameters;
            existing SQLite tests still pass.
Blocked: no
```

```text
HANDOFF
Target: devops
Reason: The CI workflow has not run on GitHub, and Windows/macOS packaging, signing, notarization, checksums, and per-OS smoke tests are still blocked.
Files: .github/workflows/**, build/**, wails.json, docs/RELEASE.md
Change: Push the CI workflow and confirm `govulncheck ./...`, `go test`,
        `go test -race`, and `pnpm audit --audit-level=high` pass on GitHub.
        Produce Windows/macOS packages, sign/notarize where supported, publish
        checksums, and complete the release checklist.
Acceptance: CI is green on the default branch; signed/notarized artifacts and
            checksums exist for each supported OS; `docs/RELEASE.md` records
            the artifact hashes.
Blocked: no
```

```text
HANDOFF
Target: qa
Reason: Per-OS release smoke tests are required and have not run.
Files: frontend/e2e/**, tests/** (test evidence only)
Change: On each packaged OS artifact, verify loopback-only bind with an
        ephemeral port, login/refresh/logout, password-reset revocation,
        the non-dismissable SQLite banner, devtools closed, and external
        navigation blocked.
Acceptance: a smoke-test record per OS is attached to the release checklist;
            any failure blocks release.
Blocked: no
```
