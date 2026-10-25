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

4. Create the initial goose migration from that SQL. Keep constraints,
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
3. Add `internal/` packages and `migrations/`.
4. Add `docker-compose.yml` for PostgreSQL 17.
5. Implement config, pool, goose, health, and the loopback server.
6. Add the Wails `GetAPIBaseURL` binding and the frontend bootstrap.
7. Implement auth and the login screen.
8. Continue with the module order above.

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
| Sales | Initial create accepts `delivered_quantity > quantity`; item add/update rejects it | Preserve; document |
| Sales | `amount_paid > total_amount` is accepted and marks the sale paid | Preserve; document |
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
| Legacy service unavailable | Snapshot fixtures and replay locally |
