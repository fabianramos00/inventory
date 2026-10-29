# Security

The desktop app handles prices, costs, customer data, and staff credentials.
Security is a release gate, not a later hardening pass.

## Threat model

### Assets

- User passwords and password hashes.
- Access and refresh tokens.
- Customer names, identity cards, emails, and phones.
- Product costs, prices, and stock.
- PostgreSQL credentials and the JWT signing secret.

### Trust boundaries

```text
WebView (React) ── HTTP loopback ── Go API ── PostgreSQL
        │                              │
        └── localStorage tokens        └── config file / env
```

1. **WebView ↔ Go API**: untrusted input crosses here.
2. **Go API ↔ PostgreSQL**: credentialed connection; SQL must be
   parameterized.
3. **Local machine ↔ app**: other local processes can reach the loopback
   port.
4. **App ↔ OS config**: secrets live here.

### Adversaries

- A malicious local process scanning loopback ports.
- A compromised or malicious web page open in another browser (cannot read
  the API without a token, but can attempt requests).
- An attacker with code execution in the WebView (XSS).
- An attacker with filesystem access to the config directory.
- A malicious or vulnerable dependency.

## Controls

### Network

- Bind to `127.0.0.1` only. Never `0.0.0.0`, never a LAN address.
- Use an OS-assigned ephemeral port.
- Do not print the API URL in release logs.
- CORS always allows the Wails webview origin. The Vite `:5173` and Playwright
  `:4173` origins are allowed only when dev mode is enabled
  (`APP_ENV=development` or `ALLOW_DEV_ORIGINS=true`). Do not use `*` with
  credentials.
- Close the HTTP server on app shutdown.
- `GET /healthz` is unauthenticated and returns no business data. It reports
  the active database backend (`{"status":"ok","database":"postgres|sqlite"}`)
  so a fallback is visible to the client.

### Authentication

- Bearer JWT, HS256, 30-minute access token.
- Access tokens carry `iss`, `iat`, and a random `jti`. The issuer is bound to
  the backend (`inventory-desktop/<backend>`), and parsing requires the
  expected issuer and HS256. A token minted for the SQLite dataset is rejected
  by PostgreSQL and vice versa, even when user IDs collide.
- Refresh token: 32 random bytes, stored as a SHA-256 hash, rotated on every
  use, revoked on logout, and all revoked for a user when an admin changes
  that user's password. Access tokens issued before the reset remain valid
  until they expire (at most 30 minutes) because they are stateless.
- bcrypt cost 12 in production; 4 only in tests.
- Login rate limit: 5 attempts per minute per client.
- Reject inactive users on login and on every authenticated request.
- All `/users` routes require `is_superuser`.
- `SECRET_KEY` is generated per install or supplied explicitly. It must be at
  least 32 bytes; known placeholders are rejected. It is never hardcoded and
  never committed.
- Passwords are 8–128 characters and at most 72 bytes (bcrypt limit). Never
  log or return them.

### Token storage

- Access and refresh tokens live in WebView `localStorage` for v1, matching
  the legacy app.
- This is an accepted risk: any XSS can steal both tokens.
- Mitigations:
  - No `dangerouslySetInnerHTML` or unsanitized HTML.
  - No third-party scripts.
  - Restrict navigation to the app origin.
  - Short access-token lifetime and refresh rotation.
  - Security review for any new HTML injection point.
- OS keyring storage is the documented upgrade path in
  [DECISIONS.md](DECISIONS.md).

### Input and output

- Validate every request server-side with explicit Go checks.
- Enforce body size limits (10 MiB) and numeric bounds.
- Use parameterized SQL only; never string-build queries.
- Encode JSON with the standard encoder; do not echo raw errors.
- Reject unknown or malformed enum values.
- Cap `search` at 256 characters and `limit` at 200.
- Never trust client-computed totals, statuses, prices, or stock.

### Secrets and configuration

- `.env`, DSNs, tokens, keys, and database dumps are gitignored.
- The OS config file is created with owner-only permissions (`0600`).
- Never log the DSN, `SECRET_KEY`, passwords, authorization headers, or full
  request bodies.
- The PostgreSQL account should have only the privileges the app needs.
- Production uses a strong, unique database password.

### Database

- PostgreSQL listens on localhost or a private network, never the public
  internet.
- TLS for remote connections.
- Backups are encrypted and access-controlled.
- Migrations run with a dedicated role where possible.
- No `echo`/query logging in production.

### Local SQLite

- The SQLite parent directory is always tightened to `0700`, including when it
  already exists.
- A symlink at the database path is rejected before opening.
- The database file and any existing `-wal`/`-shm` sidecars are best-effort
  set to `0600` after open.
- These modes are Unix permissions. Windows relies on the per-user profile
  directory because `os.Chmod` does not set an ACL.
- SQLite is **not encrypted at rest**. Local business data must be protected by
  OS account separation and full-disk encryption. Encryption is not
  implemented.
- The offline dataset is separate from PostgreSQL. A user or token from one
  backend does not imply that the same user exists in the other. The JWT
  issuer binding prevents token crossover; local users must be provisioned
  per backend.
- `DB_MODE=auto` can hide a misconfigured PostgreSQL DSN. The fallback logs a
  warning and `/healthz` reports the active backend. Use `DB_MODE=postgres`
  when the deployment must fail hard.
- Back up the SQLite file with the app stopped, or use SQLite's backup API.
  Copying a live WAL database without its sidecars can lose data.

### Logging and errors

- `slog` with request ID, method, path, status, and duration.
- No PII, tokens, or secrets.
- Client errors return `{detail, code}`; stack traces stay in logs.
- A panic is recovered and returns `500` without internals.

### Desktop and WebView

- Disable devtools and remote debugging in release builds.
- Disable arbitrary external navigation; open links in the OS browser.
- Keep the Wails and Go dependencies current.
- Do not load remote content in the webview.
- Set a Content Security Policy in `index.html` when the build pipeline
  supports it. At minimum: no remote scripts, no inline scripts, no
  `unsafe-eval`.
- Treat the webview as a browser: no `eval`, `new Function`, or string
  timers.

### Dependencies

- Pin versions in `go.mod` and `pnpm-lock.yaml`.
- Run `go list -m -u all`, `pnpm audit`, and a Go vulnerability scan before
  release.
- A new dependency needs a reason in [DECISIONS.md](DECISIONS.md).
- Remove unused packages promptly.

### Update and distribution

- Builds are produced from a clean tree.
- Release artifacts are signed where the platform supports it.
- Checksums are published with the installer.
- Auto-update, if added, must verify signatures. It is out of scope for v1.

## Audit checklist

Latest Phase 3 audit: [SECURITY-AUDIT.md](SECURITY-AUDIT.md).

- [ ] Auth flows: login, refresh rotation, logout, inactive user, superuser.
- [ ] All 67 endpoints require auth except the documented four.
- [ ] No endpoint trusts client totals or statuses.
- [ ] Stock changes are transactional and row-locked.
- [ ] No raw SQL, `eval`, or `dangerouslySetInnerHTML`.
- [ ] No secrets or tokens in logs, docs, fixtures, or error messages.
- [ ] CORS is not wildcard in release and dev origins are disabled.
- [ ] SQLite file/dir permissions and backend selection are verified.
- [ ] Loopback binding is enforced in code and tested.
- [ ] Body size, search length, and pagination bounds are enforced.
- [ ] Rate limiting works on login and logout.
- [ ] Dependencies have no known critical advisories.
- [ ] Config file permissions are owner-only.
- [ ] Devtools and external navigation are disabled in release.
- [ ] Backup and restore were tested.

## Severity

| Severity | Examples | Action |
|---|---|---|
| Critical | Auth bypass, secret in repo, remote code execution | Block release; fix immediately |
| High | XSS with token theft, SQL injection, missing auth on a module | Block release |
| Medium | Over-permissive CORS, verbose errors, weak rate limit | Fix before next release |
| Low | Missing security header, stale dependency | Schedule |

## Reporting format

Findings go to `docs/work/security/` and include: severity, location (file
and line), impact, reproduction, remediation, and verification. `security`
does not edit code; the owning developer implements the fix.
