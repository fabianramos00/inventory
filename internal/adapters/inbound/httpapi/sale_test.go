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

type stubSaleService struct {
	listResult *inbound.SaleList
	listErr    error
	listFilter inbound.SaleFilter

	createResult *domain.Sale
	createErr    error
	createInput  inbound.CreateSaleInput

	statsResult *domain.SaleStats
	statsErr    error

	getResult *domain.Sale
	getErr    error
	getID     int64

	updateResult *domain.Sale
	updateErr    error
	updateID     int64
	updateInput  inbound.UpdateSaleInput

	deleteResult *domain.Sale
	deleteErr    error
	deleteID     int64

	itemsResult *inbound.SaleItemList
	itemsErr    error
	itemsSaleID int64
	itemsSkip   int
	itemsLimit  int

	addResult *domain.Sale
	addErr    error
	addSaleID int64
	addInput  inbound.AddSaleItemInput

	updateItemResult *domain.Sale
	updateItemErr    error
	updateItemSaleID int64
	updateItemID     int64
	updateItemInput  inbound.UpdateSaleItemInput

	deleteItemResult *domain.Sale
	deleteItemErr    error
	deleteItemSaleID int64
	deleteItemID     int64
}

func sampleSale(id int64) *domain.Sale {
	updated := time.Date(2026, 1, 2, 4, 5, 6, 0, time.UTC)
	clientID := int64(9)
	return &domain.Sale{
		ID:             id,
		CreatedAt:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt:      &updated,
		TotalAmount:    decimal.RequireFromString("20.50"),
		AmountPaid:     decimal.RequireFromString("10.25"),
		DebtAmount:     decimal.RequireFromString("10.25"),
		PaymentStatus:  domain.PaymentStatusPartial,
		DeliveryStatus: domain.DeliveryStatusPending,
		PaymentMethod:  strPtr(domain.PaymentMethodCash),
		ClientID:       &clientID,
		CreatedByID:    7,
		CreatedBy: &domain.User{
			ID: 7, FullName: "Seller", Email: "seller@example.com", HashedPassword: "secret", IsActive: true,
		},
		Client: &domain.Client{ID: 9, Name: "Ana", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
}

func strPtr(value string) *string {
	return &value
}

func sampleItem(id, saleID int64) *domain.SaleItem {
	return &domain.SaleItem{
		ID:                id,
		SaleID:            saleID,
		ProductID:         3,
		Quantity:          2,
		DeliveredQuantity: 1,
		UnitPrice:         decimal.RequireFromString("10.25"),
		Subtotal:          decimal.RequireFromString("20.50"),
		ProductName:       "Hammer",
		ProductSKU:        "SKU-3",
		BrandName:         strPtr("Brand"),
		MaterialName:      strPtr("Steel"),
		CategoryName:      strPtr("Tools"),
	}
}

func (s *stubSaleService) List(_ context.Context, filter inbound.SaleFilter) (*inbound.SaleList, error) {
	s.listFilter = filter
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult == nil {
		return &inbound.SaleList{Skip: filter.Skip, Limit: filter.Limit}, nil
	}
	return s.listResult, nil
}

func (s *stubSaleService) Create(_ context.Context, input inbound.CreateSaleInput) (*domain.Sale, error) {
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.createResult == nil {
		return sampleSale(1), nil
	}
	return s.createResult, nil
}

func (s *stubSaleService) Stats(context.Context) (*domain.SaleStats, error) {
	if s.statsErr != nil {
		return nil, s.statsErr
	}
	if s.statsResult == nil {
		return &domain.SaleStats{TotalAmountSum: decimal.Zero, AmountPaidSum: decimal.Zero}, nil
	}
	return s.statsResult, nil
}

func (s *stubSaleService) Get(_ context.Context, id int64) (*domain.Sale, error) {
	s.getID = id
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getResult == nil {
		return sampleSale(id), nil
	}
	return s.getResult, nil
}

func (s *stubSaleService) Update(_ context.Context, id int64, input inbound.UpdateSaleInput) (*domain.Sale, error) {
	s.updateID, s.updateInput = id, input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.updateResult == nil {
		return sampleSale(id), nil
	}
	return s.updateResult, nil
}

func (s *stubSaleService) Delete(_ context.Context, id int64) (*domain.Sale, error) {
	s.deleteID = id
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}
	if s.deleteResult == nil {
		return sampleSale(id), nil
	}
	return s.deleteResult, nil
}

func (s *stubSaleService) ListItems(_ context.Context, saleID int64, skip, limit int) (*inbound.SaleItemList, error) {
	s.itemsSaleID, s.itemsSkip, s.itemsLimit = saleID, skip, limit
	if s.itemsErr != nil {
		return nil, s.itemsErr
	}
	if s.itemsResult == nil {
		return &inbound.SaleItemList{Skip: skip, Limit: limit}, nil
	}
	return s.itemsResult, nil
}

func (s *stubSaleService) AddItem(_ context.Context, saleID int64, input inbound.AddSaleItemInput) (*domain.Sale, error) {
	s.addSaleID, s.addInput = saleID, input
	if s.addErr != nil {
		return nil, s.addErr
	}
	if s.addResult == nil {
		return sampleSale(saleID), nil
	}
	return s.addResult, nil
}

func (s *stubSaleService) UpdateItem(_ context.Context, saleID, itemID int64, input inbound.UpdateSaleItemInput) (*domain.Sale, error) {
	s.updateItemSaleID, s.updateItemID, s.updateItemInput = saleID, itemID, input
	if s.updateItemErr != nil {
		return nil, s.updateItemErr
	}
	if s.updateItemResult == nil {
		return sampleSale(saleID), nil
	}
	return s.updateItemResult, nil
}

func (s *stubSaleService) DeleteItem(_ context.Context, saleID, itemID int64) (*domain.Sale, error) {
	s.deleteItemSaleID, s.deleteItemID = saleID, itemID
	if s.deleteItemErr != nil {
		return nil, s.deleteItemErr
	}
	if s.deleteItemResult == nil {
		return sampleSale(saleID), nil
	}
	return s.deleteItemResult, nil
}

func newSaleTestServer(t *testing.T, service *stubSaleService) *httptest.Server {
	t.Helper()
	if service == nil {
		service = &stubSaleService{}
	}
	auth := &stubAuthService{tokens: map[string]*domain.User{
		"user": {ID: 42, FullName: "Regular User", Email: "regular@example.com", IsActive: true},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:    auth,
		Users:   &stubUserService{},
		Clients: &stubClientService{},
		Sales:   service,
		Logger:  logger,
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSalesRequireAuth(t *testing.T) {
	server := newSaleTestServer(t, nil)
	tests := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/sales", nil},
		{http.MethodPost, "/api/v1/sales", map[string]any{}},
		{http.MethodGet, "/api/v1/sales/stats", nil},
		{http.MethodGet, "/api/v1/sales/1", nil},
		{http.MethodPut, "/api/v1/sales/1", map[string]any{}},
		{http.MethodDelete, "/api/v1/sales/1", nil},
		{http.MethodGet, "/api/v1/sales/1/items", nil},
		{http.MethodPost, "/api/v1/sales/1/items", map[string]any{}},
		{http.MethodPut, "/api/v1/sales/1/items/2", map[string]any{}},
		{http.MethodDelete, "/api/v1/sales/1/items/2", nil},
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

func TestSaleListFiltersShapeAndDebt(t *testing.T) {
	service := &stubSaleService{listResult: &inbound.SaleList{
		Items:   []*domain.Sale{sampleSale(5)},
		HasNext: true,
		Skip:    2,
		Limit:   1,
	}}
	server := newSaleTestServer(t, service)

	path := "/api/v1/sales?skip=2&limit=1&user_id=7&client_id=9&no_client=true" +
		"&start_date=2026-01-02T03:04:05Z&end_date=2026-01-03T03:04:05Z" +
		"&payment_status=partial&delivery_status=pending&payment_method=cash"
	status, raw := userRequest(t, http.MethodGet, server.URL+path, "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.listFilter.Skip != 2 || service.listFilter.Limit != 1 ||
		service.listFilter.UserID == nil || *service.listFilter.UserID != 7 ||
		service.listFilter.ClientID == nil || *service.listFilter.ClientID != 9 ||
		!service.listFilter.NoClient ||
		service.listFilter.StartDate == nil || !service.listFilter.StartDate.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) ||
		service.listFilter.EndDate == nil || !service.listFilter.EndDate.Equal(time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC)) ||
		service.listFilter.PaymentStatus != domain.PaymentStatusPartial ||
		service.listFilter.DeliveryStatus != domain.DeliveryStatusPending ||
		service.listFilter.PaymentMethod != domain.PaymentMethodCash {
		t.Fatalf("filter = %+v", service.listFilter)
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
	item := body.Items[0]
	for _, key := range []string{
		"id", "total_amount", "debt_amount", "payment_status", "delivery_status",
		"payment_method", "amount_paid", "client_id", "created_at", "updated_at",
		"created_by_id", "created_by", "client",
	} {
		if _, ok := item[key]; !ok {
			t.Fatalf("missing key %q in %s", key, raw)
		}
	}
	var debt decimal.Decimal
	if err := json.Unmarshal(item["debt_amount"], &debt); err != nil {
		t.Fatal(err)
	}
	if !debt.Equal(decimal.RequireFromString("10.25")) {
		t.Fatalf("debt = %s, want 10.25", debt)
	}
	var createdBy map[string]json.RawMessage
	if err := json.Unmarshal(item["created_by"], &createdBy); err != nil {
		t.Fatal(err)
	}
	if _, leaked := createdBy["hashed_password"]; leaked {
		t.Fatal("created_by leaked hashed_password")
	}
	if _, ok := createdBy["email"]; !ok {
		t.Fatal("created_by missing email")
	}
	var client map[string]json.RawMessage
	if err := json.Unmarshal(item["client"], &client); err != nil {
		t.Fatal(err)
	}
	if _, ok := client["name"]; !ok {
		t.Fatal("client missing name")
	}
}

func TestSaleListRejectsInvalidFilters(t *testing.T) {
	server := newSaleTestServer(t, nil)
	paths := []string{
		"/api/v1/sales?start_date=not-a-date",
		"/api/v1/sales?end_date=2026-13-01T00:00:00Z",
		"/api/v1/sales?payment_status=unknown",
		"/api/v1/sales?delivery_status=unknown",
		"/api/v1/sales?payment_method=unknown",
		"/api/v1/sales?no_client=maybe",
		"/api/v1/sales?user_id=0",
		"/api/v1/sales?client_id=-1",
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

func TestSaleCreateUsesTokenUserAndItemDefaults(t *testing.T) {
	service := &stubSaleService{createResult: sampleSale(11)}
	server := newSaleTestServer(t, service)

	body := map[string]any{
		"payment_method": "cash",
		"amount_paid":    5.5,
		"client_id":      9,
		"created_by_id":  999,
		"items": []map[string]any{
			{"product_id": 3, "quantity": 2, "delivered_quantity": 1},
		},
	}
	status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/sales", "user", body)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.createInput.CreatedByID != 42 {
		t.Fatalf("created_by_id = %d, want token user 42", service.createInput.CreatedByID)
	}
	if !service.createInput.AmountPaid.Equal(decimal.RequireFromString("5.5")) {
		t.Fatalf("amount_paid = %s", service.createInput.AmountPaid)
	}
	if len(service.createInput.Items) != 1 || service.createInput.Items[0].DeliveredQuantity != 1 || service.createInput.Items[0].UnitPrice != nil {
		t.Fatalf("items = %+v", service.createInput.Items)
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	var createdByID int64
	if err := json.Unmarshal(response["created_by_id"], &createdByID); err != nil {
		t.Fatal(err)
	}
	if createdByID != 7 {
		t.Fatalf("response created_by_id = %d, want 7", createdByID)
	}
}

func TestSaleCreateRejectsExplicitNullsAndUnknownFields(t *testing.T) {
	server := newSaleTestServer(t, nil)
	cases := []map[string]any{
		{"amount_paid": nil, "items": []map[string]any{{"product_id": 1, "quantity": 1}}},
		{"items": []map[string]any{{"product_id": 1, "quantity": 1, "delivered_quantity": nil}}},
		{"items": []map[string]any{{"product_id": 1, "quantity": 1}}, "unknown": true},
	}
	for i, body := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/sales", "user", body)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
		})
	}
}

func TestSaleCreateAcceptsTheFrontendPayloadShape(t *testing.T) {
	service := &stubSaleService{createResult: sampleSale(12)}
	server := newSaleTestServer(t, service)

	body := map[string]any{
		"payment_method": nil,
		"amount_paid":    0,
		"client_id":      nil,
		"items": []map[string]any{
			{"product_id": 3, "quantity": 2, "delivered_quantity": 0, "unit_price": nil},
		},
	}
	status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/sales", "user", body)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.createInput.PaymentMethod != nil || service.createInput.ClientID != nil {
		t.Fatalf("nullable fields = %+v/%+v, want nil", service.createInput.PaymentMethod, service.createInput.ClientID)
	}
	if !service.createInput.AmountPaid.IsZero() {
		t.Fatalf("amount_paid = %s, want 0", service.createInput.AmountPaid)
	}
	if len(service.createInput.Items) != 1 {
		t.Fatalf("items = %+v", service.createInput.Items)
	}
	item := service.createInput.Items[0]
	if item.ProductID != 3 || item.Quantity != 2 || item.DeliveredQuantity != 0 || item.UnitPrice != nil {
		t.Fatalf("item = %+v, want unit_price nil default", item)
	}
}

func TestSaleItemPayloadsMatchTheFrontendShape(t *testing.T) {
	service := &stubSaleService{
		addResult:        sampleSale(5),
		updateItemResult: sampleSale(5),
	}
	server := newSaleTestServer(t, service)

	status, raw := userRequest(t, http.MethodPost, server.URL+"/api/v1/sales/5/items", "user", map[string]any{
		"product_id": 3, "quantity": 2, "delivered_quantity": 0, "unit_price": nil,
	})
	if status != http.StatusOK {
		t.Fatalf("add status = %d body = %s", status, raw)
	}
	if service.addInput.ProductID != 3 || service.addInput.Quantity != 2 ||
		service.addInput.DeliveredQuantity != 0 || service.addInput.UnitPrice != nil {
		t.Fatalf("add input = %+v, want unit_price nil default", service.addInput)
	}

	status, raw = userRequest(t, http.MethodPut, server.URL+"/api/v1/sales/5/items/2", "user", map[string]any{
		"quantity": 3, "delivered_quantity": 1, "unit_price": 12.5,
	})
	if status != http.StatusOK {
		t.Fatalf("update status = %d body = %s", status, raw)
	}
	if !service.updateItemInput.Quantity.Set || *service.updateItemInput.Quantity.Value != 3 ||
		!service.updateItemInput.DeliveredQuantity.Set || *service.updateItemInput.DeliveredQuantity.Value != 1 ||
		!service.updateItemInput.UnitPrice.Set || !service.updateItemInput.UnitPrice.Value.Equal(decimal.RequireFromString("12.5")) {
		t.Fatalf("update input = %+v", service.updateItemInput)
	}
}

func TestSaleUpdateMapsPresenceAwareFields(t *testing.T) {
	service := &stubSaleService{updateResult: sampleSale(4)}
	server := newSaleTestServer(t, service)

	body := map[string]any{"payment_method": nil, "client_id": nil}
	status, raw := userRequest(t, http.MethodPut, server.URL+"/api/v1/sales/4", "user", body)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if !service.updateInput.PaymentMethod.Set || service.updateInput.PaymentMethod.Value != nil {
		t.Fatalf("payment_method = %+v, want set null", service.updateInput.PaymentMethod)
	}
	if !service.updateInput.ClientID.Set || service.updateInput.ClientID.Value != nil {
		t.Fatalf("client_id = %+v, want set null", service.updateInput.ClientID)
	}
	if service.updateInput.AmountPaid.Set {
		t.Fatal("amount_paid should be omitted")
	}
}

func TestSaleErrorsMapToHTTP(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		detail string
		code   string
	}{
		{"sale not found", &domain.NotFoundError{Entity: "Sale", ID: 5}, http.StatusNotFound, "Sale not found", ""},
		{"product not found", &domain.NotFoundError{Entity: "Product", ID: 5}, http.StatusNotFound, "Product with id 5 not found", ""},
		{"insufficient stock", &domain.InsufficientStockError{SKU: "SKU-1", Available: 1, Requested: 2}, http.StatusBadRequest, "Insufficient stock for product SKU-1. Available: 1, Requested: 2", "INSUFFICIENT_STOCK"},
		{"duplicate", fmt.Errorf("%w: duplicate", domain.ErrConflict), http.StatusBadRequest, "Product already exists in this sale.", "UNIQUE_VIOLATION"},
		{"foreign key", domain.ErrForeignKeyViolation, http.StatusBadRequest, "A referenced record does not exist.", "FOREIGN_KEY_VIOLATION"},
		{"validation", fmt.Errorf("%w: quantity must be greater than zero", domain.ErrValidation), http.StatusUnprocessableEntity, "quantity must be greater than zero", "VALIDATION_ERROR"},
		{"internal", errors.New("pq: secret internals"), http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newSaleTestServer(t, &stubSaleService{getErr: tc.err})
			status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/sales/5", "user", nil)
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
			if tc.status == http.StatusInternalServerError && len(raw) > 0 && string(raw) == tc.err.Error() {
				t.Fatal("internal error leaked")
			}
		})
	}
}

func TestSaleStatsShape(t *testing.T) {
	service := &stubSaleService{statsResult: &domain.SaleStats{
		UnpaidCount:      2,
		UndeliveredCount: 3,
		TotalAmountSum:   decimal.RequireFromString("30.50"),
		AmountPaidSum:    decimal.RequireFromString("10.25"),
	}}
	server := newSaleTestServer(t, service)
	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/sales/stats", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	var body struct {
		UnpaidCount      int64           `json:"unpaid_count"`
		UndeliveredCount int64           `json:"undelivered_count"`
		TotalAmountSum   json.RawMessage `json:"total_amount_sum"`
		AmountPaidSum    json.RawMessage `json:"amount_paid_sum"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.UnpaidCount != 2 || body.UndeliveredCount != 3 {
		t.Fatalf("counts = %+v", body)
	}
	if string(body.TotalAmountSum) != "30.5" || string(body.AmountPaidSum) != "10.25" {
		t.Fatalf("sums = %s/%s", body.TotalAmountSum, body.AmountPaidSum)
	}
}

func TestSaleItemEndpoints(t *testing.T) {
	service := &stubSaleService{
		itemsResult: &inbound.SaleItemList{
			Items:   []*domain.SaleItem{sampleItem(2, 5), sampleItem(3, 5)},
			HasNext: true,
			Skip:    1,
			Limit:   2,
		},
	}
	server := newSaleTestServer(t, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/sales/5/items?skip=1&limit=2", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.itemsSaleID != 5 || service.itemsSkip != 1 || service.itemsLimit != 2 {
		t.Fatalf("items query = %d/%d/%d", service.itemsSaleID, service.itemsSkip, service.itemsLimit)
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
		"id", "sale_id", "product_id", "quantity", "delivered_quantity", "unit_price",
		"subtotal", "product_name", "product_sku", "brand_name", "material_name", "category_name",
	} {
		if _, ok := list.Items[0][key]; !ok {
			t.Fatalf("missing item key %q in %s", key, raw)
		}
	}

	status, raw = userRequest(t, http.MethodPost, server.URL+"/api/v1/sales/5/items", "user", map[string]any{
		"product_id": 3, "quantity": 2, "delivered_quantity": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("add status = %d body = %s", status, raw)
	}
	if service.addSaleID != 5 || service.addInput.ProductID != 3 || service.addInput.Quantity != 2 || service.addInput.DeliveredQuantity != 1 {
		t.Fatalf("add input = %+v sale %d", service.addInput, service.addSaleID)
	}

	status, raw = userRequest(t, http.MethodPut, server.URL+"/api/v1/sales/5/items/2", "user", map[string]any{
		"quantity": 3,
	})
	if status != http.StatusOK {
		t.Fatalf("update status = %d body = %s", status, raw)
	}
	if service.updateItemSaleID != 5 || service.updateItemID != 2 || !service.updateItemInput.Quantity.Set || *service.updateItemInput.Quantity.Value != 3 {
		t.Fatalf("update input = %+v", service.updateItemInput)
	}

	status, raw = userRequest(t, http.MethodDelete, server.URL+"/api/v1/sales/5/items/2", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", status, raw)
	}
	if service.deleteItemSaleID != 5 || service.deleteItemID != 2 {
		t.Fatalf("delete ids = %d/%d", service.deleteItemSaleID, service.deleteItemID)
	}
}

func TestSaleItemNotFoundAndStockErrors(t *testing.T) {
	server := newSaleTestServer(t, &stubSaleService{
		deleteItemErr: &domain.NotFoundError{Entity: "Sale item", ID: 2},
	})
	status, raw := userRequest(t, http.MethodDelete, server.URL+"/api/v1/sales/5/items/2", "user", nil)
	if status != http.StatusNotFound || !contains(raw, "Sale item not found") {
		t.Fatalf("status = %d body = %s", status, raw)
	}

	server = newSaleTestServer(t, &stubSaleService{
		addErr: &domain.InsufficientStockError{SKU: "SKU-3", Available: 0, Requested: 1},
	})
	status, raw = userRequest(t, http.MethodPost, server.URL+"/api/v1/sales/5/items", "user", map[string]any{
		"product_id": 3, "quantity": 1,
	})
	if status != http.StatusBadRequest || !contains(raw, "Insufficient stock for product SKU-3. Available: 0, Requested: 1") {
		t.Fatalf("status = %d body = %s", status, raw)
	}
}

func contains(raw []byte, value string) bool {
	return json.Valid(raw) && stringContains(string(raw), value)
}

func stringContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
