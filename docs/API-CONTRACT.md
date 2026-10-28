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
- Input validation rejects more than two decimal places.

### Dates and times

- JSON timestamps: RFC 3339, UTC (`2026-10-24T15:04:05Z`).
- Date-only trend keys: `YYYY-MM-DD`.
- Dashboard boundaries: `America/Bogota`.
- UI display: `DD/MM/YYYY`.

### Search

`search` is optional, trimmed, and limited to 256 characters. It matches the
entity's name fields as defined by the legacy repository. Do not change the
match columns while porting. Catalog search treats `%` and `_` literally as a
deliberate difference from legacy `ILIKE` wildcards; see
[MIGRATION.md](MIGRATION.md#known-legacy-quirks) and
[DECISIONS.md](DECISIONS.md#adr-0015--catalog-search-treats-sql-wildcards-literally).

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
measurement unit. Missing `sku` is generated as `SKU-XXXXXXXX`.

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

Creating an order requires at least one item. One item per product per order.
`status` transitions to `received` increment stock and upsert the supplier
catalog.

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
