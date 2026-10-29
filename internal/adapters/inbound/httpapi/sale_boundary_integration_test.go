package httpapi_test

import (
	"database/sql"
	"net/http"
	"testing"

	"inventory-desktop/internal/domain"
)

// These tests cover the sales transaction boundary and the gaps not exercised
// by sale_integration_test.go: rollback on insufficient stock, status/filter
// mapping, update null semantics, item quantity deltas, empty stats, and the
// rule that sales never write price_history.

type siErrorBody struct {
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func forEachSaleBackend(t *testing.T, fn func(t *testing.T, b *siBackend)) {
	t.Helper()
	for _, backend := range []struct {
		name string
		open func(*testing.T) *siBackend
	}{
		{"sqlite", newSISQLiteBackend},
		{"postgres", newSIPostgresBackend},
	} {
		t.Run(backend.name, func(t *testing.T) {
			fn(t, backend.open(t))
		})
	}
}

func TestSaleCreateValidationAndRollback(t *testing.T) {
	forEachSaleBackend(t, func(t *testing.T, b *siBackend) {
		status, raw := b.request(t, http.MethodGet, "/api/v1/sales/stats", nil)
		if status != http.StatusOK {
			t.Fatalf("empty stats status = %d body = %s", status, raw)
		}
		empty := siDecode[siStats](t, raw)
		if empty.UnpaidCount != 0 || empty.UndeliveredCount != 0 {
			t.Fatalf("empty stats counts = %+v, want zeros", empty)
		}
		assertNumber(t, empty.TotalAmountSum, "0")
		assertNumber(t, empty.AmountPaidSum, "0")

		cases := []struct {
			name   string
			body   map[string]any
			status int
			detail string
		}{
			{
				name:   "no items",
				body:   map[string]any{"items": []map[string]any{}},
				status: http.StatusUnprocessableEntity,
			},
			{
				name: "duplicate product",
				body: map[string]any{"items": []map[string]any{
					{"product_id": b.productIDs[0], "quantity": 1},
					{"product_id": b.productIDs[0], "quantity": 1},
				}},
				status: http.StatusUnprocessableEntity,
			},
			{
				name: "delivered over quantity",
				body: map[string]any{"items": []map[string]any{
					{"product_id": b.productIDs[0], "quantity": 1, "delivered_quantity": 2},
				}},
				status: http.StatusUnprocessableEntity,
			},
			{
				name: "negative amount paid",
				body: map[string]any{"amount_paid": -1, "items": []map[string]any{
					{"product_id": b.productIDs[0], "quantity": 1},
				}},
				status: http.StatusUnprocessableEntity,
			},
			{
				name: "negative unit price",
				body: map[string]any{"items": []map[string]any{
					{"product_id": b.productIDs[0], "quantity": 1, "unit_price": -0.01},
				}},
				status: http.StatusUnprocessableEntity,
			},
			{
				name: "missing product",
				body: map[string]any{"items": []map[string]any{
					{"product_id": 999999, "quantity": 1},
				}},
				status: http.StatusNotFound,
				detail: "Product with id 999999 not found",
			},
			{
				name: "missing client",
				body: map[string]any{"client_id": 999999, "items": []map[string]any{
					{"product_id": b.productIDs[0], "quantity": 1},
				}},
				status: http.StatusNotFound,
				detail: "Client with id 999999 not found",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				status, raw := b.request(t, http.MethodPost, "/api/v1/sales", tc.body)
				if status != tc.status {
					t.Fatalf("status = %d body = %s, want %d", status, raw, tc.status)
				}
				if tc.detail != "" && !contains(raw, tc.detail) {
					t.Fatalf("detail = %s, want %q", raw, tc.detail)
				}
			})
		}

		beforeSales := siScalarInt(t, b, "SELECT COUNT(*) FROM sales")
		beforeHistory := siScalarInt(t, b, "SELECT COUNT(*) FROM price_history")
		beforeFirst := b.stock(t, b.productIDs[0])
		beforeSecond := b.stock(t, b.productIDs[1])

		status, raw = b.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
			"items": []map[string]any{
				{"product_id": b.productIDs[2], "quantity": 2, "unit_price": 1.00},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("explicit unit price status = %d body = %s", status, raw)
		}
		explicit := siDecode[siSale](t, raw)
		assertNumber(t, explicit.TotalAmount, "2.00")
		status, raw = b.request(t, http.MethodGet, "/api/v1/sales/"+itoa(explicit.ID)+"/items", nil)
		if status != http.StatusOK {
			t.Fatalf("explicit unit price items status = %d body = %s", status, raw)
		}
		explicitItems := siDecode[siList[siItem]](t, raw)
		if len(explicitItems.Items) != 1 {
			t.Fatalf("explicit unit price items = %+v", explicitItems.Items)
		}
		assertNumber(t, explicitItems.Items[0].UnitPrice, "1")
		assertNumber(t, explicitItems.Items[0].Subtotal, "2.00")
		if status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(explicit.ID), nil); status != http.StatusOK {
			t.Fatalf("explicit unit price cleanup status = %d body = %s", status, raw)
		}
		if got := b.stock(t, b.productIDs[2]); got != 10 {
			t.Fatalf("product 3 stock after explicit price cleanup = %d, want 10", got)
		}

		status, raw = b.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
			"items": []map[string]any{
				{"product_id": b.productIDs[0], "quantity": 1},
				{"product_id": b.productIDs[1], "quantity": 999},
			},
		})
		if status != http.StatusBadRequest {
			t.Fatalf("insufficient rollback status = %d body = %s", status, raw)
		}
		errBody := siDecode[siErrorBody](t, raw)
		if errBody.Code != "INSUFFICIENT_STOCK" || !contains(raw, "Insufficient stock for product") {
			t.Fatalf("insufficient rollback body = %s", raw)
		}
		if got := b.stock(t, b.productIDs[0]); got != beforeFirst {
			t.Fatalf("first product stock = %d, want unchanged %d", got, beforeFirst)
		}
		if got := b.stock(t, b.productIDs[1]); got != beforeSecond {
			t.Fatalf("second product stock = %d, want unchanged %d", got, beforeSecond)
		}
		if got := siScalarInt(t, b, "SELECT COUNT(*) FROM sales"); got != beforeSales {
			t.Fatalf("sale rows = %d, want unchanged %d", got, beforeSales)
		}
		if got := siScalarInt(t, b, "SELECT COUNT(*) FROM price_history"); got != beforeHistory {
			t.Fatalf("price_history rows = %d, want unchanged %d", got, beforeHistory)
		}
	})
}

func TestSaleStatusesFiltersAndUpdate(t *testing.T) {
	forEachSaleBackend(t, func(t *testing.T, b *siBackend) {
		status, raw := b.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
			"items": []map[string]any{{"product_id": b.productIDs[0], "quantity": 2}},
		})
		if status != http.StatusOK {
			t.Fatalf("pending create status = %d body = %s", status, raw)
		}
		pending := siDecode[siSale](t, raw)
		assertNumber(t, pending.TotalAmount, "21.00")
		assertNumber(t, pending.AmountPaid, "0")
		assertNumber(t, pending.DebtAmount, "21.00")
		if pending.PaymentStatus != domain.PaymentStatusPending || pending.DeliveryStatus != domain.DeliveryStatusPending {
			t.Fatalf("pending statuses = %q/%q", pending.PaymentStatus, pending.DeliveryStatus)
		}
		if pending.PaymentMethod != nil {
			t.Fatalf("pending payment_method = %v, want nil", pending.PaymentMethod)
		}
		assertRawSaleEnum(t, b, pending.ID, "PENDING", "PENDING", "")
		assertRawSaleTotal(t, b, pending.ID, "21.00")

		status, raw = b.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
			"payment_method": "debit",
			"amount_paid":    0.40,
			"items": []map[string]any{
				{"product_id": b.productIDs[1], "quantity": 2, "delivered_quantity": 2},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("delivered create status = %d body = %s", status, raw)
		}
		delivered := siDecode[siSale](t, raw)
		assertNumber(t, delivered.TotalAmount, "0.40")
		assertNumber(t, delivered.AmountPaid, "0.40")
		assertNumber(t, delivered.DebtAmount, "0")
		if delivered.PaymentStatus != domain.PaymentStatusPaid || delivered.DeliveryStatus != domain.DeliveryStatusDelivered {
			t.Fatalf("delivered statuses = %q/%q", delivered.PaymentStatus, delivered.DeliveryStatus)
		}
		if delivered.PaymentMethod == nil || *delivered.PaymentMethod != domain.PaymentMethodDebit {
			t.Fatalf("delivered payment_method = %v", delivered.PaymentMethod)
		}
		assertRawSaleEnum(t, b, delivered.ID, "PAID", "DELIVERED", "DEBIT")

		status, raw = b.request(t, http.MethodGet,
			"/api/v1/sales?payment_status=paid&delivery_status=delivered&payment_method=debit", nil)
		if status != http.StatusOK {
			t.Fatalf("filter status = %d body = %s", status, raw)
		}
		filtered := siDecode[siList[siSale]](t, raw)
		if len(filtered.Items) != 1 || filtered.Items[0].ID != delivered.ID {
			t.Fatalf("filtered sales = %+v, want only %d", filtered.Items, delivered.ID)
		}

		status, raw = b.request(t, http.MethodGet, "/api/v1/sales?user_id="+itoa(b.userID), nil)
		if status != http.StatusOK {
			t.Fatalf("user filter status = %d body = %s", status, raw)
		}
		if byUser := siDecode[siList[siSale]](t, raw); len(byUser.Items) != 2 {
			t.Fatalf("user filter returned %d sales, want 2", len(byUser.Items))
		}

		status, raw = b.request(t, http.MethodGet, "/api/v1/sales?client_id="+itoa(b.clientID), nil)
		if status != http.StatusOK {
			t.Fatalf("client filter status = %d body = %s", status, raw)
		}
		if byClient := siDecode[siList[siSale]](t, raw); len(byClient.Items) != 0 {
			t.Fatalf("client filter returned %+v, want none", byClient.Items)
		}

		status, raw = b.request(t, http.MethodGet, "/api/v1/sales?no_client=true", nil)
		if status != http.StatusOK {
			t.Fatalf("no_client filter status = %d body = %s", status, raw)
		}
		if noClient := siDecode[siList[siSale]](t, raw); len(noClient.Items) != 2 {
			t.Fatalf("no_client returned %d sales, want 2", len(noClient.Items))
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(pending.ID), map[string]any{
			"client_id": 999999,
		})
		if status != http.StatusNotFound || !contains(raw, "Client with id 999999 not found") {
			t.Fatalf("missing client update status = %d body = %s", status, raw)
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(pending.ID), map[string]any{
			"client_id": b.clientID,
		})
		if status != http.StatusOK {
			t.Fatalf("client update status = %d body = %s", status, raw)
		}
		linked := siDecode[siSale](t, raw)
		if linked.Client == nil || linked.Client.ID != b.clientID || linked.ClientID == nil || *linked.ClientID != b.clientID {
			t.Fatalf("linked client = %+v id %v", linked.Client, linked.ClientID)
		}
		if linked.UpdatedAt == nil {
			t.Fatal("updated_at is nil after client update")
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(pending.ID), map[string]any{
			"client_id": nil,
		})
		if status != http.StatusOK {
			t.Fatalf("client clear status = %d body = %s", status, raw)
		}
		cleared := siDecode[siSale](t, raw)
		if cleared.Client != nil || cleared.ClientID != nil {
			t.Fatalf("cleared client = %+v id %v", cleared.Client, cleared.ClientID)
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(pending.ID), map[string]any{
			"amount_paid": nil,
		})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("null amount_paid status = %d body = %s, want 422", status, raw)
		}
		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(pending.ID), map[string]any{
			"payment_method": "cheque",
		})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("invalid payment_method status = %d body = %s, want 422", status, raw)
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(pending.ID), map[string]any{
			"amount_paid": 100,
		})
		if status != http.StatusOK {
			t.Fatalf("overpay update status = %d body = %s", status, raw)
		}
		overpaid := siDecode[siSale](t, raw)
		if overpaid.PaymentStatus != domain.PaymentStatusPaid {
			t.Fatalf("overpaid status = %q, want paid", overpaid.PaymentStatus)
		}
		assertNumber(t, overpaid.DebtAmount, "0")

		for _, path := range []string{"/api/v1/sales/999999", "/api/v1/sales/999999/items"} {
			status, raw = b.request(t, http.MethodGet, path, nil)
			if status != http.StatusNotFound {
				t.Fatalf("GET %s status = %d body = %s, want 404", path, status, raw)
			}
		}
		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/999999", map[string]any{"amount_paid": 1})
		if status != http.StatusNotFound {
			t.Fatalf("update missing sale status = %d body = %s, want 404", status, raw)
		}
		status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/999999", nil)
		if status != http.StatusNotFound {
			t.Fatalf("delete missing sale status = %d body = %s, want 404", status, raw)
		}

		if status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(pending.ID), nil); status != http.StatusOK {
			t.Fatalf("cleanup pending sale status = %d body = %s", status, raw)
		}
		if status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(delivered.ID), nil); status != http.StatusOK {
			t.Fatalf("cleanup delivered sale status = %d body = %s", status, raw)
		}
	})
}

func TestSaleItemMutationsStockAndStats(t *testing.T) {
	forEachSaleBackend(t, func(t *testing.T, b *siBackend) {
		beforeHistory := siScalarInt(t, b, "SELECT COUNT(*) FROM price_history")
		status, raw := b.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
			"items": []map[string]any{{"product_id": b.productIDs[0], "quantity": 2}},
		})
		if status != http.StatusOK {
			t.Fatalf("create status = %d body = %s", status, raw)
		}
		sale := siDecode[siSale](t, raw)

		status, raw = b.request(t, http.MethodPost, "/api/v1/sales/"+itoa(sale.ID)+"/items", map[string]any{
			"product_id": b.productIDs[0], "quantity": 1,
		})
		if status != http.StatusBadRequest || !contains(raw, "already exists in this sale") {
			t.Fatalf("duplicate item status = %d body = %s, want 400", status, raw)
		}

		status, raw = b.request(t, http.MethodPost, "/api/v1/sales/"+itoa(sale.ID)+"/items", map[string]any{
			"product_id": b.productIDs[2], "quantity": 999,
		})
		if status != http.StatusBadRequest || !contains(raw, "Insufficient stock") {
			t.Fatalf("insufficient add status = %d body = %s, want 400", status, raw)
		}
		if got := b.stock(t, b.productIDs[2]); got != 10 {
			t.Fatalf("product 3 stock after rejected add = %d, want 10", got)
		}

		status, raw = b.request(t, http.MethodPost, "/api/v1/sales/"+itoa(sale.ID)+"/items", map[string]any{
			"product_id": b.productIDs[1], "quantity": 2,
		})
		if status != http.StatusOK {
			t.Fatalf("add item status = %d body = %s", status, raw)
		}
		added := siDecode[siSale](t, raw)
		assertNumber(t, added.TotalAmount, "21.40")
		if got := b.stock(t, b.productIDs[1]); got != 8 {
			t.Fatalf("product 2 stock after add = %d, want 8", got)
		}

		status, raw = b.request(t, http.MethodGet, "/api/v1/sales/"+itoa(sale.ID)+"/items?limit=1", nil)
		if status != http.StatusOK {
			t.Fatalf("first item page status = %d body = %s", status, raw)
		}
		firstPage := siDecode[siList[siItem]](t, raw)
		if !firstPage.HasNext || len(firstPage.Items) != 1 {
			t.Fatalf("first item page = %+v, want one item and has_next", firstPage)
		}
		status, raw = b.request(t, http.MethodGet, "/api/v1/sales/"+itoa(sale.ID)+"/items?skip=1&limit=1", nil)
		if status != http.StatusOK {
			t.Fatalf("second item page status = %d body = %s", status, raw)
		}
		secondPage := siDecode[siList[siItem]](t, raw)
		if secondPage.HasNext || len(secondPage.Items) != 1 || secondPage.Items[0].ID <= firstPage.Items[0].ID {
			t.Fatalf("second item page = %+v, want a later id and has_next false", secondPage)
		}

		var nailsItem siItem
		status, raw = b.request(t, http.MethodGet, "/api/v1/sales/"+itoa(sale.ID)+"/items", nil)
		if status != http.StatusOK {
			t.Fatalf("items status = %d body = %s", status, raw)
		}
		for _, item := range siDecode[siList[siItem]](t, raw).Items {
			if item.ProductID == b.productIDs[1] {
				nailsItem = item
			}
		}
		if nailsItem.ID == 0 {
			t.Fatal("added item not found")
		}
		assertNumber(t, nailsItem.Subtotal, "0.40")

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(sale.ID)+"/items/"+itoa(nailsItem.ID), map[string]any{
			"quantity": 4,
		})
		if status != http.StatusOK {
			t.Fatalf("quantity increase status = %d body = %s", status, raw)
		}
		increased := siDecode[siSale](t, raw)
		assertNumber(t, increased.TotalAmount, "21.80")
		if got := b.stock(t, b.productIDs[1]); got != 6 {
			t.Fatalf("product 2 stock after quantity increase = %d, want 6", got)
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(sale.ID)+"/items/"+itoa(nailsItem.ID), map[string]any{
			"delivered_quantity": 5,
		})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("delivered over quantity status = %d body = %s, want 422", status, raw)
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(sale.ID)+"/items/"+itoa(nailsItem.ID), map[string]any{
			"quantity": 100,
		})
		if status != http.StatusBadRequest || !contains(raw, "Insufficient stock") {
			t.Fatalf("quantity increase over stock status = %d body = %s, want 400", status, raw)
		}
		if got := b.stock(t, b.productIDs[1]); got != 6 {
			t.Fatalf("product 2 stock after rejected increase = %d, want 6", got)
		}

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(sale.ID)+"/items/"+itoa(nailsItem.ID), map[string]any{
			"unit_price": 99.99,
		})
		if status != http.StatusOK {
			t.Fatalf("unit price update status = %d body = %s", status, raw)
		}
		repriced := siDecode[siSale](t, raw)
		assertNumber(t, repriced.TotalAmount, "420.96")
		assertNumber(t, repriced.DebtAmount, "420.96")
		if repriced.PaymentStatus != domain.PaymentStatusPending || repriced.DeliveryStatus != domain.DeliveryStatusPending {
			t.Fatalf("repriced statuses = %q/%q", repriced.PaymentStatus, repriced.DeliveryStatus)
		}

		status, raw = b.request(t, http.MethodGet, "/api/v1/sales/stats", nil)
		if status != http.StatusOK {
			t.Fatalf("stats status = %d body = %s", status, raw)
		}
		stats := siDecode[siStats](t, raw)
		if stats.UnpaidCount != 1 || stats.UndeliveredCount != 1 {
			t.Fatalf("stats counts = %+v, want 1/1", stats)
		}
		assertNumber(t, stats.TotalAmountSum, "420.96")
		assertNumber(t, stats.AmountPaidSum, "0")

		status, raw = b.request(t, http.MethodPut, "/api/v1/sales/"+itoa(sale.ID)+"/items/999999", map[string]any{"quantity": 1})
		if status != http.StatusNotFound || !contains(raw, "Sale item not found") {
			t.Fatalf("update missing item status = %d body = %s, want 404", status, raw)
		}
		status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(sale.ID)+"/items/999999", nil)
		if status != http.StatusNotFound || !contains(raw, "Sale item not found") {
			t.Fatalf("delete missing item status = %d body = %s, want 404", status, raw)
		}

		status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(sale.ID)+"/items/"+itoa(nailsItem.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete item status = %d body = %s", status, raw)
		}
		afterDelete := siDecode[siSale](t, raw)
		assertNumber(t, afterDelete.TotalAmount, "21.00")
		if got := b.stock(t, b.productIDs[1]); got != 10 {
			t.Fatalf("product 2 stock after item delete = %d, want 10", got)
		}

		if status, raw = b.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(sale.ID), nil); status != http.StatusOK {
			t.Fatalf("delete sale status = %d body = %s", status, raw)
		}
		if got := b.stock(t, b.productIDs[0]); got != 10 {
			t.Fatalf("product 1 stock after sale delete = %d, want 10", got)
		}

		status, raw = b.request(t, http.MethodGet, "/api/v1/sales/stats", nil)
		if status != http.StatusOK {
			t.Fatalf("final stats status = %d body = %s", status, raw)
		}
		final := siDecode[siStats](t, raw)
		if final.UnpaidCount != 0 || final.UndeliveredCount != 0 {
			t.Fatalf("final stats counts = %+v, want zeros", final)
		}
		assertNumber(t, final.TotalAmountSum, "0")
		assertNumber(t, final.AmountPaidSum, "0")

		if got := siScalarInt(t, b, "SELECT COUNT(*) FROM price_history"); got != beforeHistory {
			t.Fatalf("price_history rows = %d, want unchanged %d", got, beforeHistory)
		}
	})
}

func siScalarInt(t *testing.T, b *siBackend, query string) int {
	t.Helper()
	var value int
	if err := b.gorm.Raw(query).Row().Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}

func assertRawSaleEnum(t *testing.T, b *siBackend, id int64, payment, delivery, method string) {
	t.Helper()
	var gotPayment, gotDelivery string
	var gotMethod sql.NullString
	if err := b.gorm.Raw(
		"SELECT payment_status, delivery_status, payment_method FROM sales WHERE id = ?", id,
	).Row().Scan(&gotPayment, &gotDelivery, &gotMethod); err != nil {
		t.Fatal(err)
	}
	if gotPayment != payment || gotDelivery != delivery {
		t.Fatalf("stored statuses = %q/%q, want %q/%q", gotPayment, gotDelivery, payment, delivery)
	}
	if method == "" {
		if gotMethod.Valid {
			t.Fatalf("stored payment_method = %q, want NULL", gotMethod.String)
		}
		return
	}
	if !gotMethod.Valid || gotMethod.String != method {
		t.Fatalf("stored payment_method = %v, want %q", gotMethod, method)
	}
}

func assertRawSaleTotal(t *testing.T, b *siBackend, id int64, want string) {
	t.Helper()
	if b.gorm.Dialector.Name() == "sqlite" {
		var cents int64
		if err := b.gorm.Raw("SELECT total_amount FROM sales WHERE id = ?", id).Row().Scan(&cents); err != nil {
			t.Fatal(err)
		}
		if got := decimalCents(cents); got != want {
			t.Fatalf("stored sqlite total = %d cents (%s), want %s", cents, got, want)
		}
		return
	}
	var text string
	if err := b.gorm.Raw("SELECT total_amount::text FROM sales WHERE id = ?", id).Row().Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != want {
		t.Fatalf("stored postgres total = %q, want %s", text, want)
	}
}

func decimalCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return sign + itoa(cents/100) + "." + pad2(cents%100)
}

func pad2(value int64) string {
	if value < 10 {
		return "0" + itoa(value)
	}
	return itoa(value)
}
