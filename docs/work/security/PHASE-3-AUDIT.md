# Security review — Phase 3 implementation (desktop Go API surface)

**Scope:** the implemented Phase 3 surface — loopback server, auth middleware,
all 67 routes, superuser Users routes, JWT/refresh lifecycle, CORS and body
limits, input validation, SQL access, logging/errors, config and SQLite
storage, and the React session flow.

**Method:** static review only. No shell commands were run (role denies shell),
no code, tests, migrations, or configuration were modified. Prior reports
`docs/work/security/phase-2-auth-review.md` and
`docs/work/security/phase-2-sqlite-security-review.md` are treated as the
baseline; their IDs are re-checked, not re-counted.

**Date:** 2026-10-29.

## Tool results (manager-run)

The manager ran the checks this review could not run (the security role denies
shell access). These results are folded into the verdict and findings below.

| Check | Command | Result |
|---|---|---|
| Go vulnerability scan | `govulncheck ./...` | **Not run — not installed** on this machine (`command not found`). The Go dependency scan remains **unverified**. |
| Frontend dependency audit | `pnpm audit --audit-level=high` in `frontend/` | **48 vulnerabilities**: 24 high, 23 moderate, 1 low. High-severity packages: `axios` (patched `>=1.20.0`), `react-router` (`>=7.18.2`), `vite` (`>=8.1.6`), `zod` (`>=4.6.6`), `postcss` (`>=8.5.18`/`>=8.5.23` depending on advisory), `brace-expansion`, `browserslist`, `follow-redirects`, `nanoid`, `source-map-js`. |
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

Three findings are release gates, not hardening. One is **High** and comes
from the manager-run dependency audit:

1. **24 High-severity frontend dependency advisories** (`P3-8`). `pnpm audit`
   reports 24 high, 23 moderate, and 1 low vulnerability. Direct packages
   `axios` (`frontend/package.json:17`), `react-router-dom` (`:23`), `zod`
   (`:25`), and `vite` (`:45`) are affected, along with transitive packages
   (`postcss`, `brace-expansion`, `browserslist`, `follow-redirects`,
   `nanoid`, `source-map-js`). These ship in the WebView bundle, which holds
   the tokens.
2. **Password change does not revoke sessions** (`P3-1`, Medium). Now that
   superuser user management exists, resetting a compromised user's password
   leaves the attacker's refresh token alive for up to seven days, and refresh
   rotation slides that window indefinitely.
3. **Customer PII and pricing now live in an unencrypted SQLite file, and the
   fallback is invisible in the UI** (`P3-2`, Medium). This crosses the SQL-6
   "Phase 3 gate" from the SQLite review without an encryption decision or an
   operator signal.

None of the auth/data findings is remotely exploitable from the network; they
are real data-protection failures under the documented threat model (stolen
disk / local process / silent divergence / malicious dependency). Everything
else is Low defense-in-depth or carried forward from Phase 2.

## 1. Verdict

**One open High release gate: 24 High-severity frontend dependency
vulnerabilities (`P3-8`).** No Critical finding, no auth bypass, no SQL
injection, no committed secret, no token or PII in logs, no wildcard CORS, and
no unauthenticated business endpoint were found. The Go dependency scan is
still **unverified** because `govulncheck` is not installed.

**Not release-ready:** `P3-8` (High) must be remediated and re-audited; `P3-1`
and `P3-2` (Medium) must have an owner and a verified fix. `P3-2` also needs a
product/manager decision (accept unencrypted local PII, or gate offline mode).
Config permissions (`0600`/`0700`) and the tracked-secret check passed.

## 2. Findings

Severity follows `docs/SECURITY.md:192-200`. "Type" separates a vulnerability
from a hardening suggestion.

### New in Phase 3

| ID | Severity | Type | Location | Issue | Impact | Remediation | Owner |
|---|---|---|---|---|---|---|---|
| P3-1 | **Medium** | Vulnerability (session lifecycle) | `internal/application/users.go:85-94`; `internal/adapters/inbound/httpapi/users.go:114-120`; `internal/application/auth.go:104`; `internal/adapters/outbound/persistence/gormstore/refresh_tokens.go:53-86` | `UserService.Update` replaces `HashedPassword` but never revokes that user's refresh tokens. `AuthService.Refresh` issues a new token with `now.Add(refreshTTL)` on every rotation, so a session can live forever. | After an admin resets a compromised account's password, the attacker's refresh token still works (access ≤30 min, refresh up to 7 days and sliding). Deactivating the user does stop both paths, so the fix is specifically for password resets. | Add `RevokeAllForUser(ctx, userID)` to `outbound.RefreshTokenRepository` (GORM: `UPDATE refresh_tokens SET revoked = true WHERE user_id = ? AND revoked = false`) and call it from `UserService.Update` whenever `input.Password != nil && *input.Password != ""`, immediately after `users.Update` succeeds. `UserService` has no unit of work today; if the two writes must be atomic, route the update through `outbound.UnitOfWork`. Keep the existing `is_active` check as the second control. | backend |
| P3-2 | **Medium** | Vulnerability (data at rest, Phase 3 gate) | `migrations/sqlite/00002_catalog.sql`–`00006_sales.sql`; `docs/SECURITY.md:117-137`; `internal/adapters/inbound/httpapi/router.go:52-54`; `internal/bootstrap/database.go:59-60`; `frontend/src/**` (no `/healthz` consumer) | The SQLite schema now stores clients (identity card, email, phone), product costs, prices, and stock. The file is not encrypted (`docs/SECURITY.md:126-128`), `DB_MODE` defaults to `auto`, and the only fallback signal is a stderr warning. `GET /healthz` reports `database` but the React app never calls it (grep of `frontend/src` for `healthz`/`sqlite` returns nothing), so `docs/SECURITY.md:52-54`'s "visible to the client" is not true today. | A stolen disk, backup, or roaming profile exposes customer PII and margins. A PostgreSQL outage silently moves writes to the local file; the user keeps entering PII believing it is in the shared database, with no sync (`docs/SYNC.md`). | (a) Release default `DB_MODE=postgres`, or require `ALLOW_OFFLINE_FALLBACK=true` for `auto`. (b) Make the client consume `/healthz` and show a persistent, non-dismissable banner when `database=sqlite`, before login. (c) Record the encryption/backup decision in `docs/DECISIONS.md` (SQLite encryption or a written acceptance), and require an explicit backend for `cmd/seed`. | frontend (banner) + backend (a, seed) + manager (c) |
| P3-3 | Low | Hardening (destructive side effect) | `internal/db/sqlite.go:24-32` | The parent directory of `SQLITE_PATH` is unconditionally `MkdirAll`'d and `Chmod`'d to `0700`. `dir` is not `Lstat`'d, so a symlinked or shared parent (for example `SQLITE_PATH=/tmp/inv.sqlite`) gets its permissions changed. The path is also interpolated into the DSN at `sqlite.go:32` without escaping, so a `?` in the path can add connection parameters. | A configuration mistake silently tightens an unrelated directory (`/tmp`, a shared work folder), breaking other users/processes. A symlinked parent redirects the chmod/DB, and a crafted path can set `_pragma`/`mode` options. Not remotely reachable. | Only create/tighten a directory the app owns (under `os.UserConfigDir()`); require an absolute, cleaned `SQLITE_PATH`; `Lstat` each path component and refuse symlinks, non-regular files, or a directory not owned by the current user; build the DSN with `net/url` instead of `fmt.Sprintf`. | backend |
| P3-4 | Low | Hardening (release hygiene) | `cmd/server/main.go:63,70,79` | The dev API binary hardcodes `DevMode: true` and logs the listen address. | If this binary is ever shipped or used as the desktop backend, the Vite/Playwright origins stay allowed and the port is advertised — both contradict `docs/SECURITY.md:46-50`. | Derive `DevMode` from `config.Config` (as `main.go:63` does) and guard the address log with dev mode; keep `cmd/server` out of release artifacts. | backend + devops |
| P3-5 | Low | Hardening (supply chain) | No `.github/` directory; `Makefile:33-36`; `docs/SECURITY.md:158-164` | Dependency vulnerability scans are required before release but nothing in the repo runs them; no CI at all. | A new Critical/High advisory in Wails, pgx, `modernc.org/sqlite`, or a React package ships undetected. | Add a CI job that installs `govulncheck` and runs `govulncheck ./...`, `go list -m -u all`, and `pnpm audit --audit-level=high`, failing on Critical/High. | devops |
| P3-6 | Low | Hardening | `internal/adapters/outbound/security/jwt.go:46-62` | `Parse` validates `exp` when present but does not require it (`jwt.WithExpirationRequired()` is absent). Issued tokens always set `exp` (`jwt.go:38`), so exploitation requires the signing key to leak. | A forged non-expiring token would be accepted if `SECRET_KEY` is ever exposed; defense-in-depth only. | Add `jwt.WithExpirationRequired()` and a unit test that a token without `exp` is rejected. | backend |
| P3-7 | Low | Hardening (latent XSS sink) | `internal/application/catalog.go:147`; `frontend/src/pages/Inventory/Inventory.tsx:58` | `logo_url` is validated for length (255) only; any string, including `javascript:` or `data:`, is stored. Today it is rendered as plain text in a table, so there is no sink. | A future link/image render of `logo_url` turns stored input into XSS in the token-bearing origin, which has no CSP (F14). | Reject non-`http`/`https` schemes in `cleanOptional` for `logo_url` (or a dedicated validator) and test `javascript:alert(1)` → `422`. | backend |
| P3-8 | **High** | Vulnerability (vulnerable dependency) | `frontend/package.json:17,23,25,45`; `frontend/pnpm-lock.yaml`; `docs/SECURITY.md:158-164` | `pnpm audit --audit-level=high` reports **24 high**, 23 moderate, and 1 low vulnerability. Direct packages `axios` (`:17`, patched `>=1.20.0`), `react-router-dom` (`:23`, `>=7.18.2`), `zod` (`:25`, `>=4.6.6`), and `vite` (`:45`, `>=8.1.6`) are affected; `postcss` needs `>=8.5.18`/`>=8.5.23` depending on advisory, and transitive `brace-expansion`, `browserslist`, `follow-redirects`, `nanoid`, and `source-map-js` are affected. | A known-High advisory is compiled into the shipped WebView bundle, which has access to the `localStorage` tokens (`docs/SECURITY.md:75-87`). Exploitability depends on the specific advisory, but `docs/SECURITY.md:197` classifies this class as release-blocking, and the audit gate is not met. | Upgrade the direct dependencies and re-resolve transitive ones: `axios >=1.20.0`, `react-router-dom >=7.18.2` (`react-router`), `zod >=4.6.6`, `vite >=8.1.6`, and `postcss >=8.5.23`. Run `pnpm update`/`pnpm dedupe` as needed, then `pnpm audit --audit-level=high` and the frontend test/build suite. Do not add overrides unless the advisory cannot be resolved by upgrading. | frontend (upgrade) + devops (CI gate) |

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
| F12 72-byte password | Low | **Partially fixed** | `internal/application/users.go:123-131` enforces 8–128 chars and ≤72 bytes; `docs/API-CONTRACT.md:74` still says only "8–128 characters". |
| F13 refresh reuse/family | Low | **Open** | `internal/application/auth.go:99-109`; `internal/adapters/outbound/persistence/gormstore/refresh_tokens.go:53-86` revokes only the presented token; expiry slides. |
| F14 remote fonts / no CSP | Low | **Open** | `frontend/src/index.css:6-7`, `frontend/index.html:8-9`; no `Content-Security-Policy` anywhere. |
| F15 trusted `X-Request-Id` | Low | **Open** | `internal/adapters/inbound/httpapi/middleware.go:62-67`. |
| F16 token responses cacheable | Low | **Resolved** | `Cache-Control: no-store` + `Pragma` on login/refresh/logout (`internal/adapters/inbound/httpapi/auth.go:40-41,73-74,95-96`). |
| F17 JWT claims | Low | **Resolved** | `iss`, `iat`, `jti` in `internal/adapters/outbound/security/jwt.go:35-41`; `WithIssuer` + HS256 pinning at `:51-52`. No TTL clamp yet. |
| F18 loopback constructor | Low | **Resolved** | `internal/adapters/inbound/httpapi/server.go:21-28` rejects any non-loopback host. |
| F19 Host/Origin/content-type | Low | **Open** | `internal/adapters/inbound/httpapi/middleware.go:90-105`; `internal/adapters/inbound/httpapi/response.go:32-43`. |
| F20 user PII in `localStorage` | Low | **Open** | `frontend/src/context/AuthContext.tsx:16-40,58-66`. |
| SQL-3 `SQLITE_PATH` validation | Low | **Open** | `internal/db/sqlite.go:24-32`; `internal/config/config.go:146-152` (see P3-3). |
| SQL-6 unencrypted local PII | Medium | **Open — now active** | See P3-2. |
| SQL-7 Windows permissions/roaming | Medium | **Open** | `internal/db/sqlite.go:28,68-74`; `internal/config/config.go:193-207`. |

## 3. Controls verified

### 3.1 Loopback binding and port

- `main.go:65` constructs the server with `127.0.0.1:0`; `NewServer`
  independently rejects a non-loopback host
  (`internal/adapters/inbound/httpapi/server.go:21-28`), so a future caller
  cannot bind `0.0.0.0`.
- The base URL is derived from the live listener
  (`server.go:64-66`), not from config, so it always carries the ephemeral port.
- The API URL is not logged in the release path; only the dev binary logs it
  (`cmd/server/main.go:79`, P3-4).
- Timeouts: read header 5 s, read 15 s, write 30 s, idle 60 s
  (`server.go:33-36`). Graceful shutdown with a 10 s drain and an idempotent
  guard (`main.go:77-85`, `app.go:32-38`).

### 3.2 Auth middleware and all 67 endpoints

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

### 3.3 JWT, refresh rotation, issuer binding, logout

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
- Refresh and access both re-check `IsActive`
  (`application/auth.go:95-97,137-139`).

### 3.4 CORS and body limits

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

### 3.5 Input validation

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
- Password: 8–128 runes and ≤72 bytes (`application/users.go:123-131`); email
  normalized with `net/mail` (`users.go:114-121`).
- Client-claimed totals, statuses, `created_by_id`, and stock are ignored or
  rejected; `created_by_id` on sale create is overwritten from the token
  (`httpapi/sale.go:28,220`). Client `created_by_id` on providers uses the
  context user.
- Duplicate/validation details are author-written; no DB error is echoed
  (`sale.go:401-424`, `purchase_order.go:338-358`, `catalog.go:280-294`,
  `users.go:142-157`).

### 3.6 SQL injection

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

### 3.7 Logs

- Request logging records `request_id`, `method`, `path`, `status`, `duration`
  only (`middleware.go:71-88`). Query strings, headers, and bodies are not
  logged, so `?search=` PII and `Authorization` never appear.
- Password hashes and tokens have no JSON tags (`domain/user.go`), and response
  DTOs are built explicitly (`httpapi/user_dto.go:5-21`).
- Error logs carry the error and request ID, not the request body
  (`auth.go:104,121`, handler `write*Error` functions).
- Panic recovery keeps the stack server-side and returns a generic `500`
  (`middleware.go:44-58`).

### 3.8 Secrets and configuration

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

### 3.9 SQLite permissions and offline fallback

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
  (`:46-60`).
- The risk is documented: unencrypted at rest, separate dataset, issuer prevents
  token crossover, backup with app stopped (`docs/SECURITY.md:117-137`).
- Residual gaps are carried in SQL-3/SQL-7 and now P3-2.

### 3.10 Frontend session behavior

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

## 4. Open questions and blockers

1. **Go dependency scan (still a blocker for the Dependencies score).**
   `govulncheck` is not installed, so the Go graph is **unverified**. Once
   installed, run `govulncheck ./...` and `go list -m -u all` and share the
   output. Pinned versions reviewed: `github.com/golang-jwt/jwt/v5 v5.3.1`,
   `golang.org/x/crypto v0.58.0`, `github.com/wailsapp/wails/v2 v2.16.0`,
   `github.com/glebarez/sqlite v1.11.0`, `modernc.org/sqlite v1.60.1`,
   `gorm.io/gorm v1.31.2`, `axios ^1.16.1`, `react ^19.2.7`,
   `react-router-dom ^7.16.0` (`go.mod`, `frontend/package.json`).
2. **Config permissions — verified.** Manager output: `600 fabian
   ~/.config/inventory-desktop/config.env`, `700 fabian
   ~/.config/inventory-desktop`. No longer open.
3. **Tracked-secret check — verified.** Manager output: `git ls-files` found
   no tracked `.env`, `.key`, `.pem`, or `.sqlite` files. No longer open.
4. **Is `cmd/server` ever shipped?** If yes, P3-4 is a real release exposure.
5. **Offline gate decision (P3-2).** Does the first release run with
   `DB_MODE=auto` and accept unencrypted local PII, or default to `postgres`?
   The answer changes the release checklist and the UI requirement.
6. **Should a password reset also invalidate access tokens?** P3-1 fixes refresh
   tokens; access tokens remain valid ≤30 min. Accept that, or add a token
   version/`jwt` `iat` floor. Document the choice.
7. **Last-superuser lockout.** `PUT /users/{id}` and `DELETE /users/{id}` allow
   a superuser to demote/delete themselves, leaving zero superusers
   (`internal/application/users.go:98-112`). Not a vulnerability; recovery
   depends on `cmd/seed`. Confirm the intended operator procedure.
8. **Backup/restore evidence.** `docs/RELEASE.md` requires a tested restore;
   no test evidence is in the repo. Also confirm the documented SQLite backup
   is WAL-aware and that `-wal`/`-shm` handling is verified.
9. **Devtools and navigation in the release artifact.** The code has no
   devtools option and `wails build` uses the production tag
   (`app.go:40-53`, `Makefile:30-31`), but there is no configured external
   navigation handler. Verify in the built binary that devtools are closed and
   that navigating to an external URL does not leave the app origin.
10. **`.env` semantics.** Go does not read `.env`; only `scripts/*.sh` source
    it. Keep that explicit so operators do not expect a runtime `.env` on a
    double-click launch.

## 5. Dependency and secret scan

| Check | Result |
|---|---|
| Go modules pinned | Yes, direct versions in `go.mod:5-15`, `go.sum` present. |
| Frontend lockfile | `pnpm-lock.yaml` present; `package.json` uses caret ranges resolved by the lock. |
| `govulncheck` | **Not run — not installed** (`command not found`). Go graph unverified. |
| `pnpm audit --audit-level=high` | **48 vulnerabilities: 24 high, 23 moderate, 1 low** (P3-8). Highs include `axios`, `react-router`, `vite`, `zod`, `postcss`, `brace-expansion`, `browserslist`, `follow-redirects`, `nanoid`, `source-map-js`. |
| CI enforcement | None — no `.github/` workflow (P3-5). |
| Committed secrets | None. `git ls-files` found no `.env`, `.key`, `.pem`, or `.sqlite`; grep found no private keys or cloud tokens. `.env`/keys/certs/DB files ignored; only `.env.example` tracked, with a blank `SECRET_KEY`. |
| Config permissions | Verified: `600 fabian config.env`, `700 fabian` config directory. |
| Test-only secrets | `frontend/playwright.config.ts:30` uses `inventory:inventory@..._test`; test fixture only, must not be reused. |
| Release devtools | Production build path (`Makefile:30-31`); verify the artifact (open question 9). |
| Signing/checksums | Documented only; not implemented (devops). |

## 6. Health score

| Area | Score | Rationale |
|---|---:|---|
| Auth | 8/10 | Fail-closed gate, DB-backed superuser check, issuer binding, hashed rotating refresh tokens, inactive-user rejection. Deductions: P3-1 password reset, F7 timing, F8 limiter scope, F13 reuse detection. |
| Input | 8/10 | 10 MiB cap, strict JSON, bounded pagination/search, closed enums, non-negative/limited money, parameterized SQL only. Deductions: F19 content-type/Host, P3-7 `logo_url` scheme. |
| API | 8/10 | All 67 routes behind auth except the four documented; exact CORS; loopback-only constructor; normalized errors; panic recovery. Deductions: F6 default DSN, P3-4 dev binary, F19. |
| Storage | 5/10 | Config now `0600`/`0700` and SQLite dir/sidecars tightened; refresh hashes at rest. Deductions: unencrypted business PII now present with no decision (P3-2/SQL-6), invisible fallback, SQL-7 Windows, SQL-3 path validation. |
| Dependencies | 3/10 | Pinned and current-looking Go stack, but `pnpm audit` reports 24 High advisories in the shipped frontend (P3-8); `govulncheck` is not installed, so the Go graph is unverified; no CI enforcement; signing not implemented. |
| Desktop | 7/10 | Loopback guard, ephemeral port, fail-closed bootstrap, no XSS sinks, hash-router-free of remote navigation. Deductions: remote font CSS and no CSP (F14), navigation lockdown unverified, devtools unverified in artifact. |
| **Overall** | **5/10** | One open High release gate (`P3-8`, 24 vulnerable frontend packages), two Medium release gates (`P3-1`, `P3-2`), and the carried F6. No Critical. Safe to continue Phase 3 development, but **do not ship** until P3-8 is upgraded and re-audited, P3-1 is fixed, and P3-2 has a signed-off decision plus a UI signal. |

**Fix order:** P3-8 (dependency upgrade + re-audit) → P3-1 → P3-2 (decision +
banner/opt-in) → F6 → P3-3/P3-4 → F8/F13/F14 → remaining Lows.

## 7. Handoffs

```text
HANDOFF
Target: backend
Reason: Password change must revoke the user's refresh tokens; dev server must not hardcode DevMode; SQLite path must not chmod arbitrary directories.
Files: internal/application/users.go, internal/ports/outbound/refresh_tokens.go,
       internal/adapters/outbound/persistence/gormstore/refresh_tokens.go,
       cmd/server/main.go, internal/db/sqlite.go, internal/config/config.go,
       internal/adapters/outbound/security/jwt.go,
       internal/application/catalog.go
Change: P3-1, P3-3, P3-4, P3-6, P3-7 as specified in the findings table.
Acceptance: new integration test proves a password reset invalidates old
            refresh tokens; `SQLITE_PATH=/tmp/x.sqlite` fails without touching
            /tmp; `cmd/server` reads DevMode from config; token without `exp`
            is rejected; `logo_url: javascript:...` returns 422.
Blocked: no
```

```text
HANDOFF
Target: frontend
Reason: The active database backend is never surfaced, so a silent SQLite fallback with local PII is invisible.
Files: frontend/src/lib/api/**, frontend/src/App.tsx,
       frontend/src/components/layout/**
Change: Call `GET /healthz` after bootstrap (unauthenticated) and render a
        persistent banner when `database=sqlite`, placed so it is visible on
        the login screen and in the app shell. Do not gate it on auth.
Acceptance: with PostgreSQL down, `DB_MODE=auto` shows the banner before login;
            with PostgreSQL up it does not render; no console errors.
Blocked: no
```

```text
HANDOFF
Target: frontend
Reason: 24 High-severity dependency advisories ship in the WebView bundle (P3-8).
Files: frontend/package.json, frontend/pnpm-lock.yaml
Change: Upgrade `axios` to `>=1.20.0`, `react-router-dom` to `>=7.18.2`
        (`react-router`), `zod` to `>=4.6.6`, `vite` to `>=8.1.6`, and
        `postcss` to `>=8.5.23`; re-resolve transitive `brace-expansion`,
        `browserslist`, `follow-redirects`, `nanoid`, and `source-map-js`
        (`pnpm update`/`pnpm dedupe`). Do not add overrides unless a package
        cannot be fixed by upgrading.
Acceptance: `cd frontend && pnpm audit --audit-level=high` reports 0 High;
            `pnpm test`, `pnpm build`, and `pnpm lint` pass; no direct
            dependency is downgraded.
Blocked: no
```

```text
HANDOFF
Target: devops
Reason: Dependency scanning is required by docs/SECURITY.md but no CI exists, and `govulncheck` is not installed on the build machine.
Files: .github/workflows/**, Makefile
Change: Add a CI job that installs a pinned `govulncheck`
        (`go install golang.org/x/vuln/cmd/govulncheck@latest` in a pinned
        toolchain step), runs `govulncheck ./...` and
        `cd frontend && pnpm audit --audit-level=high`, and fails on
        Critical/High. Keep the release binary as the Wails build, not
        `cmd/server`.
Acceptance: `govulncheck` runs in CI and fails on a seeded known-vulnerable Go
            dependency; the `pnpm audit` gate fails while P3-8 is open and
            passes after the frontend upgrade; `git ls-files` shows no
            env/key/db files.
Blocked: no
```

```text
HANDOFF
Target: manager
Reason: P3-2 needs a product/decision owner for unencrypted local PII and the offline fallback default.
Files: docs/DECISIONS.md, docs/RELEASE.md, docs/SECURITY.md
Change: Record the decision: release default `DB_MODE=postgres` (or explicit
        `ALLOW_OFFLINE_FALLBACK`), and accept-or-encrypt SQLite at rest, with a
        WAL-aware backup and restore procedure.
Acceptance: decision entry approved; release checklist references it; the
            frontend banner requirement is scheduled.
Blocked: no
```
