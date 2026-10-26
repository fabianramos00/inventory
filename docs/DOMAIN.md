# Domain

## Business context

A hardware store (ferretería) sells products to walk-in customers and
contractors, buys stock from providers, and tracks what is owed and what has
been delivered. The desktop app runs at the counter and is used by staff with
different permission levels.

All UI copy is Spanish (`es-CO`). Domain values and API fields are English.

## Roles

| Role | Capabilities |
|---|---|
| Superuser (`is_superuser=true`) | Everything, including user management |
| User | Inventory, sales, clients, providers, purchase orders, dashboard |
| Inactive user (`is_active=false`) | Cannot authenticate; existing tokens are rejected |

There is no row-level ownership restriction in the legacy system. Any active
user can read and modify any business record. Do not invent per-user
authorization during the Go port; record it as a future change.

## Entities

### Catalog

| Entity | Key fields | Rules |
|---|---|---|
| `Category` | `name` unique, `code` unique optional, `description` | Cannot delete while referenced by products if the FK blocks it |
| `Material` | `name` unique | Same |
| `Brand` | `name` unique, `logo_url` | Same |
| `MeasurementUnit` | `name` unique, `abbreviation` unique (max 5) | Same |
| `Product` | `sku` unique, `name`, `size_value`, `price`, `cost`, `stock_quantity`, `low_stock_threshold`, FKs | `sku` is generated as `SKU-XXXXXXXX` when omitted |
| `PriceHistory` | `product_id`, `old_price`, `new_price`, `changed_at` | Cascades on product delete |

A product always has `brand`, `category`, and `material` in the current
contract. `measurement_unit` is optional. `size_value` is a string, not a
number.

### Sales

| Entity | Key fields | Rules |
|---|---|---|
| `Client` | `name`, `identity_card` unique optional, `email`, `phone` | A sale may have no client |
| `Sale` | `total_amount`, `amount_paid`, `payment_status`, `payment_method`, `delivery_status`, `client_id`, `created_by_id` | Totals are server-calculated |
| `SaleItem` | `sale_id`, `product_id`, `quantity`, `delivered_quantity`, `unit_price`, `subtotal` | One row per product per sale |

### Supply chain

| Entity | Key fields | Rules |
|---|---|---|
| `Provider` | `name`, `contact_info`, `email`, `phone` | Orders require a provider |
| `SupplierCatalog` | `product_id`, `provider_id`, `supplier_sku`, `last_quoted_cost`, `lead_time` | Created or updated when an order is received |
| `PurchaseOrder` | `provider_id`, `status`, `order_date`, `total_amount`, `payment_status` | Totals are server-calculated |
| `OrderItem` | `order_id`, `product_id`, `quantity`, `unit_cost`, `subtotal`, `supplier_sku` | One row per product per order |

### Users and auth

| Entity | Key fields | Rules |
|---|---|---|
| `User` | `full_name`, `email` unique, `hashed_password`, `is_active`, `is_superuser` | Passwords are bcrypt hashes |
| `RefreshToken` | `user_id`, `token_hash` unique, `expires_at`, `revoked` | Rotated on every refresh; cascades on user delete |

## Statuses

### Sale payment (`payment_status`)

| Value | Meaning |
|---|---|
| `pending` | `amount_paid = 0` |
| `partial` | `0 < amount_paid < total_amount` |
| `paid` | `amount_paid >= total_amount` |

The server derives this status from `amount_paid`. The client may send a
status, but the server value wins.

### Sale delivery (`delivery_status`)

| Value | Meaning |
|---|---|
| `pending` | All delivered quantities are zero |
| `partial` | At least one unit delivered, not all |
| `delivered` | Every item has `delivered_quantity >= quantity` |

Derived by the server from the sale items.

### Purchase order (`status`)

| Value | Meaning |
|---|---|
| `pending` | Created, not sent |
| `sent` | Sent to provider |
| `received` | Stock has been received |
| `cancelled` | Closed without receiving |

Transitioning to `received` is the only status change with stock side effects.

### Purchase order payment (`payment_status`)

| Value | Meaning |
|---|---|
| `pending` | Not paid |
| `paid` | Paid |

## Database representation

PostgreSQL stores SQLAlchemy enum **names**, not values:

| API/JSON | Database label |
|---|---|
| `cash` / `credit` / `debit` | `CASH` / `CREDIT` / `DEBIT` |
| `pending` / `partial` / `paid` (sale payment) | `PENDING` / `PARTIAL` / `PAID` |
| `pending` / `partial` / `delivered` (delivery) | `PENDING` / `PARTIAL` / `DELIVERED` |
| `pending` / `sent` / `received` / `cancelled` (order) | `PENDING` / `SENT` / `RECEIVED` / `CANCELLED` |
| `pending` / `paid` (order payment) | `PENDING` / `PAID` |

Go must map API lowercase values to database labels on writes and filters,
and map labels back to lowercase on reads. Filtering a cast enum with the
wrong case returns an empty result instead of an error. Sale payment and
purchase-order payment are different enum types; `PARTIAL` exists only for
sales.

Several legacy defaults are Python-side only and are absent from PostgreSQL:
`products.stock_quantity` (0), `products.low_stock_threshold` (10),
`sales.amount_paid` (0), `sale_items.delivered_quantity` (0), and
`purchase_orders.total_amount` (0). Go inserts must supply them explicitly.

## Invariants

1. `products.sku` is unique. `clients.identity_card` is unique when present.
2. `sale_items.quantity > 0`; `delivered_quantity` is between 0 and
   `quantity`. PostgreSQL enforces this with `ck_sale_items_delivered_valid`;
   the legacy add/update item paths also validate it. The create path must map
   the database constraint violation to a readable `400`/`422`.
3. `amount_paid` is non-negative. The UI prevents overpayment. The legacy API
   accepts `amount_paid > total_amount` and marks the sale paid; preserve that
   behavior in v1 and record the gap.
4. Sale creation decrements stock atomically. Insufficient stock rejects the
   whole sale.
5. Deleting a sale restores stock for every item.
6. Increasing a sale item quantity consumes stock; decreasing it restores
   stock.
7. Removing a sale item restores its stock.
8. `sale.total_amount` is the sum of item subtotals. Every item mutation
   recalculates the sale total, payment status, and delivery status.
9. A purchase order total is the sum of item subtotals.
10. Moving a purchase order from any status to `received` increments stock
    once and creates/updates supplier catalog rows. Receiving an
    already-received order has no effect.
11. Deleting a purchase order does **not** reverse received stock. This is a
    known legacy behavior; document it before changing it.
12. Money arithmetic is decimal. Never use `float64` for sums, subtotals, or
    balance calculations.
13. Stock is an integer. Do not introduce fractional stock.

## Stock rules

| Operation | Effect |
|---|---|
| Create sale | `stock -= quantity` per item, row-locked |
| Add sale item | `stock -= quantity` |
| Increase item quantity | `stock -= delta` |
| Decrease item quantity | `stock += |delta|` |
| Remove sale item | `stock += quantity` |
| Delete sale | `stock += quantity` per item |
| Receive purchase order | `stock += quantity` per item |
| Delete purchase order | No stock change |

Product rows are locked with `SELECT ... FOR UPDATE` before stock changes so
two concurrent sales cannot oversell the same unit.

## Money

- Stored as `NUMERIC(12,2)`.
- Go uses `shopspring/decimal`; SQL scanning never goes through `float64`.
- JSON carries money as a number, matching the legacy `MoneyInput` serializer.
- Display uses `es-CO` and COP: `$ 1.234,56`.
- Rounding is half away from zero only where a value must be rounded; the
  database remains the source of truth.

## Time

- All timestamps are stored in UTC.
- API timestamps are RFC 3339 with an offset.
- Display dates are `DD/MM/YYYY` in `America/Bogota`.
- “Today”, the 7-day trend, and top-product windows use `America/Bogota`
  boundaries.
- The 7-day trend includes days with zero sales.

## Dashboard semantics

These are the current legacy definitions. Preserve them during the port and
resolve any change explicitly.

| Metric | Definition |
|---|---|
| `inventory_value` | `sum(product.price * product.stock_quantity)` |
| `total_sales_today` | Sum of sale totals created since midnight Bogotá |
| `pending_payment_sales` | Sum of `total_amount - amount_paid` for sales whose payment status is not `paid` |
| `pending_orders` | Count of purchase orders with status `pending` |
| `delivered_orders` | Count of sales with delivery status `delivered` |
| `sales-trend` | Last 7 Bogotá days; sales totals and purchase-order costs; zero-filled |
| `top-products` | Last 7 days; grouped by product; ordered by subtotal |
| `order-status-distribution` | Purchase-order counts and percentages by status |

Known quirks:

- `pending_payment_sales` is a **money amount**, although the UI label reads
  like a count.
- Top products sum `delivered_quantity` but rank by the full item subtotal.
- `inventory_value` uses sale price, not cost.

## Deletion behavior

| Delete | Behavior |
|---|---|
| Product | Fails if referenced by sales, orders, or catalogs |
| Category / brand / material / unit | Fails if referenced by products |
| Client | Fails if referenced by sales; the FK must not cascade-delete sales |
| Provider | Fails if referenced by orders or catalogs |
| Sale | Deletes items and restores stock |
| Purchase order | Deletes items; received stock is not reversed |
| User | Fails or cascades refresh tokens; historical `created_by_id` may become null |
