# Phase 2 Database Schema Review

**Status:** Review only. No code, SQL, migration, or configuration changed.
**Reviewer scope:** `database` (Markdown only).
**Inputs:** `AGENTS.md`, `docs/DOMAIN.md`, `docs/API-CONTRACT.md`,
`docs/BACKEND.md`, `docs/MIGRATION.md`, `migrations/00001_initial_schema.sql`,
`../inventory-api/app/models/`, `../inventory-api/app/repositories/`,
`../inventory-api/alembic/versions/`.

## Verdict

`migrations/00001_initial_schema.sql` is a faithful translation of the
accumulated Alembic head. I found **no table, column, type, nullability,
server-default, enum, constraint, or index mismatch**. The goose file matches
the legacy schema as it exists in PostgreSQL, including legacy artifacts.

The Phase 2/3 risks are not schema drift. They are:

1. The database stores **uppercase enum names** while the API exposes
   **lowercase values**; every Go status write/read/filter needs an explicit
   mapping.
2. Several defaults are **Python-side only** and do not exist in PostgreSQL.
   Go INSERTs must supply them, or columns become `NULL` or fail `NOT NULL`.
3. The documented "over-delivery gap" is **contradicted by a real CHECK
   constraint** (`delivered_quantity <= quantity`).
4. `supplier_catalogs`, `sales.created_by_id`, and `refresh_tokens.user_id`
   have **no FK indexes**, which Phase 3 queries will hit.
5. The initial migration has **no adoption/baseline path** for an existing
   legacy database; it only works on an empty database.

No **Critical** findings. The items below are ordered by impact.

---

## 1. Schema parity with the legacy Alembic head

Parity was checked against the final state of the Alembic chain
(`65dec522a617` → `a57c178c6663` → `8c7cca1a10a8` → `64fc2b953820` →
`d6b78e1f6e38` → `cc84e1876572` → `ebe1f48383d2`), not against ORM
annotations alone.

| Object | Legacy source | Goose location | Verdict |
|---|---|---|---|
| 5 enum types | initial, `a57c…`, `8c7c…` | `00001` L14-62 | Match, labels and label order |
| `brands` | initial | L71-95 | Match |
| `categories` (+ `code`) | initial, `ebe1…` | L102-127 | Match |
| `clients` | `a57c…` | L134-162 | Match |
| `materials` | initial | L169-192 | Match |
| `measurement_units` | initial | L199-223 | Match |
| `products` | initial, `cc84…`, `d6b…` | L299-335, L312-314, L589, L795-799, L929-967 | Match |
| `price_history` (+ cascade) | initial, `d6b…`, `cc84…` | L266-292, L1135-1139 | Match |
| `providers` | initial | L342-369 | Match |
| `sales` (+ client FK, statuses) | initial, `a57c…`, `d6b…`, `cc84…` | L482-514, L1111-1115 | Match |
| `sale_items` (+ delivered qty) | initial, `a57c…`, `d6b…` | L445-475 | Match |
| `purchase_orders` (+ payment status) | initial, `8c7c…`, `d6b…`, `cc84…` | L376-405 | Match |
| `order_items` | initial, `d6b…`, `cc84…` | L230-259 | Match |
| `supplier_catalogs` | initial, `cc84…` | L521-548 | Match |
| `users` | initial | L555-582, L1090-1107 | Match |
| `refresh_tokens` | `64fc…` | L412-438, L1202-1203 | Match |
| 17 FK constraints | accumulated head | L1111-1243 | Match |
| 6 UNIQUE constraints | accumulated head | L691-767 | Match |
| 8 CHECK constraints | `d6b…` | L238, L312-314, L384, L453-454, L493 | Match |
| 36 indexes | accumulated head | L858-1107 | Match |

### Deliberate legacy artifacts (do not "fix" in the import)

- Every table has an `ix_<table>_id` btree index in addition to the primary
  key (`00001` L858-1107). These duplicate the PK index. They are in the
  legacy dump, so they are preserved. Candidate for removal later, after
  measurement.
- `products.price`/`cost` CHECKs are stored as
  `(price)::double precision > 0` (`00001` L312-313) because they were created
  while those columns were `DOUBLE PRECISION` and the columns were later
  altered to `NUMERIC(12,2)` (`cc84…`). This is a faithful dump artifact.
  `NUMERIC(12,2)` values fit exactly in double precision, so it is harmless.
- Several FK columns are nullable even where the domain says "required":
  `sale_items.sale_id`, `sale_items.product_id`,
  `supplier_catalogs.product_id`, `supplier_catalogs.provider_id`,
  `purchase_orders.provider_id`, `price_history.product_id`. This is the
  legacy schema; the services always set them.
- `users.is_active`, `users.is_superuser`, and `refresh_tokens.revoked` are
  nullable booleans with server defaults. This is legacy; see F13.

### Python-side defaults that are correctly absent from PostgreSQL

These are ORM/Pydantic defaults, not server defaults. Their absence is
correct for a schema dump, but Go must replace them:

| Column | Legacy source | Python default | In DB |
|---|---|---|---|
| `products.stock_quantity` | `models/inventory.py` L55 | `0` | nullable, no default |
| `products.low_stock_threshold` | `models/inventory.py` L56 | `10` | nullable, no default |
| `sales.amount_paid` | `models/sales.py` L47 | `0` | `NOT NULL`, no default |
| `sale_items.delivered_quantity` | `models/sales.py` L95 | `0` | `NOT NULL`, no default |
| `purchase_orders.total_amount` | `models/supply_chain.py` L59 | `0` | `NOT NULL`, no default |
| `clients.updated_at`, `sales.updated_at` | `models/sales.py` L32, L45 | `onupdate=func.now()` | nullable, no default/trigger |

See F2.

---

## 2. Enum representation and required Go mapping

### What the database stores

`00001` L14-62 defines five PostgreSQL enum types. SQLAlchemy stores the
**enum member name**, not the Python value, unless `values_callable` is used.
The legacy models define lowercase values (`models/sales.py` L10-25,
`models/supply_chain.py` L35-45), but the database and the goose migration
store uppercase labels. The API serializes the Pydantic enum **value**, so the
JSON is lowercase.

| API/JSON value | Database label | Column(s) |
|---|---|---|
| `cash` / `credit` / `debit` | `CASH` / `CREDIT` / `DEBIT` | `sales.payment_method` |
| `pending` / `partial` / `paid` | `PENDING` / `PARTIAL` / `PAID` | `sales.payment_status` |
| `pending` / `partial` / `delivered` | `PENDING` / `PARTIAL` / `DELIVERED` | `sales.delivery_status` |
| `pending` / `sent` / `received` / `cancelled` | `PENDING` / `SENT` / `RECEIVED` / `CANCELLED` | `purchase_orders.status` |
| `pending` / `paid` | `PENDING` / `PAID` | `purchase_orders.payment_status` |

Note there are **two different payment-status types**:
`sales.payment_status` has `PARTIAL`; `purchase_order_payment_status` does
not. Do not share a single Go enum across both without validating the allowed
set, or a `PARTIAL` purchase-order write will be rejected by PostgreSQL.

### Failure modes if the mapping is missing

- **Write/filter by parameter (usual pgx path): loud.**
  `WHERE payment_status = $1` with `$1 = "pending"` is inferred by PostgreSQL
  as the enum type and raises
  `22P02 invalid input value for enum payment_status: "pending"`.
- **Filter after casting the enum to text: silent zero rows.**
  `WHERE payment_status::text = $1` with `"pending"` never matches `PENDING`.
  This is the dangerous variant because it returns `200` with an empty list.
  The same happens with `lower(payment_status) = 'pending'`.
- **Filter dropped by the Go layer: silent wrong rows.**
  If the handler validates the lowercase value, fails to map it, and then
  omits the predicate, the endpoint returns unfiltered rows.
- **Read without lowering: contract mismatch.**
  Returning `"PENDING"` instead of `"pending"` fails
  `docs/API-CONTRACT.md` parity. It is loud in contract tests, but the UI
  may branch on the wrong value until noticed.
- **Dashboard status distribution: must lower-case.**
  `app/repositories/dashboard.py` L184 returns `row.status.value` (lowercase).
  The Go equivalent must map DB labels back to lowercase; `GROUP BY status`
  returns `PENDING`, `SENT`, etc.

### Required Go rule

Define one Go string type per enum with explicit `toDB`/`fromDB` maps, validate
input against the allowed set, and never interpolate enum values into SQL.
Scan enum columns as `string` (pgx returns the enum label as text; it has no
built-in enum OID codec). Example for `sales.payment_status`:

```go
type SalePaymentStatus string

const (
    SalePaymentPending SalePaymentStatus = "pending"
    SalePaymentPartial SalePaymentStatus = "partial"
    SalePaymentPaid    SalePaymentStatus = "paid"
)

func (s SalePaymentStatus) db() (string, bool) {
    switch s {
    case SalePaymentPending:
        return "PENDING", true
    case SalePaymentPartial:
        return "PARTIAL", true
    case SalePaymentPaid:
        return "PAID", true
    default:
        return "", false
    }
}
```

Apply the same pattern to `payment_method`, `delivery_status`, `order_status`,
and `purchase_order_payment_status`. Add a round-trip integration test:
POST a sale with `payment_status` inputs, read it back, assert the JSON is
lowercase, and assert `SELECT payment_status FROM sales` is uppercase.

**Severity: High. Must fix before the auth/sales/supply-chain modules.**

---

## 3. Auth tables (`users`, `refresh_tokens`)

### `users`

- `email` uniqueness: **present** as `ix_users_email` unique index
  (`00001` L1090-1093). This matches `models/users.py` L12 and
  `64fc2b953820`/initial. The API duplicate-email path must map PostgreSQL
  `23505` to `400` (`docs/API-CONTRACT.md` L79).
- The unique index is **case-sensitive**. Legacy `get_by_email` uses exact
  equality (`app/repositories/users.py` L19-22), and login uses the form
  `username` as the email. Do not switch to `citext` or `lower(email)` during
  the port; that changes behavior. See F12.
- `is_active` and `is_superuser` are nullable with server defaults
  (`00001` L560-561). Python treats `NULL` as false in
  `authenticate` (`app/services/users.py` L47). In Go, scanning `NULL` into
  `bool` fails. Use `COALESCE(is_active, false)` /
  `COALESCE(is_superuser, false)` in queries, or harden the columns later.
  See F13.
- `users.full_name` is nullable and indexed. `GET /users?search=` does
  `ILIKE '%term%'` on `email` and `full_name`
  (`app/repositories/users.py` L33-38). Btree indexes cannot serve a leading
  wildcard; see F15.

### `refresh_tokens`

- `token_hash` uniqueness: **present** as
  `ix_refresh_tokens_token_hash` unique index (`00001` L1020-1023), matching
  the model and `64fc…`.
- Hash format: legacy stores `sha256(raw_token).hexdigest()` (64 lowercase
  hex chars, `app/core/security.py` L32-33). The column is unbounded
  `character varying` (`00001` L415). If the desktop app shares a database or
  tokens with the legacy web app (`docs/MIGRATION.md` "shared Postgres with
  multiple counters"), Go **must** use the same hash format. If tokens are
  never shared, any format works, but matching is cheaper and safer.
- Expiry index: **absent**. The hot path is
  `WHERE token_hash = $1 AND revoked = false AND expires_at > now()`
  (`app/repositories/users.py` L99-103). The unique `token_hash` index
  narrows this to one row, so a separate `expires_at` index does not help the
  refresh flow. It would only help a periodic purge of expired rows, which
  does not exist in the legacy service. **Do not add speculatively.**
- Revoked index: **absent**. Same reason: `token_hash` is unique, so a
  `revoked` index adds write cost and is not used by `find_valid`. **Do not
  add speculatively.** A future "revoke all tokens for user" job would use
  `user_id`, not `revoked`.
- Cascade on user delete: **present** as
  `refresh_tokens_user_id_fkey ... ON DELETE CASCADE` (`00001` L1199-1203).
  This matches the model (`models/users.py` L22-24) and `64fc…`.
- Missing index: `refresh_tokens.user_id` has **no index** (`00001` L1013-1023
  only creates `id` and `token_hash`). PostgreSQL must scan `refresh_tokens`
  to apply the cascade when a user is deleted. Add it if user deletion is in
  scope. See F7.
- `revoked` is nullable (`00001` L417). Python checks
  `RefreshToken.revoked.is_(False)`, which does not match `NULL`. Rows are
  created with the default `false`, so this is mostly theoretical, but Go
  should use `COALESCE(revoked, false)` or the column should be hardened.
  See F13.
- Rotation: legacy deletes the old row and inserts a new one
  (`app/repositories/users.py` L110-115). In Go, delete/revoke and insert in
  one transaction; the unique `token_hash` index is the concurrency guard.
  There is no cleanup job, so expired/revoked rows accumulate. That is a
  storage issue, not a schema blocker.

**Auth verdict:** unique email and token-hash uniqueness are sufficient for
Phase 2. The only index that should be added before Phase 3 is
`refresh_tokens.user_id`. Expiry and revoked indexes are intentionally not
proposed.

---

## 4. Remaining high-risk issues for Phase 3

### 4.1 Missing FK indexes

Each proposed index names the query it serves. These are all absent in
`00001`, and they are absent in the legacy schema too, so they must be added
as a **new** goose migration, never by editing `00001`.

| Column | Query that needs it | Impact |
|---|---|---|
| `supplier_catalogs.product_id` + `provider_id` | `get_catalog_entry`: `WHERE product_id = $1 AND provider_id = $2` (`app/repositories/supply_chain.py` L153-161); catalog list filters by `provider_id` / `product_id` | Order receiving does one catalog lookup per item; no index means a full catalog scan per item |
| `sales.created_by_id` | `GET /sales?user_id=`: `WHERE created_by_id = $1 ORDER BY id DESC` (`app/repositories/sales.py` L81-82, L104); FK check on user delete | Sales list filter and user delete scan |
| `refresh_tokens.user_id` | `ON DELETE CASCADE` from `users`; future revoke-all | Full scan of `refresh_tokens` on user delete |
| `products.measurement_unit_id` | FK check on unit delete; product detail join uses PKs, not this index | Unit delete scan; no API filter |
| `price_history.product_id` | `ON DELETE CASCADE` from `products` (`00001` L1135-1139) | Product delete scan; table is currently unused |
| `providers.created_by_id` | FK check on user delete | User delete scan; no API filter |
| `purchase_orders.created_by_id` | FK check on user delete; display join uses PK | User delete scan; no API filter |

The high-value ones are the first three. The rest are Low and should be
measured first.

### 4.2 CHECK constraints

| Invariant | Status | Note |
|---|---|---|
| `products.stock_quantity >= 0` | Present (`00001` L314) | Column is nullable, so `NULL` bypasses the check. `stock_status` filters (`> 0`, `= 0`, `<= threshold`, `app/repositories/inventory.py` L82-90) silently exclude `NULL` rows. |
| `sale_items.delivered_quantity <= quantity` | Present (`00001` L453) | **This contradicts the documented "initial create does not reject over-delivery" gap.** See F3. |
| `sales.amount_paid <= total_amount` | **Absent, intentionally** | Legacy accepts overpayment and marks the sale paid (`docs/DOMAIN.md` L113-115, `docs/API-CONTRACT.md` L214-216). Do **not** add this check; it would break parity. |
| `sales.amount_paid >= 0` | Absent | Domain says non-negative (`docs/DOMAIN.md` L113). Pydantic `MoneyInput` has no `ge=0` (`app/schemas/types.py`), so a negative value can be stored. Add only after checking existing data. |
| `sale_items.quantity > 0`, `order_items.quantity > 0` | Present (`00001` L238, L454) | The API schemas do not validate `quantity > 0`; the DB rejects with `23514`. Go must validate and return a clean `400`, not a `500`. |
| `products.price > 0`, `products.cost > 0` | Present (`00001` L312-313) | Same: the API accepts `0`/negative; the DB rejects. Map `23514` to `400`. |
| Money fields on items/catalogs/history | Absent | `unit_price`, `unit_cost`, `subtotal`, `last_quoted_cost`, `old_price`, `new_price`, `low_stock_threshold` have no non-negative checks. Measure existing data before adding. |

### 4.3 The over-delivery contradiction (F3)

`docs/DOMAIN.md` L109-113 and `docs/API-CONTRACT.md` L211-214 say the initial
create path does not reject `delivered_quantity > quantity`, and ask the port
to preserve that gap. The database says otherwise: `ck_sale_items_delivered_valid`
(`00001` L453, added in `d6b…`) rejects it.

Concretely, `POST /sales` with `quantity = 1`, `delivered_quantity = 5`:
the service computes status (`app/services/sales.py` L145-151) and inserts
without validation; the `INSERT` fails with `23514`; `SaleService.create`
catches only `Exception`, rolls back, and re-raises (`app/services/sales.py`
L178-180). That is a `500` in FastAPI, not a clean business error.

This is a docs-vs-schema conflict and must be resolved explicitly. The Go API
should validate before the insert and return `400`/`422` with a readable
detail. If product insists on preserving the gap, the check constraint must be
dropped, which is a destructive schema change and is not recommended.

### 4.4 NUMERIC usage

All 12 monetary columns are `NUMERIC(12,2)` after `cc84…`:

`products.price`, `products.cost`, `sale_items.unit_price`,
`sale_items.subtotal`, `sales.total_amount`, `sales.amount_paid`,
`order_items.unit_cost`, `order_items.subtotal`,
`purchase_orders.total_amount`, `supplier_catalogs.last_quoted_cost`,
`price_history.old_price`, `price_history.new_price`.

No floats remain in the database. Go must scan through a decimal/string path,
not `float64` (`docs/BACKEND.md` L108-116). Note that the legacy API converts
to `float` for JSON; the database value is the source of truth. The maximum
`NUMERIC(12,2)` value is `9,999,999,999.99`; a stored `subtotal`/`total_amount`
above that would raise `22003`. This is not a realistic risk, but the Go
service should reject absurd quantities/prices at the boundary.

### 4.5 Timestamp types

All timestamps are `timestamptz`, which is correct for UTC storage:
`clients.created_at/updated_at`, `sales.created_at/updated_at`,
`purchase_orders.order_date`, `price_history.changed_at`,
`refresh_tokens.expires_at`. No `timestamp without time zone` exists.

`updated_at` on `clients` and `sales` has **no server default and no trigger**
(`00001` L137, L485). Legacy relies on SQLAlchemy `onupdate=func.now()`
(`models/sales.py` L32, L45). Go updates must set `updated_at = now()`
explicitly, or it silently stays stale. `created_at`/`order_date`/
`changed_at` do have `now()` server defaults.

### 4.6 Dashboard query indexes

Existing indexes already cover the range predicates:

| Dashboard query | Predicate | Existing index |
|---|---|---|
| summary, today sales | `sales.created_at >= today_start` | `ix_sales_created_at` |
| summary, pending payment | no predicate (full `sales` aggregate) | none, full scan |
| summary, delivered count | no predicate (same aggregate) | `ix_sales_delivery_status` unused for this shape |
| summary, pending orders | `purchase_orders.status = PENDING` | `ix_purchase_orders_status` |
| sales-trend | `sales.created_at >= …`, `purchase_orders.order_date >= …` | `ix_sales_created_at`, `ix_purchase_orders_order_date` |
| top-products | `sales.created_at >= …`, join `sale_items.sale_id`, join `products.id` | `ix_sales_created_at`, `ix_sale_items_sale_id`, PK |
| order-status-distribution | `GROUP BY purchase_orders.status` | `ix_purchase_orders_status` (small table; may seq scan) |

The `date(created_at AT TIME ZONE 'America/Bogota')` expression
(`app/repositories/dashboard.py` L88) is not indexable as written, but the
range predicate on `created_at`/`order_date` already limits the rows. Do not
add an expression index until `EXPLAIN (ANALYZE, BUFFERS)` shows the range
index is not being used or the sort is the bottleneck.

The summary query computes three aggregates over all `sales` with no `WHERE`
(`app/repositories/dashboard.py` L29-57). As the table grows, this becomes a
full scan. Partial/covering indexes are possible, but only after measurement.
Do not guess in Phase 2.

### 4.7 Search (`ILIKE '%term%'`)

`products.name`, `providers.name`, `users.email`, `users.full_name`,
`categories.name`, and the catalog/entity searches use leading-wildcard
`ILIKE` (e.g. `app/repositories/base.py` L68,
`app/repositories/users.py` L33-38). Btree indexes cannot serve `%term%`.
At hardware-store scale this is usually fine. If it becomes slow, add
`pg_trgm` + GIN indexes. That is a measured optimization, not a Phase 2
change.

---

## Findings

| ID | Severity | Location | Category | Issue | Fix | Verification |
|---|---|---|---|---|---|---|
| F1 | High | DB enums `00001` L14-62; API `docs/API-CONTRACT.md`; Go queries | Enum mapping | DB stores `PENDING`/`PAID`/…; API exposes `pending`/`paid`/… | Explicit `toDB`/`fromDB` per enum; validate input; never cast enum to text for filtering | Round-trip integration test; assert JSON lowercase and DB uppercase |
| F2 | High | `products.stock_quantity`, `low_stock_threshold`, `sales.amount_paid`, `sale_items.delivered_quantity`, `purchase_orders.total_amount`, `*_at` | Defaults | Python-side defaults are absent from PostgreSQL | Go INSERTs must set 0/10/0/0/0 and `updated_at = now()` explicitly | Create each row via API and via direct SQL; assert no `NULL` stock and no insert failure |
| F3 | High | `ck_sale_items_delivered_valid` `00001` L453; docs L109-113/L211-214 | Constraint vs docs | DB rejects over-delivery; docs say preserve the gap | Validate in Go, return `400`/`422`, update docs; do not drop the check | `INSERT` over-delivery and assert `23514`; API test expects `400` |
| F4 | High | `supplier_catalogs.product_id`, `provider_id` `00001` L521-528 | Index | No FK indexes; receive loop does one full scan per item | Add `product_id` and `provider_id` indexes; dedupe then consider a unique `(product_id, provider_id)` | `EXPLAIN` catalog lookup before/after; receiving test with many catalog rows |
| F5 | High | `migrations/00001_initial_schema.sql` | Migration | No baseline path for an existing legacy DB; `CREATE TYPE`/`CREATE TABLE` fail on a populated schema | Document empty-DB deployment; or backup + diff + stamp `goose_db_version` before `00002+` | Schema diff; restore rehearsal |
| F6 | Medium | `sales.created_by_id` `00001` L490 | Index | No index; `GET /sales?user_id=` and user delete scan | Add `ix_sales_created_by_id` | `EXPLAIN` user filter; user-delete test |
| F7 | Medium | `refresh_tokens.user_id` `00001` L414 | Index | Cascade scans the whole table | Add `ix_refresh_tokens_user_id` | `EXPLAIN DELETE`/cascade test |
| F8 | Medium | `sale_items`, `order_items`, `supplier_catalogs` | Uniqueness | No unique `(sale_id, product_id)`, `(order_id, product_id)`, `(product_id, provider_id)`; concurrent creates can duplicate | Check duplicates; add unique indexes if clean | Duplicate queries; concurrent create test |
| F9 | Medium | `sales.amount_paid`, item money columns | CHECK | No non-negative checks; negative `amount_paid` accepted | Check existing data; add `amount_paid >= 0` and item money checks | Negative insert/update test; data count query |
| F10 | Medium | `products.stock_quantity` `00001` L307, L314 | CHECK/nullability | `NULL` bypasses `stock_quantity >= 0`; stock filters exclude `NULL` | Go always sets a value; optionally backfill + `NOT NULL DEFAULT 0` later | `SELECT count(*) … IS NULL`; stock-status parity test |
| F11 | Medium | `sales.created_by_id` `00001` L490 vs `Sale` schema `created_by_id: int` | Nullability | DB allows `NULL`; API schema requires `int` and `User` | Check for `NULL` rows; backfill or make the response nullable (contract change) | `SELECT count(*) FROM sales WHERE created_by_id IS NULL`; GET test |
| F12 | Low | `ix_users_email` `00001` L1093 | Uniqueness | Case-sensitive; login is exact-match | Keep for parity; consider `citext`/`lower(email)` only as a future decision | Duplicate `Ana@` vs `ana@` test |
| F13 | Low | `users.is_active`, `is_superuser`, `refresh_tokens.revoked` | Nullability | `NULL` booleans: Python is falsy, Go scan fails | `COALESCE(..., false)`; optionally `SET NOT NULL` later | `NULL` row query; auth test |
| F14 | Low | 15 `ix_*_id` indexes `00001` L858-1107 | Index | Duplicate PK indexes; extra write cost | Measure; drop later if no query uses them | `pg_stat_user_indexes.idx_scan` |
| F15 | Low | `ILIKE '%term%'` searches | Index | Btree cannot serve leading wildcard | `pg_trgm` + GIN only if measured slow | `EXPLAIN` before/after |
| F16 | Low | Dashboard summary `dashboard.py` L29-57 | Index | Full-table aggregates | Measure; consider partial/covering indexes | `EXPLAIN (ANALYZE, BUFFERS)` on production-sized copy |
| F17 | Low | `sales.payment_method`, `purchase_orders.payment_status` | Index | Filter columns with 2-3 values | Low selectivity; measure before indexing | `EXPLAIN` list filters |
| F18 | Low | `price_history.product_id`, `products.measurement_unit_id`, `providers.created_by_id`, `purchase_orders.created_by_id` | Index | FK/cascade/display only | Add only after measurement | `EXPLAIN` delete/update paths |
| F19 | Low | `refresh_tokens.expires_at`, `revoked` | Index | No query needs a standalone index; `token_hash` is unique | Do not add unless a purge job exists | Refresh-flow `EXPLAIN` |
| F20 | Info | `00001` L312-313 | CHECK expression | Double-precision casts from the float→numeric migration | Do not "fix"; harmless for `NUMERIC(12,2)` | Constraint definition diff |
| F21 | Low | `sale_items.sale_id`, `order_items.order_id` FKs | Cascade | No `ON DELETE CASCADE`; legacy relies on ORM cascade | Go must delete items explicitly before deleting the parent | Delete-sale/order integration test |

### Must fix before Phase 3

- **F1** enum mapping (auth, sales, supply chain, dashboard).
- **F2** explicit defaults and `updated_at` in Go writes.
- **F3** resolve the over-delivery docs/schema conflict; validate in Go.
- **F4** `supplier_catalogs` indexes before the supply-chain module.
- **F5** decide and document the migration adoption path.
- **F6** `sales.created_by_id` index before the sales module.
- **F7** `refresh_tokens.user_id` index before user deletion is exposed.

### Measure first (do not change 00001)

- **F8** duplicate rows before adding unique indexes.
- **F9** existing negative money values before adding checks.
- **F10**, **F11**, **F13** existing `NULL` counts before `NOT NULL`/backfill.
- **F14** unused `ix_*_id` indexes.
- **F15** search latency before `pg_trgm`.
- **F16**, **F17** dashboard/list plans before any composite/partial index.
- **F18**, **F19** FK/cascade/expiry indexes only with a named query and a
  measured plan.
- **F20** no change at all.

---

## Schema changes

These are **proposals for a new Phase 3 migration**, not edits to
`00001_initial_schema.sql`. `00001` must stay immutable after release.

```sql
-- Proposed: migrations/00002_phase_3_indexes.sql
-- +goose Up

-- Serves: supplier catalog upsert lookup in order receiving
-- (WHERE product_id = $1 AND provider_id = $2) and the product_id list filter.
CREATE INDEX IF NOT EXISTS ix_supplier_catalogs_product_id
    ON public.supplier_catalogs (product_id);
-- Serves: the provider_id list filter. The lookup above can use the
-- product_id index and filter provider_id.
CREATE INDEX IF NOT EXISTS ix_supplier_catalogs_provider_id
    ON public.supplier_catalogs (provider_id);

-- Serves: GET /sales?user_id= (WHERE created_by_id = $1 ORDER BY id DESC)
-- and user-delete FK checks.
CREATE INDEX IF NOT EXISTS ix_sales_created_by_id
    ON public.sales (created_by_id);

-- Serves: ON DELETE CASCADE from users and future revoke-all operations.
CREATE INDEX IF NOT EXISTS ix_refresh_tokens_user_id
    ON public.refresh_tokens (user_id);

-- Measure first; add only if the plan shows a scan.
-- CREATE INDEX IF NOT EXISTS ix_products_measurement_unit_id
--     ON public.products (measurement_unit_id);
-- CREATE INDEX IF NOT EXISTS ix_price_history_product_id
--     ON public.price_history (product_id);
-- CREATE INDEX IF NOT EXISTS ix_providers_created_by_id
--     ON public.providers (created_by_id);
-- CREATE INDEX IF NOT EXISTS ix_purchase_orders_created_by_id
--     ON public.purchase_orders (created_by_id);

-- +goose Down
DROP INDEX IF EXISTS ix_refresh_tokens_user_id;
DROP INDEX IF EXISTS ix_sales_created_by_id;
DROP INDEX IF EXISTS ix_supplier_catalogs_provider_id;
DROP INDEX IF EXISTS ix_supplier_catalogs_product_id;
```

If any target table is large or the app is live, change these to
`CREATE INDEX CONCURRENTLY` and add `-- +goose NO TRANSACTION` after
`-- +goose Up`. `CONCURRENTLY` cannot run inside a transaction.

```sql
-- Proposed: integrity hardening after data validation.
-- Do NOT add amount_paid <= total_amount: legacy accepts overpayment.

ALTER TABLE public.sales
    ADD CONSTRAINT ck_sales_amount_paid_nonnegative
    CHECK (amount_paid >= 0) NOT VALID;
ALTER TABLE public.sales VALIDATE CONSTRAINT ck_sales_amount_paid_nonnegative;

-- Optional, after checking existing values:
-- ALTER TABLE public.sale_items
--     ADD CONSTRAINT ck_sale_items_unit_price_nonnegative
--     CHECK (unit_price >= 0) NOT VALID,
--     ADD CONSTRAINT ck_sale_items_subtotal_nonnegative
--     CHECK (subtotal >= 0) NOT VALID;
-- ALTER TABLE public.order_items
--     ADD CONSTRAINT ck_order_items_unit_cost_nonnegative
--     CHECK (unit_cost >= 0) NOT VALID,
--     ADD CONSTRAINT ck_order_items_subtotal_nonnegative
--     CHECK (subtotal >= 0) NOT VALID;
-- ALTER TABLE public.supplier_catalogs
--     ADD CONSTRAINT ck_supplier_catalogs_cost_nonnegative
--     CHECK (last_quoted_cost >= 0) NOT VALID;

-- Optional, after duplicate checks and NULL checks. A unique compound covers
-- the product_id-only predicate, so drop ix_supplier_catalogs_product_id in
-- the same migration if you add it.
-- CREATE UNIQUE INDEX IF NOT EXISTS ux_sale_items_sale_product
--     ON public.sale_items (sale_id, product_id);
-- CREATE UNIQUE INDEX IF NOT EXISTS ux_order_items_order_product
--     ON public.order_items (order_id, product_id);
-- CREATE UNIQUE INDEX IF NOT EXISTS ux_supplier_catalogs_product_provider
--     ON public.supplier_catalogs (product_id, provider_id);

-- Optional, after NULL backfill. This replaces the Python-side defaults.
-- ALTER TABLE public.products ALTER COLUMN stock_quantity SET DEFAULT 0;
-- ALTER TABLE public.products ALTER COLUMN low_stock_threshold SET DEFAULT 10;
-- UPDATE public.products SET stock_quantity = 0 WHERE stock_quantity IS NULL;
-- UPDATE public.products SET low_stock_threshold = 10 WHERE low_stock_threshold IS NULL;
-- ALTER TABLE public.products ALTER COLUMN stock_quantity SET NOT NULL;
-- ALTER TABLE public.products ALTER COLUMN low_stock_threshold SET NOT NULL;
```

`NULL` values are distinct in a unique index, so the unique indexes above do
not prevent duplicates whose key columns are `NULL`. Check for `NULL` and
duplicates before relying on them.

---

## Safe migration plan

### A. Fresh database (the documented Phase 2 path)

1. Create database A, apply all legacy Alembic migrations to head.
   `pg_dump --schema-only --no-owner --no-privileges` it.
2. Create database B, run `goose up` with `00001` only.
   `pg_dump --schema-only --no-owner --no-privileges` it.
3. Diff A and B. Exclude `alembic_version` (A only) and `goose_db_version`
   (B only). Expected result: **no differences** in tables, columns, types,
   defaults, constraints, indexes, or enums.
4. Keep `00001` immutable. Add all future changes as `00002+`.

### B. Existing legacy database (only if adoption is required)

1. Take a verified backup first:
   `pg_dump -Fc "$DATABASE_URL" > inventory-before-goose.dump`.
2. Prove the live schema matches `00001` with a schema-only diff, excluding
   `alembic_version`.
3. Stamp goose's version table **only after the diff is clean**. Use the
   `goose_db_version` table goose itself creates on a scratch database as the
   reference; do not guess its DDL. A manual stamp is a one-off operation and
   must be reviewed by `backend`/`devops`.
4. Run `00002+` migrations. Do not run `00001` against the populated DB; its
   `CREATE TYPE`/`CREATE TABLE` statements are not `IF NOT EXISTS` and will
   fail.
5. Restore rehearsal: restore the dump into a scratch database and confirm
   the app starts and the dashboard queries run.

### C. Applying `00002+`

1. Backup before any destructive or `NOT NULL` change.
2. Validation queries first (see below). If they return rows, stop and agree
   on a backfill or accept the legacy state.
3. Add CHECK constraints with `NOT VALID`, then `VALIDATE CONSTRAINT`.
4. Add indexes with `CREATE INDEX` (small/local) or
   `CREATE INDEX CONCURRENTLY` in a `NO TRANSACTION` migration (large/live).
5. Add unique indexes only after deduplication.
6. Rollback: forward-only. Index-only migrations can be reversed with
   `DROP INDEX`; constraint changes with `ALTER TABLE ... DROP CONSTRAINT`.
   `NOT NULL` and type changes require a restore from the backup.

### D. What not to do

- Do not edit `00001` to "clean up" `ix_*_id`, the double-precision CHECK
  casts, nullable booleans, or nullable FKs. It is the faithful import.
- Do not add `amount_paid <= total_amount`; it breaks the documented legacy
  behavior.
- Do not add `expires_at`/`revoked` indexes without a purge/revoke-all query.
- Do not add composite/partial/trigram indexes without an `EXPLAIN` from a
  production-sized copy.

---

## Verification and requested measurements

I cannot run shell commands. Please run these and paste the output where
indicated; I will refine the plan from the results.

### Schema diff

```bash
pg_dump --schema-only --no-owner --no-privileges \
  --exclude-table=alembic_version --exclude-table=goose_db_version \
  inventory > schema.sql

# after goose on the second DB:
pg_dump --schema-only --no-owner --no-privileges \
  --exclude-table=alembic_version --exclude-table=goose_db_version \
  inventory_goose > restored.sql

diff schema.sql restored.sql
```

Expected: empty diff.

### Data checks before hardening

```sql
SELECT count(*) AS null_stock FROM products WHERE stock_quantity IS NULL;
SELECT count(*) AS null_threshold FROM products WHERE low_stock_threshold IS NULL;
SELECT count(*) AS null_created_by FROM sales WHERE created_by_id IS NULL;
SELECT count(*) AS negative_paid FROM sales WHERE amount_paid < 0;
SELECT count(*) AS null_active FROM users WHERE is_active IS NULL;
SELECT count(*) AS null_superuser FROM users WHERE is_superuser IS NULL;
SELECT count(*) AS null_revoked FROM refresh_tokens WHERE revoked IS NULL;

SELECT sale_id, product_id, count(*)
FROM sale_items GROUP BY 1,2 HAVING count(*) > 1;
SELECT order_id, product_id, count(*)
FROM order_items GROUP BY 1,2 HAVING count(*) > 1;
SELECT product_id, provider_id, count(*)
FROM supplier_catalogs GROUP BY 1,2 HAVING count(*) > 1;
```

### `EXPLAIN` before adding indexes

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, last_quoted_cost, lead_time
FROM supplier_catalogs
WHERE product_id = 1 AND provider_id = 1;

EXPLAIN (ANALYZE, BUFFERS)
SELECT id FROM sales
WHERE created_by_id = 1
ORDER BY id DESC
LIMIT 101;

EXPLAIN (ANALYZE, BUFFERS)
SELECT count(*) FROM refresh_tokens WHERE user_id = 1;
```

### Constraint tests

Run each statement in its own transaction so an expected error does not abort
the rest:

```sql
BEGIN;
UPDATE products SET stock_quantity = -1 WHERE id = 1;          -- expect 23514
ROLLBACK;

BEGIN;
INSERT INTO sale_items (sale_id, product_id, quantity, delivered_quantity,
                        unit_price, subtotal)
VALUES (1, 1, 1, 2, 1.00, 1.00);                               -- expect 23514
ROLLBACK;

BEGIN;
UPDATE sales SET amount_paid = -1 WHERE id = 1;                -- currently succeeds
ROLLBACK;
```

The last statement succeeding is the F9 gap, not a bug in the test.

### Enum tests

- Through the API: create/update each status with lowercase JSON and read it
  back; assert lowercase JSON and uppercase DB labels.
- Directly in `psql`: `SELECT 'pending'::payment_status;` must raise
  `22P02`; `SELECT count(*) FROM sales WHERE payment_status::text = 'pending';`
  must return `0` (demonstrates the silent-filter failure mode).

### Auth tests

- Delete a user with refresh tokens: tokens are gone (cascade).
- Delete a user referenced by `sales.created_by_id`: FK violation, mapped to
  `400`, not `500`.
- Refresh rotation: old token no longer valid; new token row exists; one
  transaction.

### Query/behavior tests

- `limit + 1` pagination on every list endpoint.
- Stable ordering (`ORDER BY id DESC` for lists; dashboard ordering matches
  legacy, including ties).
- Concurrent sale of the last unit: one succeeds, one gets `400`.

---

## Open questions

1. **Deployment target:** is the desktop app always installed against a fresh
   empty database, or must it adopt an existing legacy database with data?
   This decides whether the baseline/stamp path in section B is required.
2. **Over-delivery:** `docs/DOMAIN.md`/`docs/API-CONTRACT.md` say to preserve
   the gap, but `ck_sale_items_delivered_valid` rejects it. Should Go return
   `400`/`422` and the docs be corrected, or should the constraint be dropped?
   Dropping it is destructive and not recommended.
3. **Overpayment:** confirm `amount_paid > total_amount` remains allowed
   (legacy behavior). If yes, never add `amount_paid <= total_amount`.
4. **Negative money:** are there existing rows with `amount_paid < 0`,
   negative item prices/costs, or negative `last_quoted_cost`? If none, add
   the non-negative checks.
5. **NULL/backfill:** can we make `products.stock_quantity`,
   `products.low_stock_threshold`, `users.is_active`, `users.is_superuser`,
   and `refresh_tokens.revoked` `NOT NULL`? This needs counts and product
   sign-off.
6. **`created_by_id`:** are there `sales.created_by_id IS NULL` rows? If yes,
   the current `Sale` response schema (`created_by_id: int`,
   `created_by: User`) will fail parity on those rows.
7. **Shared database:** will the Go app and the legacy FastAPI app share a
   database and refresh tokens? If yes, the Go token hash must be SHA-256
   lowercase hex, and `SECRET_KEY` must be shared.
8. **Token cleanup:** is a purge job planned for expired/revoked refresh
   tokens? If yes, add `expires_at` (and possibly `revoked`) only when that
   query exists.
9. **Measurements:** please provide the schema diff, the data-count queries,
   and the `EXPLAIN (ANALYZE, BUFFERS)` output above from a production-sized
   copy before I recommend any composite, partial, or trigram index.
