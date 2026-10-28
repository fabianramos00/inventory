# Migration Plan — FastAPI to Go

The legacy FastAPI service remains the behavioral reference. This plan ports
the API module by module and proves parity before cutover.

## Ground rules

- Do not change the database schema while porting a module.
- Do not change an endpoint’s success shape to make the Go code easier.
- Port business rules from `app/services/` to Go services, including
  transactions and stock locking.
- Every module gets handler tests, service tests, and contract fixtures.
- A module is “ported” only when its parity fixtures pass.
- Document every intentional difference in this file and in
  [DECISIONS.md](DECISIONS.md).

## Source of truth

| Concern | Source |
|---|---|
| Endpoint shape | FastAPI OpenAPI + live responses |
| Business rules | `../inventory-api/app/services/` |
| SQL behavior | `../inventory-api/app/repositories/` |
| Schema | `../inventory-api/alembic/` and a migrated database |
| UI behavior | `../inventory-app/src/` and its docs |
| New locale rules | [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md) |

## Schema import

1. Start PostgreSQL 17 with the legacy compose file or an equivalent.
2. Apply all Alembic migrations to an empty database.
3. Capture the schema without data:

   ```bash
   pg_dump --schema-only --no-owner --no-privileges inventory > schema.sql
   ```

4. Create the initial goose migrations from that SQL, split by domain and kept
   in dependency order: `00001_enums`, `00002_catalog`, `00003_identity`,
   `00004_products`, `00005_purchases`, `00006_sales`. Keep constraints,
   indexes, enum types, and defaults exactly as captured.
5. Verify by applying goose to a second empty database and diffing the two
   schemas:

   ```bash
   pg_dump --schema-only ... > restored.sql
   diff schema.sql restored.sql
   ```

6. Add an ADR if any schema change is required for Go compatibility. Prefer
   no change.
7. Future migrations are goose-only. Alembic is not part of the desktop app.

Seed data for development and parity tests is separate from migrations.

## Storage backends and SQLite

Persistence now follows the ports/adapters layout. PostgreSQL is the primary
backend and keeps the full legacy schema:

```text
migrations/postgres/00001_enums.sql
migrations/postgres/00002_catalog.sql
migrations/postgres/00003_identity.sql
migrations/postgres/00004_products.sql
migrations/postgres/00005_purchases.sql
migrations/postgres/00006_sales.sql
```

Goose applies versions 1–6 in order. Databases created before the split must
be recreated; the old single-file baseline is not compatible.

SQLite is the offline/standalone fallback:

```text
migrations/sqlite/00001_auth.sql
```

The SQLite migration currently creates only `users` and `refresh_tokens`.
The remaining 15 business tables are **Phase 3** work and must land as one
coherent SQLite schema with parity fixtures, not as ad-hoc per-module tables.

`DB_MODE=auto` (default) probes PostgreSQL with a 3-second timeout and falls
back to SQLite on failure. `DB_MODE=postgres` fails hard. `DB_MODE=sqlite`
opens the local database directly. `SQLITE_PATH` defaults to
`<OS config dir>/inventory-desktop/data/inventory.sqlite`.

Future sync is not implemented. A later sync design will require stable sync
IDs, reliable timestamps, tombstones, and an outbox for local changes; do not
build that protocol yet.

## Module order

| Order | Module | Endpoints | Why |
|---:|---|---:|---|
| 1 | Infrastructure | 1 | Server, config, pool, migrations, health |
| 2 | Auth + users | 8 | Everything else needs tokens and test users |
| 3 | Catalog | 18 | Products depend on categories, brands, materials, units |
| 4 | Products | 6 | Sales and orders depend on products |
| 5 | Clients + providers | 10 | Sales and orders depend on them |
| 6 | Sales + items | 15 | Core revenue flow and stock rules |
| 7 | Purchase orders + catalogs | 12 | Stock receiving and supplier data |
| 8 | Dashboard | 4 | Aggregations over the completed schema |

Total: 67 endpoints plus `healthz`.

## Per-module checklist

For each module:

- [ ] Read the FastAPI endpoint, schema, service, and repository.
- [ ] List every branch and error path.
- [ ] Write Go service tests for the rules.
- [ ] Write handler tests for status codes and JSON shape.
- [ ] Implement repository SQL with parameters and decimal scanning.
- [ ] Use a transaction for multi-row writes.
- [ ] Run the contract fixtures against FastAPI and Go.
- [ ] Compare list ordering and pagination.
- [ ] Update the parity matrix in [TESTING.md](TESTING.md).
- [ ] Update [API-CONTRACT.md](API-CONTRACT.md) if anything differs.
- [ ] Add integration coverage for concurrency or stock changes.

## Phase 2 scaffolding

1. Copy `../inventory-app` to `frontend/` with exclusions:
   `.git`, `node_modules`, `dist`, `test-results`, `playwright-report`,
   `.env`.
2. Add the Wails scaffold: `main.go`, `app.go`, `wails.json`, `build/`.
3. Add `internal/` packages and `migrations/{postgres,sqlite}/`.
4. Add `docker-compose.yml` for PostgreSQL 17.
5. Implement config, backend bootstrap, goose, health, and the loopback
   server.
6. Add the Wails `GetAPIBaseURL` binding and the frontend bootstrap.
7. Implement auth and the login screen.
8. Continue with the module order above.

## Phase 3 — SQLite business schema

1. Translate the remaining 15 business tables to `migrations/sqlite/`.
2. Preserve integer stock, decimal money semantics, and UTC timestamps.
3. Add adapter contract tests that run the same fixtures against PostgreSQL
   and SQLite.
4. Document any SQL-semantic differences (`ILIKE`, enums, `RETURNING`,
   aggregation) and gate features that cannot reach parity.
5. Do not implement sync in this phase.

## Frontend migration tasks

- Replace `VITE_API_URL` module-load usage with the binding bootstrap.
- Switch the desktop build to `HashRouter`.
- Replace `es-PE` formatters with `es-CO`.
- Change currency display to COP.
- Review browser-only APIs (`beforeunload`, `window.location`).
- Keep existing hooks, query keys, components, and Spanish copy.
- Fix the known type drift in `src/types/index.ts` where API responses and
  TypeScript types disagree; the API contract wins.
- Keep Playwright tests, updating URLs to hash routes.

## Known legacy quirks

| Area | Quirk | Port decision |
|---|---|---|
| Dashboard | `pending_payment_sales` is a money amount, not a count | Preserve; document |
| Dashboard | `inventory_value` uses sale price, not cost | Preserve; document |
| Dashboard | Top products sum delivered quantity but rank by subtotal | Preserve; document |
| Orders | Deleting a received order does not reverse stock | Preserve until approved |
| Products | `sku` is generated when omitted | Preserve |
| Products | `size_value` is a string | Preserve |
| Purchase order detail | Returns the order without items; items are a separate endpoint | Preserve |
| Sales | `amount_paid > total_amount` is accepted and marks the sale paid | Preserve; document |
| Enums | PostgreSQL stores uppercase enum labels; the API uses lowercase | Map explicitly in every Go read/filter/write |
| Defaults | Several legacy defaults are Python-side only (`stock_quantity`, `low_stock_threshold`, `amount_paid`, `delivered_quantity`, `total_amount`) | Supply them explicitly in Go inserts |
| Validation | FastAPI `422` detail is an array | Normalize to a string detail + code |
| Frontend types | `Product` type uses `stock`/`min_stock`/`unit`; API returns `stock_quantity`/`low_stock_threshold`/related objects | Fix frontend to match API |
| Locale | UI uses `es-PE` while backend uses Bogotá | New rule: `es-CO` / COP |

## Cutover criteria

- All 67 endpoints pass parity fixtures or have approved differences.
- Auth, stock, and order-receiving integration tests pass under `-race`.
- Frontend E2E passes against the Go API.
- A production-sized database copy migrates and serves the dashboard without
  errors.
- A full backup/restore rehearsal succeeds.
- Security checklist has no open Critical or High findings.
- The desktop app runs on each target OS, survives restart, and persists
  data.
- Release checklist in [RELEASE.md](RELEASE.md) is complete.

## Risks

| Risk | Mitigation |
|---|---|
| FastAPI behavior is implicit in SQLAlchemy | Read repositories, not only endpoints; compare live responses |
| Dashboard SQL semantics drift | Port queries directly; fixture the aggregates |
| Numeric precision changes | Decimal scanning tests and no float in Go |
| Stock race conditions | Row locks and concurrency tests |
| Wails webview differences | HashRouter, manual platform smoke tests |
| Shared Postgres with multiple counters | Shared `SECRET_KEY` and documented deployment |
| SQLite fallback diverges from PostgreSQL | Auth-only schema today; Phase 3 full-schema parity tests before exposing business modules |
| Legacy service unavailable | Snapshot fixtures and replay locally |
