# Phase Contract — Providers and Supplier Catalogs

Status: frozen
Owner: manager
Frozen at: pre-implementation
Scope: providers CRUD + supplier catalog reads

Purchase orders are explicitly out of scope for this phase. They will get
their own frozen contract before implementation.

## Endpoints

| Method | Path | Auth | Request | Response | Status |
|---|---|---|---|---|---|
| GET | `/api/v1/supply-chain/providers` | active user | `skip`, `limit`, `search` | paginated providers | 200 |
| POST | `/api/v1/supply-chain/providers` | active user | provider body | provider | 200 |
| GET | `/api/v1/supply-chain/providers/{id}` | active user | path | provider | 200 |
| PUT | `/api/v1/supply-chain/providers/{id}` | active user | partial provider | provider | 200 |
| DELETE | `/api/v1/supply-chain/providers/{id}` | active user | path | provider | 200 |
| GET | `/api/v1/supply-chain/supplier-catalogs` | active user | `skip`, `limit`, `provider_id`, `product_id` | paginated catalogs | 200 |
| GET | `/api/v1/supply-chain/supplier-catalogs/{id}` | active user | path | catalog | 200 |

## Provider

Response:

```json
{
  "id": 1,
  "name": "Distribuidora Metálica",
  "contact_info": "Juan Pérez",
  "email": "ventas@example.com",
  "phone": "3001234567"
}
```

No timestamps. `created_by_id` is set from the token on create but is not
returned.

Request:

| Key | Type | Required | Null | Rules |
|---|---|---|---|---|
| `name` | string | yes | no | trimmed, non-empty, max 255 |
| `contact_info` | string | no | clears | max 500 |
| `email` | string | no | clears | max 255; no format validation |
| `phone` | string | no | clears | max 50; no format validation |

Update is partial: omitted unchanged, explicit null or empty string clears an
optional field, null/blank name is `422`.

List:

- `{items, has_next, skip, limit}`, `id DESC`.
- `limit` default 100, max 200; `skip >= 0`.
- `search` trimmed, max 256, case-insensitive substring over **name, email,
  phone** (legacy columns; contact_info is not searched).
- `%` and `_` are literal.

Errors:

| Case | Status | Code | Detail |
|---|---|---|---|
| Validation | 422 | `VALIDATION_ERROR` | |
| Missing | 404 | | `Provider not found` |
| Referenced delete | 400 | `FOREIGN_KEY_VIOLATION` | readable detail |
| Unknown field | 422 | `VALIDATION_ERROR` | |

## Supplier catalog

Read-only endpoints. Creation and update happen only when a purchase order is
received.

Response:

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

- `last_quoted_cost` is money, JSON number, two decimals.
- `lead_time` is days, nullable integer.
- List `{items, has_next, skip, limit}`, `id DESC`, filters
  `provider_id`/`product_id` when positive.
- Missing: `404 Supplier catalog not found`.
- No create/update/delete routes in this phase.

## Frontend mapping

- `Provider` type matches the response exactly.
- `providersApi.getProviders` returns `{items, has_next, skip, limit}`.
- `useProviders` exposes `hasNext`; `Providers` page uses cursor pagination.
- Route: `/providers`.
- Provider edit sends explicit `null` for cleared optional fields.
- No supplier catalog UI in this phase.

## Database notes

- Providers: `name varchar(255)`, `contact_info varchar(500)`,
  `email varchar(255)`, `phone varchar(50)`, `created_by_id` nullable.
- Supplier catalogs: `product_id`/`provider_id` nullable FKs,
  `supplier_sku varchar(100)`, `last_quoted_cost NUMERIC(12,2)` /
  SQLite integer cents, `lead_time` nullable integer.
- Money and search normalization follow ADR-0015/ADR-0016.

## Acceptance

- [ ] Provider CRUD tested on PostgreSQL + SQLite.
- [ ] Provider search matches name/email/phone; contact_info not searched.
- [ ] Provider delete conflict tested with a purchase order and catalog.
- [ ] Supplier catalog list/get tested, including relations and money.
- [ ] Frontend provider route, cursor pagination, and clearing tested.
- [ ] Docs and parity matrix updated.
- [ ] Backend and frontend verification commands pass.
