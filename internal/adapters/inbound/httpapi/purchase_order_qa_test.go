package httpapi_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
)

// These cases close gaps left by the phase tests: inclusive date bounds,
// reject-uppercase enums, item pagination, supplier-SKU edits, null update
// semantics, missing-order 404s, and item mutations on a received order.
func TestPurchaseOrderQAEdgeCasesBothBackends(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		runPurchaseOrderQA(t, newPOSQLiteBackend(t))
	})
	t.Run("postgres", func(t *testing.T) {
		runPurchaseOrderQA(t, newPOPostgresBackend(t))
	})
}

func runPurchaseOrderQA(t *testing.T, backend *poBackend) {
	t.Helper()

	t.Run("date filters are inclusive to the microsecond", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)

		instant := time.Date(2026, 10, 24, 15, 4, 5, 123456000, time.UTC)
		if err := backend.gorm.Exec(
			"UPDATE purchase_orders SET order_date = ? WHERE id = ?",
			gormstore.Timestamp(instant), order.ID,
		).Error; err != nil {
			t.Fatal(err)
		}

		list := func(minDate, maxDate time.Time) siList[poOrder] {
			t.Helper()
			path := "/api/v1/supply-chain/purchase-orders?provider_id=" + itoa(backend.providerID) +
				"&min_date=" + url.QueryEscape(minDate.Format(time.RFC3339Nano)) +
				"&max_date=" + url.QueryEscape(maxDate.Format(time.RFC3339Nano))
			status, raw := backend.request(t, http.MethodGet, path, nil)
			if status != http.StatusOK {
				t.Fatalf("list status = %d body = %s", status, raw)
			}
			return siDecode[siList[poOrder]](t, raw)
		}

		if got := list(instant, instant); len(got.Items) != 1 || got.Items[0].ID != order.ID {
			t.Fatalf("exact boundary list = %+v, want order %d", got.Items, order.ID)
		}
		if got := list(instant.Add(time.Microsecond), instant.Add(time.Hour)); len(got.Items) != 0 {
			t.Fatalf("min_date after order matched %+v", got.Items)
		}
		if got := list(instant.Add(-time.Hour), instant.Add(-time.Microsecond)); len(got.Items) != 0 {
			t.Fatalf("max_date before order matched %+v", got.Items)
		}

		if status, raw := backend.request(t, http.MethodDelete, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), nil); status != http.StatusOK {
			t.Fatalf("cleanup delete status = %d body = %s", status, raw)
		}
	})

	t.Run("money is exact and updates keep order_date", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 3, "unit_cost": 0.10}},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)
		assertNumber(t, order.TotalAmount, "0.30")
		item := firstOrderItem(t, backend, order.ID)
		assertNumber(t, item.UnitCost, "0.10")
		assertNumber(t, item.Subtotal, "0.30")

		status, raw = backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), map[string]any{
			"payment_status": "paid",
		})
		if status != http.StatusOK {
			t.Fatalf("update status = %d body = %s", status, raw)
		}
		updated := siDecode[poOrder](t, raw)
		if updated.OrderDate != order.OrderDate {
			t.Fatalf("order_date changed from %q to %q", order.OrderDate, updated.OrderDate)
		}
		if _, err := time.Parse(time.RFC3339, updated.OrderDate); err != nil {
			t.Fatalf("order_date = %q: %v", updated.OrderDate, err)
		}
	})

	t.Run("lead time is whole days and catalog stores cost and sku", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items": []map[string]any{
				{"product_id": backend.productIDs[3], "quantity": 2, "unit_cost": 10.25, "supplier_sku": "LEAD"},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)
		orderedAt := time.Now().UTC().Add(-50 * time.Hour).Truncate(time.Microsecond)
		if err := backend.gorm.Exec(
			"UPDATE purchase_orders SET order_date = ? WHERE id = ?",
			gormstore.Timestamp(orderedAt), order.ID,
		).Error; err != nil {
			t.Fatal(err)
		}

		status, raw = backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), map[string]any{
			"status": "received",
		})
		if status != http.StatusOK {
			t.Fatalf("receive status = %d body = %s", status, raw)
		}
		catalogs := backend.catalogsForProduct(t, backend.providerID, backend.productIDs[3])
		if len(catalogs) != 1 {
			t.Fatalf("catalogs = %+v, want one", catalogs)
		}
		catalog := catalogs[0]
		assertNumber(t, catalog.LastQuotedCost, "10.25")
		if catalog.SupplierSKU == nil || *catalog.SupplierSKU != "LEAD" {
			t.Fatalf("supplier sku = %v, want LEAD", catalog.SupplierSKU)
		}
		if catalog.LeadTime == nil || *catalog.LeadTime != 2 {
			t.Fatalf("lead_time = %v, want 2", catalog.LeadTime)
		}
	})

	t.Run("empty supplier sku keeps the existing catalog value", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items": []map[string]any{
				{"product_id": backend.productIDs[3], "quantity": 1, "unit_cost": 12},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)

		status, raw = backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), map[string]any{
			"status": "received",
		})
		if status != http.StatusOK {
			t.Fatalf("receive status = %d body = %s", status, raw)
		}
		catalogs := backend.catalogsForProduct(t, backend.providerID, backend.productIDs[3])
		if len(catalogs) != 1 {
			t.Fatalf("catalogs = %+v, want one", catalogs)
		}
		catalog := catalogs[0]
		assertNumber(t, catalog.LastQuotedCost, "12")
		if catalog.SupplierSKU == nil || *catalog.SupplierSKU != "LEAD" {
			t.Fatalf("supplier sku = %v, want the existing LEAD", catalog.SupplierSKU)
		}
	})

	t.Run("enums accept lowercase and reject uppercase", func(t *testing.T) {
		for _, body := range []map[string]any{
			{"provider_id": backend.providerID, "status": "RECEIVED", "items": []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}}},
			{"provider_id": backend.providerID, "payment_status": "PAID", "items": []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}}},
		} {
			status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", body)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("uppercase enum create status = %d body = %s, want 422", status, raw)
			}
		}
		for _, query := range []string{"status=SENT", "payment_status=PAID"} {
			status, raw := backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders?"+query, nil)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("uppercase enum filter %q status = %d body = %s, want 422", query, status, raw)
			}
		}

		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id":    backend.providerID,
			"status":         "sent",
			"payment_status": "paid",
			"items":          []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusOK {
			t.Fatalf("lowercase enum create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)
		if order.Status != "sent" || order.PaymentStatus != "paid" {
			t.Fatalf("response enums = %q/%q, want lowercase", order.Status, order.PaymentStatus)
		}
		var rawStatus, rawPayment string
		if err := backend.gorm.Raw(
			"SELECT status, payment_status FROM purchase_orders WHERE id = ?", order.ID,
		).Row().Scan(&rawStatus, &rawPayment); err != nil {
			t.Fatal(err)
		}
		if rawStatus != "SENT" || rawPayment != "PAID" {
			t.Fatalf("stored enums = %q/%q, want SENT/PAID", rawStatus, rawPayment)
		}

		status, raw = backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"status":      "cancelled",
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusOK {
			t.Fatalf("cancelled create status = %d body = %s", status, raw)
		}
		cancelled := siDecode[poOrder](t, raw)
		if cancelled.Status != "cancelled" {
			t.Fatalf("cancelled response status = %q", cancelled.Status)
		}
		if err := backend.gorm.Raw(
			"SELECT status FROM purchase_orders WHERE id = ?", cancelled.ID,
		).Row().Scan(&rawStatus); err != nil {
			t.Fatal(err)
		}
		if rawStatus != "CANCELLED" {
			t.Fatalf("stored cancelled status = %q, want CANCELLED", rawStatus)
		}
	})

	t.Run("item list paginates and missing order is 404", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items": []map[string]any{
				{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1},
				{"product_id": backend.productIDs[1], "quantity": 1, "unit_cost": 2},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)
		path := "/api/v1/supply-chain/purchase-orders/" + itoa(order.ID) + "/items"

		status, raw = backend.request(t, http.MethodGet, path+"?limit=1", nil)
		if status != http.StatusOK {
			t.Fatalf("first page status = %d body = %s", status, raw)
		}
		first := siDecode[siList[poItem]](t, raw)
		if !first.HasNext || len(first.Items) != 1 {
			t.Fatalf("first page = %+v, want one item with has_next", first)
		}

		status, raw = backend.request(t, http.MethodGet, path+"?skip=1&limit=1", nil)
		if status != http.StatusOK {
			t.Fatalf("second page status = %d body = %s", status, raw)
		}
		second := siDecode[siList[poItem]](t, raw)
		if second.HasNext || len(second.Items) != 1 || second.Items[0].ID <= first.Items[0].ID {
			t.Fatalf("second page = %+v, want a later item with no has_next", second)
		}

		status, raw = backend.request(t, http.MethodGet, path+"?limit=201", nil)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("limit 201 status = %d body = %s, want 422", status, raw)
		}

		status, raw = backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders/999999/items", nil)
		if status != http.StatusNotFound || !contains(raw, "Purchase order not found") {
			t.Fatalf("missing order items status = %d body = %s", status, raw)
		}
	})

	t.Run("item supplier sku can be set and cleared", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 2, "unit_cost": 3, "supplier_sku": "A"}},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)
		itemID := firstOrderItemID(t, backend, order.ID)

		path := "/api/v1/supply-chain/purchase-orders/" + itoa(order.ID) + "/items/" + itoa(itemID)
		status, raw = backend.request(t, http.MethodPut, path, map[string]any{"supplier_sku": "B"})
		if status != http.StatusOK {
			t.Fatalf("set sku status = %d body = %s", status, raw)
		}
		assertNumber(t, siDecode[poOrder](t, raw).TotalAmount, "6")
		if got := firstOrderItem(t, backend, order.ID); got.SupplierSKU == nil || *got.SupplierSKU != "B" {
			t.Fatalf("supplier sku after set = %v, want B", got.SupplierSKU)
		}

		status, raw = backend.request(t, http.MethodPut, path, map[string]any{"supplier_sku": nil})
		if status != http.StatusOK {
			t.Fatalf("clear sku status = %d body = %s", status, raw)
		}
		if got := firstOrderItem(t, backend, order.ID); got.SupplierSKU != nil {
			t.Fatalf("supplier sku after clear = %v, want nil", got.SupplierSKU)
		}
	})

	t.Run("explicit nulls are 422", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)
		itemID := firstOrderItemID(t, backend, order.ID)

		for _, field := range []string{"provider_id", "status", "payment_status"} {
			status, raw := backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), map[string]any{field: nil})
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("null %s status = %d body = %s, want 422", field, status, raw)
			}
		}
		for _, field := range []string{"quantity", "unit_cost"} {
			status, raw := backend.request(t, http.MethodPut,
				"/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items/"+itoa(itemID),
				map[string]any{field: nil})
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("null item %s status = %d body = %s, want 422", field, status, raw)
			}
		}
	})

	t.Run("received item mutations keep stock and catalogs", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items": []map[string]any{
				{"product_id": backend.productIDs[0], "quantity": 2, "unit_cost": 10.25, "supplier_sku": "ORIG"},
				{"product_id": backend.productIDs[1], "quantity": 1, "unit_cost": 1.25},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)
		orderPath := "/api/v1/supply-chain/purchase-orders/" + itoa(order.ID)

		status, raw = backend.request(t, http.MethodPut, orderPath, map[string]any{"status": "received"})
		if status != http.StatusOK {
			t.Fatalf("receive status = %d body = %s", status, raw)
		}
		stock0 := backend.stock(t, backend.productIDs[0])
		stock1 := backend.stock(t, backend.productIDs[1])
		stock2 := backend.stock(t, backend.productIDs[2])
		catalogCount := len(backend.catalogs(t, backend.providerID))

		status, raw = backend.request(t, http.MethodPost, orderPath+"/items", map[string]any{
			"product_id": backend.productIDs[2], "quantity": 3, "unit_cost": 5,
		})
		if status != http.StatusOK {
			t.Fatalf("add item status = %d body = %s", status, raw)
		}
		if backend.stock(t, backend.productIDs[2]) != stock2 || len(backend.catalogs(t, backend.providerID)) != catalogCount {
			t.Fatal("adding an item to a received order changed stock or catalogs")
		}
		addedID := orderItemIDForProduct(t, backend, order.ID, backend.productIDs[2])

		status, raw = backend.request(t, http.MethodPut, orderPath+"/items/"+itoa(addedID), map[string]any{
			"quantity": 4, "unit_cost": 6, "supplier_sku": "ADDED",
		})
		if status != http.StatusOK {
			t.Fatalf("update added item status = %d body = %s", status, raw)
		}
		if backend.stock(t, backend.productIDs[2]) != stock2 || len(backend.catalogs(t, backend.providerID)) != catalogCount {
			t.Fatal("updating a received order item changed stock or catalogs")
		}

		status, raw = backend.request(t, http.MethodDelete, orderPath+"/items/"+itoa(addedID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete added item status = %d body = %s", status, raw)
		}
		if backend.stock(t, backend.productIDs[2]) != stock2 || len(backend.catalogs(t, backend.providerID)) != catalogCount {
			t.Fatal("deleting a received order item changed stock or catalogs")
		}

		firstID := firstOrderItemID(t, backend, order.ID)
		status, raw = backend.request(t, http.MethodDelete, orderPath+"/items/"+itoa(firstID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete received item status = %d body = %s", status, raw)
		}
		if backend.stock(t, backend.productIDs[0]) != stock0 || backend.stock(t, backend.productIDs[1]) != stock1 {
			t.Fatal("deleting a received item changed stock")
		}
		for _, catalog := range backend.catalogsForProduct(t, backend.providerID, backend.productIDs[0]) {
			if catalog.SupplierSKU == nil || *catalog.SupplierSKU != "ORIG" || catalog.LastQuotedCost.String() != "10.25" {
				t.Fatalf("received item mutation changed catalog: %+v", catalog)
			}
		}

		status, raw = backend.request(t, http.MethodGet, orderPath, nil)
		if status != http.StatusOK {
			t.Fatalf("get received order status = %d body = %s", status, raw)
		}
		assertNumber(t, siDecode[poOrder](t, raw).TotalAmount, "1.25")
	})

	t.Run("missing order and item are 404", func(t *testing.T) {
		for _, request := range []struct {
			method string
			path   string
			body   any
		}{
			{http.MethodGet, "/api/v1/supply-chain/purchase-orders/999999", nil},
			{http.MethodPut, "/api/v1/supply-chain/purchase-orders/999999", map[string]any{"status": "sent"}},
			{http.MethodDelete, "/api/v1/supply-chain/purchase-orders/999999", nil},
			{http.MethodGet, "/api/v1/supply-chain/purchase-orders/999999/items", nil},
			{http.MethodPost, "/api/v1/supply-chain/purchase-orders/999999/items", map[string]any{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
			{http.MethodPut, "/api/v1/supply-chain/purchase-orders/999999/items/1", map[string]any{"quantity": 1}},
			{http.MethodDelete, "/api/v1/supply-chain/purchase-orders/999999/items/1", nil},
		} {
			status, raw := backend.request(t, request.method, request.path, request.body)
			if status != http.StatusNotFound || !contains(raw, "Purchase order not found") {
				t.Fatalf("%s %s status = %d body = %s, want 404 Purchase order not found", request.method, request.path, status, raw)
			}
		}

		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)
		itemPath := "/api/v1/supply-chain/purchase-orders/" + itoa(order.ID) + "/items/999999"
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			var body any
			if method == http.MethodPut {
				body = map[string]any{"quantity": 1}
			}
			status, raw := backend.request(t, method, itemPath, body)
			if status != http.StatusNotFound || !contains(raw, "Order item not found") {
				t.Fatalf("%s missing item status = %d body = %s, want 404 Order item not found", method, status, raw)
			}
		}
	})

	t.Run("update validates the provider reference", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		order := siDecode[poOrder](t, raw)

		status, raw = backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), map[string]any{
			"provider_id": 999999,
		})
		if status != http.StatusBadRequest || !contains(raw, "Provider with id 999999 not found") {
			t.Fatalf("missing provider update status = %d body = %s", status, raw)
		}
	})
}

func firstOrderItem(t *testing.T, backend *poBackend, orderID int64) poItem {
	t.Helper()
	status, raw := backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders/"+itoa(orderID)+"/items", nil)
	if status != http.StatusOK {
		t.Fatalf("items status = %d body = %s", status, raw)
	}
	items := siDecode[siList[poItem]](t, raw).Items
	if len(items) == 0 {
		t.Fatal("order has no items")
	}
	return items[0]
}

func firstOrderItemID(t *testing.T, backend *poBackend, orderID int64) int64 {
	t.Helper()
	return firstOrderItem(t, backend, orderID).ID
}

func orderItemIDForProduct(t *testing.T, backend *poBackend, orderID, productID int64) int64 {
	t.Helper()
	status, raw := backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders/"+itoa(orderID)+"/items?limit=200", nil)
	if status != http.StatusOK {
		t.Fatalf("items status = %d body = %s", status, raw)
	}
	for _, item := range siDecode[siList[poItem]](t, raw).Items {
		if item.ProductID == productID {
			return item.ID
		}
	}
	t.Fatalf("product %d not found in order %d", productID, orderID)
	return 0
}
