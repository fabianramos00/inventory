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

SQLite is the offline/standalone fallback and uses its own version map:

| SQLite version | File | Contents |
|---:|---|---|
| 1 | `migrations/sqlite/00001_auth.sql` | `users`, `refresh_tokens` |
| 2 | `migrations/sqlite/00002_catalog.sql` | `categories`, `materials`, `brands`, `measurement_units` + indexes |
| 3 | `migrations/sqlite/00003_identity.sql` | `clients`, `providers` |
| 4 | `migrations/sqlite/00004_products.sql` | `products`, `price_history` + indexes |
| 5 | `migrations/sqlite/00005_purchases.sql` | `supplier_catalogs`, `purchase_orders`, `order_items` + indexes |
| 6 | `migrations/sqlite/00006_sales.sql` | `sales`, `sale_items` + indexes |

A SQLite database that already applied the old single
`00002_business_schema.sql` is incompatible with the split baseline and must
be recreated: delete the local file at `SQLITE_PATH` (default
`<OS config dir>/inventory-desktop/data/inventory.sqlite`) and let the app
re-run migrations on the next start. Migrations are forward-only; there is no
in-place upgrade from the monolith.

The full business schema now exists in SQLite. Catalog, product, client,
provider, supplier catalog, and sales repositories/handlers are implemented on
both backends. Purchase orders and dashboard aggregates are still pending; the
schema being present does not mean those offline business modules work yet. The
two goose version tables are independent, so SQLite and PostgreSQL version
numbers do not match.

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

1. The 13 business tables now exist in SQLite, split by domain across
   `00002_catalog.sql` through `00006_sales.sql`.
2. Preserve integer stock, integer-cent money, and UTC text timestamps in the
   remaining business adapters. SQLite money is integer cents; see [ADR-0016](DECISIONS.md#adr-0016--sqlite-money-is-stored-as-integer-cents).
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
| Orders | Creating an order directly with `status: received` does not apply stock or catalogs; only the update transition does | Preserve; document the gap |
| Orders | Item add/update/delete on a received order does not adjust stock or catalogs | Preserve; document the gap |
| Orders | Status and payment enums are lowercase in the API and uppercase in PostgreSQL/SQLite | Map explicitly in every read/filter/write |
| Orders | Legacy order create validates the provider with `400` but relies on the product FK; Go validates products and returns `404` | Preserve the documented `404 Product with id X not found` |
| Search wildcards | Legacy `ILIKE` treats `%` and `_` as wildcards | Escape/treat literally on both backends; see ADR-0015 |
| Products | `sku` is generated when omitted | Preserve; retry generated collisions |
| Products | `size_value` is a string | Preserve |
| Products | `price_history` is declared but never written by the legacy API | Preserve; do not record price changes |
| Products | Update omits unchanged fields; explicit null clears only `size_value` and `measurement_unit_id` | Go is stricter: explicit null clears only `size_value`/`measurement_unit_id`; null for any other product field is `422`. |
| Unknown JSON fields | Pydantic ignores extra fields | Go rejects them with `422 VALIDATION_ERROR`; see ADR-0017 |
| Non-positive path IDs | FastAPI treats id 0/negative as not found (`404`) | Go returns `422 VALIDATION_ERROR` for id <= 0; documented difference |
| Product whitespace | Legacy preserves leading/trailing whitespace in name/SKU | Go trims name and supplied SKU before validation |
| Products | Legacy `ILIKE` searches name and SKU with wildcards | Treat `%`/`_` literally on both backends; see ADR-0015 |
| Clients | Legacy does not trim `name`, `identity_card`, `email`, or `phone` | Go trims all four; empty optional values are stored as `null` |
| Clients | Legacy `identity_card` is a unique optional column | Duplicate values return `400 UNIQUE_VIOLATION`; multiple `null` values are allowed |
| Clients | Legacy `updated_at` is set by SQLAlchemy `onupdate` whenever an UPDATE runs | Go sets it only when at least one field actually changes; no-op and whitespace-only updates leave it untouched |
| Clients | Legacy `PUT` uses `exclude_unset`, so explicit `null` clears and omitted fields stay | Preserve; `null`/empty/blank `name` is `422` instead of violating the NOT NULL column |
| Clients | Legacy search uses `ILIKE` over name, identity card, email, and phone | Case-insensitive literal substring on both backends; `%`/`_` are ordinary characters |
| Clients | Legacy does not validate email or phone format | Preserve; no format validation |
| Providers | Legacy search uses `ILIKE` over name, email, and phone; `contact_info` is not searched | Case-insensitive literal substring on name/email/phone; `%`/`_` are ordinary characters |
| Providers | Legacy has no timestamps and allows duplicate names | Preserve; response is `{id,name,contact_info,email,phone}` and no unique name constraint |
| Providers | `created_by_id` is set from the authenticated user but not returned | Set on create from the token; omit from every response |
| Supplier catalogs | Legacy is read-only through the API; rows are written when an order is received | Keep list/get read-only in this phase; no create/update/delete routes |
| Supplier catalogs | Legacy `last_quoted_cost` is `NUMERIC(12,2)` | PostgreSQL reads text decimals; SQLite stores integer cents; JSON stays an exact number |
| Client timestamps | PostgreSQL `timestamptz` and SQLite text have different precision | Round-trip through `gormstore.Timestamp`; PostgreSQL keeps microseconds, SQLite keeps nanoseconds |
| Money (SQLite) | SQLite has no exact decimal type | Store integer cents, convert at the adapter boundary; see ADR-0016 |
| Purchase order detail | Returns the order without items; items are a separate endpoint | Preserve |
| Sales | `amount_paid > total_amount` is accepted and marks the sale paid | Preserve; document |
| Sales | `delivered_quantity` is validated by the legacy add/update paths and by a PostgreSQL CHECK | Validate in the service; Go returns `422`, legacy `400` |
| Sales | Legacy accepts duplicate product rows in one sale | Go rejects duplicates with `422`; docs require one item per product |
| Sales | Total `0` with `amount_paid = 0` is `paid` because `0 >= 0` | Go returns `pending` per DOMAIN |
| Sale timestamps | Legacy may serialize microseconds | Go serializes second-precision RFC 3339; `end_date` filtering is inclusive and compared against the response timestamp |
| Empty `PUT /sales/{id}` | No update field set | Both leave `updated_at` unchanged in v1; no difference |
| Sales | Create/update/item paths return the sale with `created_by` and `client` loaded | Preserve; batch-load users and clients in the adapter |
| Sales | Item list order is `id ASC`; sale lists are `id DESC` | Preserve both orderings |
| Sales | Deleting a sale or an item restores stock; changing quantity adjusts by the delta | Preserve inside the unit-of-work transaction |
| Sales | Empty sale after removing every item keeps total `0` and both statuses `pending` | Preserve; special-case empty recalculation |
| Sales | Legacy never writes `price_history` during sales or product price changes | Preserve; do not add price-history writes |
| Enums | PostgreSQL stores uppercase enum labels; the API uses lowercase | Map explicitly in every Go read/filter/write |
| Defaults | Several legacy defaults are Python-side only (`stock_quantity`, `low_stock_threshold`, `amount_paid`, `delivered_quantity`, `total_amount`) | Supply them explicitly in Go inserts |
| Validation | FastAPI `422` detail is an array | Normalize to a string detail + code |
| Frontend product fields | `Product` type reads `stock_quantity` and `low_stock_threshold` and treats related category/brand/material/unit objects as nullable; legacy `stock`/`min_stock`/`unit` remain optional aliases | API fields are authoritative; frontend matches the API |
| Locale | UI uses `es-PE` while backend uses Bogotá | New rule: `es-CO` / COP |

### Dashboard

- **Bogota grouping.** Legacy uses `ZoneInfo("America/Bogota")`; Go uses the
  fixed `-05:00` offset because Colombia has no DST. Results are identical and
  the runtime does not need tzdata. PostgreSQL buckets with
  `AT TIME ZONE 'America/Bogota'`; SQLite derives the day from the stored UTC
  text with `date(created_at, '-5 hours')`. The host timezone is never used.
- **Money amount.** `pending_payment_sales` stays an amount
  (`total_amount - amount_paid` for `payment_status != paid`), never a count.
  Paid sales are excluded even when overpaid. The value can be negative when a
  non-paid sale is overpaid, matching the legacy sum.
- **Sale-price inventory.** `inventory_value` is
  `SUM(products.price * products.stock_quantity)` using the sale price, not the
  cost.
- **Top-product quirk.** Quantity is `SUM(sale_items.delivered_quantity)`, but
  ranking uses `SUM(sale_items.subtotal)`, so a product with undelivered items
  can outrank one with more delivered units. Preserve this legacy behavior.
- **Exact money.** PostgreSQL sums `NUMERIC(12,2)` as text decimals; SQLite
  sums integer cents and converts once at the adapter boundary. No `float64`
  money path.
- **Trend shape.** The repository returns only days with rows; the application
  builds the seven Bogota dates, oldest first, and zero-fills the gaps.
- **Distribution percentage.** Go rounds to two decimals for a stable JSON
  number; the legacy float is otherwise the same ratio. Statuses are lowercase
  and ordered by count descending.
- **Limit validation.** `limit` defaults to `10` and is constrained to
  `1..100`, returning `422` outside that range. FastAPI accepted any integer;
  the frozen contract requires the bound.

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
| SQLite fallback diverges from PostgreSQL | Business schema now exists; full-schema parity tests still required before exposing business modules |
| Legacy service unavailable | Snapshot fixtures and replay locally |
