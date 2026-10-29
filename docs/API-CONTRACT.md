# API Contract

The Go API must behave like the legacy FastAPI API for every existing
endpoint. This document defines the contract, the deliberate differences, and
the parity checks.

Base URL: `http://127.0.0.1:<port>/api/v1`

## Authentication

| Item | Value |
|---|---|
| Scheme | Bearer JWT |
| Access token lifetime | 30 minutes |
| Refresh token lifetime | 7 days |
| Password hashing | bcrypt |
| Login content type | `application/x-www-form-urlencoded` |
| Refresh/logout content type | `application/json` |

Public endpoints: `POST /auth/login`, `POST /auth/refresh`,
`POST /auth/logout`, and `GET /healthz` (desktop readiness, outside the
contract). Every other endpoint requires a valid access token and an active
user.

### `POST /auth/login`

Form fields: `username` (the email) and `password`.

```json
{
  "access_token": "jwt",
  "refresh_token": "opaque-token",
  "token_type": "bearer",
  "user": {
    "id": 1,
    "full_name": "Ana Pérez",
    "email": "ana@example.com",
    "is_active": true,
    "is_superuser": true
  }
}
```

- Invalid credentials: `401` with `{"detail": "Incorrect email or password"}`.
- Inactive user: `401` with `{"detail": "Inactive user"}`.
- Rate limit: 5 attempts per minute per client.

### `POST /auth/refresh`

```json
{ "refresh_token": "opaque-token" }
```

Returns a new access token and a rotated refresh token:

```json
{ "access_token": "jwt", "refresh_token": "new-opaque-token", "token_type": "bearer" }
```

- The old refresh token is revoked on success.
- Invalid or expired token: `401`.

### `POST /auth/logout`

Revokes the refresh token and returns `{"detail": "Logged out"}`.

### Users

All user endpoints require a superuser.

| Method | Path | Notes |
|---|---|---|
| GET | `/users` | `skip`, `limit`, `search`; paginated |
| POST | `/users` | `email`, `password` (8–128 characters, max 72 bytes), `full_name`, `is_active`, `is_superuser` |
| GET | `/users/{id}` | |
| PUT | `/users/{id}` | Partial update; optional password |
| DELETE | `/users/{id}` | |

Duplicate email: `400` with a readable `detail`.

## Conventions

### Pagination

Every list endpoint returns:

```json
{
  "items": [],
  "has_next": false,
  "skip": 0,
  "limit": 100
}
```

- `skip >= 0`.
- `limit` defaults to 100 and is capped at 200.
- The repository fetches `limit + 1` rows to compute `has_next`.
- Filter objects use exact field names from the endpoint tables below.

### Errors

All errors use:

```json
{ "detail": "Human readable message", "code": "MACHINE_CODE" }
```

`detail` is always a string. `code` is optional; when present it is stable
and suitable for frontend branching. Validation errors also return
`{"detail": "...", "code": "VALIDATION_ERROR"}` plus an optional
`errors` array.

This is a deliberate difference from FastAPI's `422` array-shaped `detail`.
The legacy frontend only consumes a string `detail`, so the normalized shape
is both safer and compatible with the UI.

| Status | Meaning |
|---|---|
| 200 | Success |
| 201 | Created |
| 204 | No content (not currently used; legacy returns the deleted object) |
| 400 | Business rule violation |
| 401 | Missing/invalid token or credentials |
| 403 | Authenticated but not a superuser |
| 404 | Record not found |
| 409 | Reserved; the legacy API uses `400` for integrity conflicts |
| 422 | Input validation |
| 500 | Unexpected server error; no internals leaked |

### Money

- Database: `NUMERIC(12,2)`.
- Go: `shopspring/decimal`.
- JSON: number with up to two decimals.
- Values with more than two decimals round half away from zero to two decimals; PostgreSQL `NUMERIC(12,2)` stores the rounded value.

### Dates and times

- JSON timestamps: RFC 3339, UTC (`2026-10-24T15:04:05Z`).
- Date-only trend keys: `YYYY-MM-DD`.
- Dashboard boundaries: `America/Bogota`.
- UI display: `DD/MM/YYYY`.

### Search

`search` is optional, trimmed, and limited to 256 characters. It matches the
entity's name fields as defined by the legacy repository. Do not change the
match columns while porting. Inventory search (catalog names and product
name+SKU) treats `%` and `_` literally as a deliberate difference from legacy
`ILIKE` wildcards; see [MIGRATION.md](MIGRATION.md#known-legacy-quirks) and
[DECISIONS.md](DECISIONS.md#adr-0015--inventory-search-treats-sql-wildcards-literally).

## Endpoints

### Inventory

| Method | Path | Parameters / body |
|---|---|---|
| GET | `/inventory/categories` | `skip`, `limit`, `search` |
| POST | `/inventory/categories` | `name`, `code?`, `description?` |
| GET | `/inventory/categories/{id}` | |
| PUT | `/inventory/categories/{id}` | Partial |
| DELETE | `/inventory/categories/{id}` | |
| GET | `/inventory/materials` | `skip`, `limit`, `search` |
| POST | `/inventory/materials` | `name` |
| PUT | `/inventory/materials/{id}` | Partial |
| DELETE | `/inventory/materials/{id}` | |
| GET | `/inventory/brands` | `skip`, `limit`, `search` |
| POST | `/inventory/brands` | `name`, `logo_url?` |
| GET | `/inventory/brands/{id}` | |
| PUT | `/inventory/brands/{id}` | Partial |
| DELETE | `/inventory/brands/{id}` | |
| GET | `/inventory/measurement-units` | `skip`, `limit`, `search` |
| POST | `/inventory/measurement-units` | `name`, `abbreviation` |
| PUT | `/inventory/measurement-units/{id}` | Partial |
| DELETE | `/inventory/measurement-units/{id}` | |
| GET | `/inventory/products` | `skip`, `limit`, `search`, `category_id`, `brand_id`, `material_id`, `stock_status` |
| POST | `/inventory/products` | `name`, `sku?`, `size_value?`, `measurement_unit_id?`, `price`, `cost`, `stock_quantity`, `low_stock_threshold`, `brand_id`, `category_id`, `material_id` |
| GET | `/inventory/products/{id}` | Includes category, material, brand, unit |
| PUT | `/inventory/products/{id}` | Partial |
| DELETE | `/inventory/products/{id}` | |
| GET | `/inventory/products/stats/overview` | `total_products`, `total_stock`, `low_stock_count`, `total_inventory_value` |

`stock_status`: `in_stock`, `low_stock`, `out_of_stock`.

Product list items include the related category, material, brand, and
measurement unit. Missing `sku` is generated as `SKU-XXXXXXXX`. Create, update,
and delete return `200` with the plain product, without relations. Product
updates are partial: omitted fields stay unchanged; `size_value` and
`measurement_unit_id` accept `null` to clear them; `null` for any other field
is `422`. Product `price` and `cost` round half away from zero to two decimals.

### Sales and clients

| Method | Path | Parameters / body |
|---|---|---|
| GET | `/sales/clients` | `skip`, `limit`, `search` |
| POST | `/sales/clients` | `name`, `identity_card?`, `email?`, `phone?` |
| GET | `/sales/clients/{id}` | |
| PUT | `/sales/clients/{id}` | Partial |
| DELETE | `/sales/clients/{id}` | |
| GET | `/sales` | `skip`, `limit`, `user_id`, `client_id`, `no_client`, `start_date`, `end_date`, `payment_status`, `delivery_status`, `payment_method` |
| POST | `/sales` | `payment_method?`, `amount_paid`, `client_id?`, `items[]` |
| GET | `/sales/stats` | `unpaid_count`, `undelivered_count`, `total_amount_sum`, `amount_paid_sum` |
| GET | `/sales/{id}` | |
| PUT | `/sales/{id}` | `payment_method?`, `amount_paid?`, `client_id?` |
| DELETE | `/sales/{id}` | Restores stock |
| GET | `/sales/{id}/items` | `skip`, `limit` |
| POST | `/sales/{id}/items` | `product_id`, `quantity`, `delivered_quantity?`, `unit_price?` |
| PUT | `/sales/{id}/items/{item_id}` | Partial |
| DELETE | `/sales/{id}/items/{item_id}` | Restores stock |

#### Clients

Item shape:

```json
{
  "id": 1,
  "name": "Ana Pérez",
  "identity_card": "1020304050",
  "email": "ana@example.com",
  "phone": "3001234567",
  "created_at": "2026-10-24T15:04:05Z",
  "updated_at": null
}
```

- `GET /sales/clients` returns the standard `{items, has_next, skip,
  limit}` shape, ordered `id DESC`, with `limit` defaulting to 100 and capped
  at 200. `search` is trimmed and capped at 256 characters.
- Search matches `name`, `identity_card`, `email`, and `phone` by
  case-insensitive substring. `%` and `_` are literal characters; see
  [MIGRATION.md](MIGRATION.md#known-legacy-quirks).
- `name` is required, trimmed, and non-empty. `identity_card`, `email`, and
  `phone` are optional, trimmed, and stored as `null` when empty. Email and
  phone formats are not validated (legacy behavior).
- Create sets `created_at` to the current UTC instant and returns
  `updated_at: null`. Update is partial: omitted fields stay unchanged,
  explicit `null` clears one of the three optional fields, and
  `null`/empty/blank `name` is `422`. `updated_at` is set only when a field
  changes.
- Duplicate `identity_card`: `400` with `code: "UNIQUE_VIOLATION"`.
  Deleting a client referenced by a sale: `400` with
  `code: "FOREIGN_KEY_VIOLATION"`. Missing client: `404` with
  `detail: "Client not found"`.
- Timestamps are RFC 3339 UTC on both backends.

#### Sales

Sale response:

```json
{
  "id": 1,
  "total_amount": 21.2,
  "debt_amount": 15.95,
  "payment_status": "partial",
  "delivery_status": "partial",
  "payment_method": "cash",
  "amount_paid": 5.25,
  "client_id": 9,
  "created_at": "2026-10-24T15:04:05Z",
  "updated_at": null,
  "created_by_id": 2,
  "created_by": {
    "id": 2,
    "full_name": "Ana Pérez",
    "email": "ana@example.com",
    "is_active": true,
    "is_superuser": false
  },
  "client": {
    "id": 9,
    "name": "Carlos Ruiz",
    "identity_card": "1020304050",
    "email": "carlos@example.com",
    "phone": "3001234567",
    "created_at": "2026-10-24T15:04:05Z",
    "updated_at": null
  }
}
```

- `debt_amount` is `max(total_amount - amount_paid, 0)`.
- `created_by` is the full user without password fields and is never supplied
  by the client; it comes from the bearer token.
- `client` is the full client object or `null`.
- `payment_status` is `pending`, `partial`, or `paid`; `delivery_status` is
  `pending`, `partial`, or `delivered`; `payment_method` is `cash`, `credit`,
  `debit`, or `null`.
- Every list and detail response includes both relations.
- Lists order `id DESC`. `start_date` and `end_date` are RFC 3339 instants and
  inclusive. Invalid dates or enum values return `422 VALIDATION_ERROR`.
- `no_client=true` returns only sales with `client_id IS NULL`.

Sale item response:

```json
{
  "id": 1,
  "sale_id": 1,
  "product_id": 3,
  "quantity": 2,
  "delivered_quantity": 1,
  "unit_price": 10.5,
  "subtotal": 21.0,
  "product_name": "Martillo",
  "product_sku": "SKU-1234ABCD",
  "brand_name": "Truper",
  "material_name": "Acero",
  "category_name": "Herramientas"
}
```

- `GET /sales/{id}/items` orders `id ASC` with the shared pagination rules.
- `GET /sales/{id}/items` returns a paginated `SaleItem` list; add/update/delete item return the updated sale.
- Deleting a sale or an item restores stock. Missing sale is `404 Sale not
  found`; a missing item under an existing sale is `404 Sale item not found`.
- Adding a product already present in the sale is `400` with a readable
  detail. Insufficient stock is `400` with the legacy message.

Creating a sale requires at least one item. One item per product per sale.
`unit_price` defaults to the product price. The server calculates subtotals,
total, payment status, and delivery status.

Known legacy gap to preserve in v1:

- `amount_paid > total_amount` is accepted and the sale becomes `paid`; the UI
  prevents it but the API does not. PostgreSQL does enforce
  `delivered_quantity <= quantity`, so the create path must surface that
  constraint as a readable error.

### Supply chain

| Method | Path | Parameters / body |
|---|---|---|
| GET | `/supply-chain/providers` | `skip`, `limit`, `search` |
| POST | `/supply-chain/providers` | `name`, `contact_info?`, `email?`, `phone?` |
| GET | `/supply-chain/providers/{id}` | |
| PUT | `/supply-chain/providers/{id}` | Partial |
| DELETE | `/supply-chain/providers/{id}` | |
| GET | `/supply-chain/supplier-catalogs` | `skip`, `limit`, `provider_id`, `product_id` |
| GET | `/supply-chain/supplier-catalogs/{id}` | |
| GET | `/supply-chain/purchase-orders` | `skip`, `limit`, `provider_id`, `status`, `payment_status`, `min_date`, `max_date` |
| POST | `/supply-chain/purchase-orders` | `provider_id`, `status?`, `payment_status?`, `items[]` |
| GET | `/supply-chain/purchase-orders/{id}` | |
| PUT | `/supply-chain/purchase-orders/{id}` | Partial |
| DELETE | `/supply-chain/purchase-orders/{id}` | Does not reverse received stock |
| GET | `/supply-chain/purchase-orders/{id}/items` | `skip`, `limit` |
| POST | `/supply-chain/purchase-orders/{id}/items` | `product_id`, `quantity`, `unit_cost`, `supplier_sku?` |
| PUT | `/supply-chain/purchase-orders/{id}/items/{item_id}` | Partial |
| DELETE | `/supply-chain/purchase-orders/{id}/items/{item_id}` | |

#### Providers

Provider shape:

```json
{
  "id": 1,
  "name": "Distribuidora Metálica",
  "contact_info": "Juan Pérez",
  "email": "ventas@example.com",
  "phone": "3001234567"
}
```

- No timestamps. `created_by_id` comes from the bearer token and is not
  returned.
- `GET /supply-chain/providers` returns `{items, has_next, skip, limit}`,
  ordered `id DESC`; `limit` defaults to 100 and caps at 200.
- `search` is trimmed, capped at 256 characters, and matches `name`, `email`,
  and `phone` only, case-insensitively. `contact_info` is not searched. `%`
  and `_` are literal characters.
- `name` is required, trimmed, and at most 255 characters. `contact_info`
  (500), `email` (255), and `phone` (50) are optional, trimmed, and stored as
  `null` when empty. No email or phone format validation.
- Create accepts omitted, `null`, or empty optional fields. Update is partial:
  omitted fields stay unchanged, explicit `null` or an empty string clears an
  optional field, and `null`/blank `name` is `422 VALIDATION_ERROR`.
- Duplicate names are allowed. Deleting a provider referenced by a purchase
  order or supplier catalog is `400 FOREIGN_KEY_VIOLATION`; a missing provider
  is `404 Provider not found`.

#### Supplier catalogs

Supplier catalog shape:

```json
{
  "id": 1,
  "product_id": 3,
  "provider_id": 1,
  "supplier_sku": "PROV-001",
  "last_quoted_cost": 12.5,
  "lead_time": 7,
  "provider": {
    "id": 1,
    "name": "Distribuidora Metálica",
    "contact_info": "Juan Pérez",
    "email": "ventas@example.com",
    "phone": "3001234567"
  },
  "products": {
    "id": 3,
    "sku": "SKU-1234ABCD",
    "name": "Martillo",
    "size_value": "16 oz",
    "measurement_unit_id": 2,
    "price": 19.9,
    "cost": 10.0,
    "stock_quantity": 4,
    "low_stock_threshold": 5,
    "brand_id": 1,
    "category_id": 1,
    "material_id": 1
  }
}
```

- Read-only in this phase: list and get only. Rows are created or updated when
  a purchase order is received.
- `last_quoted_cost` is money and is serialized as an exact JSON number;
  `lead_time` is a nullable integer number of days. `products` is the plain
  product object with no relations.
- Lists return `{items, has_next, skip, limit}`, order `id DESC`, and share the
  100/200 rules. `provider_id` and `product_id` filters apply only when
  positive.
- Missing catalog: `404 Supplier catalog not found`.

#### Purchase orders

Order shape:

```json
{
  "id": 1,
  "provider_id": 3,
  "status": "sent",
  "payment_status": "pending",
  "order_date": "2026-10-24T15:04:05Z",
  "total_amount": 42.5,
  "provider": {
    "id": 3,
    "name": "Distribuidora Metálica",
    "contact_info": "Juan Pérez",
    "email": "ventas@example.com",
    "phone": "3001234567"
  }
}
```

Item shape:

```json
{
  "id": 1,
  "order_id": 1,
  "product_id": 3,
  "quantity": 2,
  "unit_cost": 10.25,
  "subtotal": 20.5,
  "supplier_sku": "PROV-001",
  "product_name": "Martillo",
  "product_sku": "SKU-1234ABCD",
  "brand_name": "Truper",
  "material_name": "Acero",
  "category_name": "Herramientas"
}
```

- `created_by` is never returned; `created_by_id` comes from the token on
  create. `order_date` is set at create and is RFC 3339 UTC.
- `total_amount` is the sum of item subtotals, calculated server-side.
- Statuses are `pending`, `sent`, `received`, `cancelled`; payment statuses are
  `pending`, `paid`.
- `GET /supply-chain/purchase-orders` returns `{items, has_next, skip, limit}`,
  ordered `id DESC`, with the shared 100/200 rules. Filters: `provider_id`,
  `status`, `payment_status`, `min_date`, `max_date`; dates are RFC 3339,
  inclusive, and compared to `order_date`. Invalid dates or enums are `422`.
- Create requires an existing provider (`400 Provider with id X not found`),
  at least one item, one item per product, `quantity > 0`,
  `unit_cost >= 0`, and `supplier_sku` of at most 100 characters. A missing
  product is `404 Product with id X not found`.
- Creating directly with `status: received` does **not** apply stock or
  supplier catalogs; only the update transition does (legacy behavior).
- Update is partial over `provider_id`, `status`, `payment_status`; explicit
  null is `422`. Changing a non-`received` order to `received` locks the item
  products in ascending id order, increments stock by each item quantity, and
  upserts `supplier_catalogs` with cost, whole-day lead time since
  `order_date`, and the supplier SKU when non-empty. An order already
  `received` changes neither stock nor catalogs.
- Delete removes items and the order and never reverses received stock.
- `GET /supply-chain/purchase-orders/{id}/items` verifies the order, orders
  `id ASC`, and uses the shared pagination rules. Item add/update/delete
  recalculate the order total. Duplicate products are `400`; a missing item is
  `404 Order item not found`. Item mutations on a received order are allowed
  and do not adjust stock or catalogs (legacy behavior).

### Dashboard

| Method | Path | Parameters |
|---|---|---|
| GET | `/dashboard/summary` | |
| GET | `/dashboard/sales-trend` | |
| GET | `/dashboard/top-products` | `limit` (default 10) |
| GET | `/dashboard/order-status-distribution` | |

Shapes:

```json
// summary
{ "inventory_value": 0, "total_sales_today": 0, "pending_orders": 0,
  "delivered_orders": 0, "pending_payment_sales": 0 }

// sales-trend
{ "data": [{ "date": "2026-10-24", "sales_amount": 0, "orders_cost": 0 }] }

// top-products
{ "products": [{ "product_id": 1, "product_name": "", "product_sku": "",
  "brand_name": null, "category_name": null, "material_name": null,
  "total_quantity_sold": 0, "total_sales_amount": 0 }] }

// order-status-distribution
{ "data": [{ "status": "pending", "count": 0, "percentage": 0 }] }
```

Known legacy semantics that must be preserved unless explicitly changed:

- `pending_payment_sales` is an amount, not a count.
- `inventory_value` uses sale price, not cost.
- Top products sum delivered quantities but rank by full subtotals.

## Endpoint count

67 legacy endpoints plus `GET /healthz`. The parity matrix in
[TESTING.md](TESTING.md) tracks each one.

## Parity rules

1. Status code, response shape, field names, and value types must match.
2. Money values compare numerically with a tolerance of zero after decimal
   parsing; do not compare floats.
3. Ordering must match for list and dashboard endpoints.
4. Timestamps compare as instants, not strings.
5. Additive fields are allowed only when the frontend does not break and the
   addition is documented here.
6. Removed or renamed fields require a frontend change in the same task.
7. Error codes may be richer than the legacy API, but `detail` must remain a
   readable string.
8. Any intentional difference gets an entry in [MIGRATION.md](MIGRATION.md)
   and [DECISIONS.md](DECISIONS.md).
