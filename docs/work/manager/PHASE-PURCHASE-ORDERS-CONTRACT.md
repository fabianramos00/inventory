# Phase Contract — Purchase Orders

Status: frozen
Owner: manager
Frozen at: pre-implementation
Scope: purchase orders, order items, receiving

## Endpoints

| Method | Path | Auth | Request | Response | Status |
|---|---|---|---|---|---|
| GET | `/api/v1/supply-chain/purchase-orders` | active user | `skip`, `limit`, `provider_id`, `status`, `payment_status`, `min_date`, `max_date` | paginated orders | 200 |
| POST | `/api/v1/supply-chain/purchase-orders` | active user | create body | order | 200 |
| GET | `/api/v1/supply-chain/purchase-orders/{id}` | active user | path | order | 200 |
| PUT | `/api/v1/supply-chain/purchase-orders/{id}` | active user | partial body | order | 200 |
| DELETE | `/api/v1/supply-chain/purchase-orders/{id}` | active user | path | order | 200 |
| GET | `/api/v1/supply-chain/purchase-orders/{id}/items` | active user | `skip`, `limit` | paginated items | 200 |
| POST | `/api/v1/supply-chain/purchase-orders/{id}/items` | active user | item body | updated order | 200 |
| PUT | `/api/v1/supply-chain/purchase-orders/{id}/items/{item_id}` | active user | partial item | updated order | 200 |
| DELETE | `/api/v1/supply-chain/purchase-orders/{id}/items/{item_id}` | active user | path | updated order | 200 |

## Purchase order response

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

- No `created_by` in the response; `created_by_id` is set from the token on
  create and never accepted from the body.
- `total_amount` = sum of item subtotals.
- `order_date` is set at create; updates do not change it.
- Statuses: `pending`, `sent`, `received`, `cancelled`.
- Payment statuses: `pending`, `paid`.

## Create

| Key | Type | Required | Default | Rules |
|---|---|---|---|---|
| `provider_id` | int | yes | | must exist; missing -> `400 Provider with id X not found` |
| `status` | enum | no | `pending` | valid enum |
| `payment_status` | enum | no | `pending` | valid enum |
| `items` | array | yes | | at least one; one per product |
| `items[].product_id` | int | yes | | must exist -> `404 Product with id X not found` |
| `items[].quantity` | int | yes | | > 0 |
| `items[].unit_cost` | money | yes | | >= 0 |
| `items[].supplier_sku` | string | no | null | max 100 |

`subtotal = quantity * unit_cost`. No unknown fields.

Legacy behavior preserved: creating directly with `status: received` inserts
the order but does **not** apply stock or upsert supplier catalogs. Receiving
side effects happen only on the `received` transition in `PUT`.

## Update

Partial: `provider_id`, `status`, `payment_status`. Omitted unchanged.
Explicit null for any field is `422`.

When status changes **from any non-received value to `received`**:

- lock all item products in ascending id order;
- increment `stock_quantity` by each item quantity;
- upsert `supplier_catalogs` for `(product_id, provider_id)`:
  - `last_quoted_cost = item.unit_cost`;
  - `lead_time = whole days between order_date and now`;
  - `supplier_sku = item.supplier_sku` when non-empty, otherwise keep the
    existing value;
- missing product -> `404 Product with id X not found`;
- all of this happens in one transaction;
- if the order was already `received`, no stock/catalog change.

Other status changes have no stock side effects. Invalid status/payment values
are `422`.

## Delete

Deletes the order and its items. **Never reverses received stock.** This is a
known legacy behavior and must be documented.

## Items

Item response:

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

- List verifies the order exists, orders `id ASC`, cursor pagination
  default 100/cap 200.
- Add: order must exist; product must exist; one item per product; quantity > 0;
  unit_cost >= 0; duplicate -> `400` readable detail; recalculates order total.
- Update: partial `quantity`, `unit_cost`, `supplier_sku`; quantity > 0;
  unit_cost >= 0; recalculates subtotal and order total; explicit null for
  non-null fields is `422`; `supplier_sku: null` clears it.
- Delete: removes the item and recalculates the order total.
- Item mutations on an already-received order are allowed for legacy parity and
  do **not** adjust stock or supplier catalogs. Document this gap.
- Missing order -> `404 Purchase order not found`.
- Missing item -> `404 Order item not found`.

## List

- `{items, has_next, skip, limit}`, `id DESC`.
- `limit` default 100, max 200.
- Filters: `provider_id`, `status`, `payment_status`, `min_date`, `max_date`.
- `min_date`/`max_date` are RFC 3339 instants, inclusive, compared to
  `order_date`.
- Invalid dates/enums -> `422 VALIDATION_ERROR`.

## Frontend mapping

- `Order` and `OrderItem` types match the responses exactly.
- `ordersApi` uses cursor pagination.
- Routes: `/orders`, `/orders/create`, `/orders/:id`.
- Date filters convert to `America/Bogota` day bounds (start 00:00:00,
  end 23:59:59.999, `-05:00`) before sending.
- Update to `received` invalidates `products`, `inventoryStats`, supplier
  catalogs, dashboard order status, and order keys.
- Create/delete invalidate orders + dashboard.
- Item mutations invalidate `order`, `orderItems`, `orders`.
- API errors surface backend `detail` with `role="alert"`.

## Database notes

- `purchase_orders`: `provider_id` nullable FK, `status`/`payment_status`
  uppercase enums, `order_date` timestamptz / UTC text, `total_amount`
  numeric(12,2) / integer cents, `created_by_id` nullable.
- `order_items`: `order_id`/`product_id` FKs, integer quantity,
  `unit_cost`/`subtotal` money, `supplier_sku` varchar(100).
- `supplier_catalogs` has no unique `(product_id, provider_id)` constraint;
  receiving uses select-then-insert/update inside the transaction.
- Extend `outbound.TxRepos` with `Orders`, `Catalogs`, and `Providers`;
  keep the existing `UnitOfWork` and `StockRepository`.

## Acceptance

- [ ] 9 endpoints tested on PostgreSQL + SQLite.
- [ ] Receiving increments stock exactly once and upserts catalogs.
- [ ] Concurrent receiving cannot double-increment.
- [ ] Delete does not reverse received stock.
- [ ] Order totals recalculated after every item mutation.
- [ ] Frontend routes, date bounds, invalidation, and payloads tested.
- [ ] Docs + parity matrix updated.
- [ ] All verification commands pass.
