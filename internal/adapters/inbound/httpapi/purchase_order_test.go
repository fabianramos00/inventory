package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type stubPurchaseOrderService struct {
	listResult *inbound.PurchaseOrderList
	listErr    error
	listFilter inbound.PurchaseOrderFilter

	createResult *domain.PurchaseOrder
	createErr    error
	createInput  inbound.CreatePurchaseOrderInput

	getResult *domain.PurchaseOrder
	getErr    error
	getID     int64

	updateResult *domain.PurchaseOrder
	updateErr    error
	updateID     int64
	updateInput  inbound.UpdatePurchaseOrderInput

	deleteResult *domain.PurchaseOrder
	deleteErr    error
	deleteID     int64

	itemsResult *inbound.OrderItemList
	itemsErr    error
	itemsOrder  int64
	itemsSkip   int
	itemsLimit  int

	addResult *domain.PurchaseOrder
	addErr    error
	addOrder  int64
	addInput  inbound.AddOrderItemInput

	updateItemResult *domain.PurchaseOrder
	updateItemErr    error
	updateItemOrder  int64
	updateItemID     int64
	updateItemInput  inbound.UpdateOrderItemInput

	deleteItemResult *domain.PurchaseOrder
	deleteItemErr    error
	deleteItemOrder  int64
	deleteItemID     int64
}

func (s *stubPurchaseOrderService) List(_ context.Context, filter inbound.PurchaseOrderFilter) (*inbound.PurchaseOrderList, error) {
	s.listFilter = filter
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return &inbound.PurchaseOrderList{Skip: filter.Skip, Limit: filter.Limit}, nil
	}
	return s.listResult, nil
}

func (s *stubPurchaseOrderService) Create(_ context.Context, input inbound.CreatePurchaseOrderInput) (*domain.PurchaseOrder, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.createResult == nil {
		return poSampleOrder(1), nil
	}
	return s.createResult, nil
}

func (s *stubPurchaseOrderService) Get(_ context.Context, id int64) (*domain.PurchaseOrder, error) {
	s.getID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getResult == nil {
		return poSampleOrder(id), nil
	}
	return s.getResult, nil
}

func (s *stubPurchaseOrderService) Update(_ context.Context, id int64, input inbound.UpdatePurchaseOrderInput) (*domain.PurchaseOrder, error) {
	s.updateID, s.updateInput = id, input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.updateResult == nil {
		return poSampleOrder(id), nil
	}
	return s.updateResult, nil
}

func (s *stubPurchaseOrderService) Delete(_ context.Context, id int64) (*domain.PurchaseOrder, error) {
	s.deleteID = id
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	if s.deleteResult == nil {
		return poSampleOrder(id), nil
	}
	return s.deleteResult, nil
}

func (s *stubPurchaseOrderService) ListItems(_ context.Context, orderID int64, skip, limit int) (*inbound.OrderItemList, error) {
	s.itemsOrder, s.itemsSkip, s.itemsLimit = orderID, skip, limit
	if s.itemsErr != nil {
		return nil, s.itemsErr
	}
	if s.itemsResult == nil {
		return &inbound.OrderItemList{Skip: skip, Limit: limit}, nil
	}
	return s.itemsResult, nil
}

func (s *stubPurchaseOrderService) AddItem(_ context.Context, orderID int64, input inbound.AddOrderItemInput) (*domain.PurchaseOrder, error) {
	s.addOrder, s.addInput = orderID, input
	if s.addErr != nil {
		return nil, s.addErr
	}
	if s.addResult == nil {
		return poSampleOrder(orderID), nil
	}
	return s.addResult, nil
}

func (s *stubPurchaseOrderService) UpdateItem(_ context.Context, orderID, itemID int64, input inbound.UpdateOrderItemInput) (*domain.PurchaseOrder, error) {
	s.updateItemOrder, s.updateItemID, s.updateItemInput = orderID, itemID, input
	if s.updateItemErr != nil {
		return nil, s.updateItemErr
	}
	if s.updateItemResult == nil {
		return poSampleOrder(orderID), nil
	}
	return s.updateItemResult, nil
}

func (s *stubPurchaseOrderService) DeleteItem(_ context.Context, orderID, itemID int64) (*domain.PurchaseOrder, error) {
	s.deleteItemOrder, s.deleteItemID = orderID, itemID
	if s.deleteItemErr != nil {
		return nil, s.deleteItemErr
	}
	if s.deleteItemResult == nil {
		return poSampleOrder(orderID), nil
	}
	return s.deleteItemResult, nil
}

func poSampleOrder(id int64) *domain.PurchaseOrder {
	return &domain.PurchaseOrder{
		ID:            id,
		ProviderID:    3,
		Status:        domain.OrderStatusSent,
		PaymentStatus: domain.PurchasePaymentStatusPending,
		OrderDate:     time.Date(2026, 10, 24, 15, 4, 5, 0, time.UTC),
		TotalAmount:   decimal.RequireFromString("42.5"),
		CreatedByID:   7,
		Provider: &domain.Provider{
			ID: 3, Name: "Distribuidora Metálica", ContactInfo: strPtr("Juan Pérez"),
			Email: strPtr("ventas@example.com"), Phone: strPtr("3001234567"),
		},
	}
}

func poSampleItem(id, orderID int64) *domain.OrderItem {
	return &domain.OrderItem{
		ID:           id,
		OrderID:      orderID,
		ProductID:    3,
		Quantity:     2,
		UnitCost:     decimal.RequireFromString("10.25"),
		Subtotal:     decimal.RequireFromString("20.5"),
		SupplierSKU:  strPtr("PROV-001"),
		ProductName:  "Martillo",
		ProductSKU:   "SKU-1234ABCD",
		BrandName:    strPtr("Truper"),
		MaterialName: strPtr("Acero"),
		CategoryName: strPtr("Herramientas"),
	}
}

func newPurchaseOrderTestServer(t *testing.T, service *stubPurchaseOrderService) *httptest.Server {
	t.Helper()
	if service == nil {
		service = &stubPurchaseOrderService{}
	}
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"user": {ID: 42, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:           auth,
		Users:          &stubUserService{},
		PurchaseOrders: service,
		Logger:         logger,
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPurchaseOrdersRequireAuth(t *testing.T) {
	server := newPurchaseOrderTestServer(t, nil)
	tests := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/supply-chain/purchase-orders", nil},
		{http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{}},
		{http.MethodGet, "/api/v1/supply-chain/purchase-orders/1", nil},
		{http.MethodPut, "/api/v1/supply-chain/purchase-orders/1", map[string]any{}},
		{http.MethodDelete, "/api/v1/supply-chain/purchase-orders/1", nil},
		{http.MethodGet, "/api/v1/supply-chain/purchase-orders/1/items", nil},
		{http.MethodPost, "/api/v1/supply-chain/purchase-orders/1/items", map[string]any{}},
		{http.MethodPut, "/api/v1/supply-chain/purchase-orders/1/items/2", map[string]any{}},
		{http.MethodDelete, "/api/v1/supply-chain/purchase-orders/1/items/2", nil},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			status, raw := userRequest(t, tt.method, server.URL+tt.path, "", tt.body)
			if status != http.StatusUnauthorized {
				t.Fatalf("status = %d body = %s, want 401", status, raw)
			}
		})
	}
}

func TestPurchaseOrderListFiltersAndShape(t *testing.T) {
	service := &stubPurchaseOrderService{listResult: &inbound.PurchaseOrderList{
		Items:   []*domain.PurchaseOrder{poSampleOrder(5)},
		HasNext: true,
		Skip:    2,
		Limit:   1,
	}}
	server := newPurchaseOrderTestServer(t, service)

	path := "/api/v1/supply-chain/purchase-orders?skip=2&limit=1&provider_id=3" +
		"&status=sent&payment_status=pending" +
		"&min_date=2026-10-01T00:00:00Z&max_date=2026-10-31T23:59:59Z"
	status, raw := userRequest(t, http.MethodGet, server.URL+path, "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	filter := service.listFilter
	if filter.Skip != 2 || filter.Limit != 1 || filter.ProviderID == nil || *filter.ProviderID != 3 ||
		filter.Status != domain.OrderStatusSent || filter.PaymentStatus != domain.PurchasePaymentStatusPending ||
		filter.MinDate == nil || !filter.MinDate.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) ||
		filter.MaxDate == nil || !filter.MaxDate.Equal(time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC)) {
		t.Fatalf("filter = %+v", filter)
	}

	var body struct {
		Items   []map[string]json.RawMessage `json:"items"`
		HasNext bool                         `json:"has_next"`
		Skip    int                          `json:"skip"`
		Limit   int                          `json:"limit"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if !body.HasNext || body.Skip != 2 || body.Limit != 1 || len(body.Items) != 1 {
		t.Fatalf("pagination = %+v", body)
	}
	order := body.Items[0]
	for _, key := range []string{"id", "provider_id", "status", "payment_status", "order_date", "total_amount", "provider"} {
		if _, ok := order[key]; !ok {
			t.Fatalf("missing key %q in %s", key, raw)
		}
	}
	if _, leaked := order["created_by"]; leaked {
		t.Fatal("purchase order leaked created_by")
	}
	var orderDate string
	if err := json.Unmarshal(order["order_date"], &orderDate); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339, orderDate); err != nil {
		t.Fatalf("order_date = %q: %v", orderDate, err)
	}
	if string(order["total_amount"]) != "42.5" {
		t.Fatalf("total_amount = %s, want 42.5", order["total_amount"])
	}
	var provider map[string]json.RawMessage
	if err := json.Unmarshal(order["provider"], &provider); err != nil {
		t.Fatal(err)
	}
	if _, ok := provider["name"]; !ok {
		t.Fatal("provider missing name")
	}
}

func TestPurchaseOrderListRejectsInvalidFilters(t *testing.T) {
	server := newPurchaseOrderTestServer(t, nil)
	paths := []string{
		"/api/v1/supply-chain/purchase-orders?status=unknown",
		"/api/v1/supply-chain/purchase-orders?payment_status=unknown",
		"/api/v1/supply-chain/purchase-orders?min_date=not-a-date",
		"/api/v1/supply-chain/purchase-orders?max_date=2026-13-01T00:00:00Z",
		"/api/v1/supply-chain/purchase-orders?provider_id=0",
		"/api/v1/supply-chain/purchase-orders?limit=201",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+path, "user", nil)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
		})
	}
}

func TestPurchaseOrderCreateUsesTokenAndPayload(t *testing.T) {
	service := &stubPurchaseOrderService{createResult: poSampleOrder(11)}
	server := newPurchaseOrderTestServer(t, service)

	body := map[string]any{
		"provider_id": 3,
		"status":      "sent",
		"items": []map[string]any{
			{"product_id": 3, "quantity": 2, "unit_cost": 10.25, "supplier_sku": "PROV-001"},
		},
	}
	status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/supply-chain/purchase-orders", "user", body)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.createInput.CreatedByID != 42 || service.createInput.ProviderID != 3 || service.createInput.Status != domain.OrderStatusSent {
		t.Fatalf("create input = %+v", service.createInput)
	}
	if len(service.createInput.Items) != 1 {
		t.Fatalf("items = %+v", service.createInput.Items)
	}
	item := service.createInput.Items[0]
	if item.ProductID != 3 || item.Quantity != 2 || !item.UnitCost.Equal(decimal.RequireFromString("10.25")) ||
		item.SupplierSKU == nil || *item.SupplierSKU != "PROV-001" {
		t.Fatalf("item = %+v", item)
	}

	cases := []map[string]any{
		{"items": []map[string]any{{"product_id": 3, "quantity": 1, "unit_cost": 1}}},
		{"provider_id": 3, "items": []map[string]any{{"product_id": 3, "quantity": 1}}},
		{"provider_id": 3, "items": []map[string]any{{"product_id": 3, "quantity": 1, "unit_cost": 1}}, "unknown": true},
	}
	for i, invalid := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/supply-chain/purchase-orders", "user", invalid)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
		})
	}
}

func TestPurchaseOrderUpdateMapsPresenceAwareFields(t *testing.T) {
	service := &stubPurchaseOrderService{updateResult: poSampleOrder(4)}
	server := newPurchaseOrderTestServer(t, service)

	status, raw := userRequest(t, http.MethodPut, server.URL+"/api/v1/supply-chain/purchase-orders/4", "user", map[string]any{
		"provider_id": nil, "status": nil, "payment_status": "paid",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if !service.updateInput.ProviderID.Set || service.updateInput.ProviderID.Value != nil {
		t.Fatalf("provider_id = %+v, want set null", service.updateInput.ProviderID)
	}
	if !service.updateInput.Status.Set || service.updateInput.Status.Value != nil {
		t.Fatalf("status = %+v, want set null", service.updateInput.Status)
	}
	if !service.updateInput.PaymentStatus.Set || service.updateInput.PaymentStatus.Value == nil ||
		*service.updateInput.PaymentStatus.Value != domain.PurchasePaymentStatusPaid {
		t.Fatalf("payment_status = %+v", service.updateInput.PaymentStatus)
	}
}

func TestPurchaseOrderItemEndpoints(t *testing.T) {
	service := &stubPurchaseOrderService{
		itemsResult: &inbound.OrderItemList{
			Items:   []*domain.OrderItem{poSampleItem(1, 5), poSampleItem(2, 5)},
			HasNext: true,
			Skip:    1,
			Limit:   2,
		},
	}
	server := newPurchaseOrderTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/purchase-orders/5/items?skip=1&limit=2", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.itemsOrder != 5 || service.itemsSkip != 1 || service.itemsLimit != 2 {
		t.Fatalf("items query = %d/%d/%d", service.itemsOrder, service.itemsSkip, service.itemsLimit)
	}
	var list struct {
		Items   []map[string]json.RawMessage `json:"items"`
		HasNext bool                         `json:"has_next"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if !list.HasNext || len(list.Items) != 2 {
		t.Fatalf("list = %+v", list)
	}
	for _, key := range []string{
		"id", "order_id", "product_id", "quantity", "unit_cost", "subtotal", "supplier_sku",
		"product_name", "product_sku", "brand_name", "material_name", "category_name",
	} {
		if _, ok := list.Items[0][key]; !ok {
			t.Fatalf("missing item key %q in %s", key, raw)
		}
	}

	status, raw = userRequest(t, http.MethodPost, server.URL+"/api/v1/supply-chain/purchase-orders/5/items", "user", map[string]any{
		"product_id": 3, "quantity": 2, "unit_cost": 10.25, "supplier_sku": "PROV-001",
	})
	if status != http.StatusOK {
		t.Fatalf("add status = %d body = %s", status, raw)
	}
	if service.addOrder != 5 || service.addInput.ProductID != 3 || service.addInput.Quantity != 2 ||
		!service.addInput.UnitCost.Equal(decimal.RequireFromString("10.25")) {
		t.Fatalf("add input = %+v order %d", service.addInput, service.addOrder)
	}

	status, raw = userRequest(t, http.MethodPut, server.URL+"/api/v1/supply-chain/purchase-orders/5/items/2", "user", map[string]any{
		"quantity": 3, "unit_cost": 4.5, "supplier_sku": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("update status = %d body = %s", status, raw)
	}
	if service.updateItemOrder != 5 || service.updateItemID != 2 ||
		!service.updateItemInput.Quantity.Set || *service.updateItemInput.Quantity.Value != 3 ||
		!service.updateItemInput.UnitCost.Set || !service.updateItemInput.UnitCost.Value.Equal(decimal.RequireFromString("4.5")) ||
		!service.updateItemInput.SupplierSKU.Set || service.updateItemInput.SupplierSKU.Value != nil {
		t.Fatalf("update input = %+v", service.updateItemInput)
	}

	status, raw = userRequest(t, http.MethodDelete, server.URL+"/api/v1/supply-chain/purchase-orders/5/items/2", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", status, raw)
	}
	if service.deleteItemOrder != 5 || service.deleteItemID != 2 {
		t.Fatalf("delete ids = %d/%d", service.deleteItemOrder, service.deleteItemID)
	}
}

func TestPurchaseOrderErrorsMapToHTTP(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		detail string
		code   string
	}{
		{"provider missing", &domain.ProviderNotFoundError{ID: 9}, http.StatusBadRequest, "Provider with id 9 not found", ""},
		{"order missing", &domain.NotFoundError{Entity: "Purchase order", ID: 5}, http.StatusNotFound, "Purchase order not found", ""},
		{"item missing", &domain.NotFoundError{Entity: "Order item", ID: 2}, http.StatusNotFound, "Order item not found", ""},
		{"product missing", &domain.NotFoundError{Entity: "Product", ID: 3}, http.StatusNotFound, "Product with id 3 not found", ""},
		{"duplicate", fmt.Errorf("%w: product with id 3 already exists in this order", domain.ErrConflict), http.StatusBadRequest, "product with id 3 already exists in this order", "UNIQUE_VIOLATION"},
		{"validation", fmt.Errorf("%w: quantity must be greater than zero", domain.ErrValidation), http.StatusUnprocessableEntity, "quantity must be greater than zero", "VALIDATION_ERROR"},
		{"internal", errors.New("pq: secret internals"), http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newPurchaseOrderTestServer(t, &stubPurchaseOrderService{getErr: tc.err})
			status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/purchase-orders/5", "user", nil)
			if status != tc.status {
				t.Fatalf("status = %d body = %s, want %d", status, raw, tc.status)
			}
			var body errorResponse
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
			if body.Detail != tc.detail || body.Code != tc.code {
				t.Fatalf("error = %+v, want detail %q code %q", body, tc.detail, tc.code)
			}
			if tc.status == http.StatusInternalServerError && string(raw) == tc.err.Error() {
				t.Fatal("internal error leaked")
			}
		})
	}
}
