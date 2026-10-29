# Phase Contract — Dashboard

Status: frozen
Owner: manager
Frozen at: pre-implementation
Scope: 4 read-only aggregate endpoints

## Endpoints

| Method | Path | Auth | Query | Response |
|---|---|---|---|---|
| GET | `/api/v1/dashboard/summary` | active user | | summary |
| GET | `/api/v1/dashboard/sales-trend` | active user | | `{data: [...]}` |
| GET | `/api/v1/dashboard/top-products` | active user | `limit` (default 10, max 100) | `{products: [...]}` |
| GET | `/api/v1/dashboard/order-status-distribution` | active user | | `{data: [...]}` |

All responses `200`. No writes, no migrations, no cache.

## Summary

```json
{
  "inventory_value": 0,
  "total_sales_today": 0,
  "pending_orders": 0,
  "delivered_orders": 0,
  "pending_payment_sales": 0
}
```

- `inventory_value = SUM(products.price * products.stock_quantity)`.
  Uses **sale price**, not cost.
- `total_sales_today = SUM(sales.total_amount)` for sales created since
  midnight `America/Bogota`.
- `pending_payment_sales = SUM(sales.total_amount - sales.amount_paid)` for
  sales whose `payment_status != paid`. This is **money**, not a count.
- `pending_orders = COUNT(purchase_orders)` where status is `pending`.
- `delivered_orders = COUNT(sales)` where `delivery_status` is `delivered`.
  This counts **sales**, not purchase orders.
- Empty database returns all zeros.
- Money: decimal, JSON number. PG `NUMERIC(12,2)`; SQLite integer cents.

## Sales trend

```json
{
  "data": [
    { "date": "2026-10-24", "sales_amount": 0, "orders_cost": 0 }
  ]
}
```

- Exactly 7 entries, oldest first, ending today.
- Days are `America/Bogota` calendar days.
- Zero-filled for days with no rows.
- `sales_amount` = sum of sale totals created that day.
- `orders_cost` = sum of purchase-order `total_amount` with `order_date` that
  day.
- Date keys are `YYYY-MM-DD` strings.

Database rules:

- PostgreSQL: `(created_at AT TIME ZONE 'America/Bogota')::date`.
- SQLite: derive the Bogota date from the stored UTC text with the fixed
  `-05:00` offset (Colombia has no DST), or bucket in Go. Do not use the host
  timezone.
- The seven dates are generated in Go from the Bogota "today".

## Top products

```json
{
  "products": [
    {
      "product_id": 1,
      "product_name": "Martillo",
      "product_sku": "SKU-1234ABCD",
      "brand_name": null,
      "category_name": "Herramientas",
      "material_name": "Acero",
      "total_quantity_sold": 3,
      "total_sales_amount": 21.0
    }
  ]
}
```

- Window: last 7 days from `now`, matching legacy (`now - 7 days` in UTC).
- Join `sale_items` → `sales` → `products`; left join brand/category/material.
- Group by product.
- `total_quantity_sold = SUM(sale_items.delivered_quantity)`.
- `total_sales_amount = SUM(sale_items.subtotal)`.
- Order by `SUM(sale_items.subtotal) DESC`.
- `limit` default 10, minimum 1, maximum 100.
- Brand/category/material names are `null` when the relation is missing.
- Empty result -> `{"products": []}`.

## Order status distribution

```json
{
  "data": [
    { "status": "pending", "count": 2, "percentage": 50.0 }
  ]
}
```

- Group purchase orders by status.
- Status lowercase in JSON.
- Order by count descending.
- Percentage = `count / total * 100`; `0` when total is 0.
- Statuses with no rows are omitted; empty database -> `{"data": []}`.

## Database / money

- PostgreSQL: `NUMERIC(12,2)` sums as text/decimal; never `float64`.
- SQLite: integer cents; convert once at the adapter boundary.
- `percentage` is a JSON number, two-decimal rounding optional but stable.
- Timestamps are compared as UTC instants; date keys are display dates in
  Bogota.

## Frontend mapping

- Existing `dashboardApi` endpoints and `useDashboard` queries stay.
- Types:
  - `TopProduct.brand_name`, `category_name`, `material_name` are
    `string | null`.
  - Summary money fields are numbers.
- Dashboard KPI cards:
  - `Ventas de hoy` = `total_sales_today` as COP.
  - `Ventas sin pagar` = `pending_payment_sales` as COP, never a count.
  - `Pedidos pendientes` = `pending_orders`.
  - `Stock bajo` remains from inventory stats; not part of this contract.
- Trend chart uses `date` strings without timezone conversion that shifts the
  day.
- Empty/loading/error states per section remain.
- No new route; `/` already renders Dashboard.

## Acceptance

- [ ] 4 endpoints tested on PostgreSQL + SQLite.
- [ ] Empty DB -> zeros / 7 zero-filled days / empty arrays.
- [ ] Bogota midnight boundary test.
- [ ] `pending_payment_sales` is money and never counts.
- [ ] `delivered_orders` counts sales.
- [ ] Top products rank by subtotal and quantity uses delivered quantity.
- [ ] Nullable brand/category/material names.
- [ ] Status distribution percentages and ordering.
- [ ] Frontend tests for KPI formatting, trend, top products, empty data.
- [ ] Docs + parity matrix updated.
- [ ] All verification commands pass.
