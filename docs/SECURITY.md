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
- CORS allows only the Wails webview origin and the Vite dev origin. Do not
  use `*` with credentials.
- Close the HTTP server on app shutdown.
- `GET /healthz` is unauthenticated and returns no business data.

### Authentication

- Bearer JWT, HS256, 30-minute access token.
- Refresh token: 32 random bytes, stored as a SHA-256 hash, rotated on every
  use, revoked on logout.
- bcrypt cost 12 in production; 4 only in tests.
- Login rate limit: 5 attempts per minute per client.
- Reject inactive users on login and on every authenticated request.
- All `/users` routes require `is_superuser`.
- `SECRET_KEY` is generated per install or supplied explicitly. It is never
  hardcoded and never committed.
- Passwords are 8–128 characters. Never log or return them.

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
- Auto-update, if added, must verify signatures. It is out of scope for this
  phase.

## Audit checklist

- [ ] Auth flows: login, refresh rotation, logout, inactive user, superuser.
- [ ] All 67 endpoints require auth except the documented four.
- [ ] No endpoint trusts client totals or statuses.
- [ ] Stock changes are transactional and row-locked.
- [ ] No raw SQL, `eval`, or `dangerouslySetInnerHTML`.
- [ ] No secrets or tokens in logs, docs, fixtures, or error messages.
- [ ] CORS is not wildcard in release.
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
