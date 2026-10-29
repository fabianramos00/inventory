package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type supplierCatalogJSON struct {
	ID             int64          `json:"id"`
	ProductID      int64          `json:"product_id"`
	ProviderID     int64          `json:"provider_id"`
	SupplierSKU    *string        `json:"supplier_sku"`
	LastQuotedCost json.Number    `json:"last_quoted_cost"`
	LeadTime       *int64         `json:"lead_time"`
	Provider       *providerJSON  `json:"provider"`
	Products       map[string]any `json:"products"`
}

func catalogFixture() *domain.SupplierCatalog {
	size := "16 oz"
	unitID := int64(2)
	leadTime := int64(7)
	return &domain.SupplierCatalog{
		ID:             1,
		ProductID:      3,
		ProviderID:     1,
		SupplierSKU:    providerPtr("PROV-001"),
		LastQuotedCost: decimal.RequireFromString("12.34"),
		LeadTime:       &leadTime,
		Provider: &domain.Provider{
			ID:          1,
			Name:        "Distribuidora Metálica",
			ContactInfo: providerPtr("Juan Pérez"),
			Email:       providerPtr("ventas@example.com"),
			Phone:       providerPtr("3001234567"),
		},
		Product: &domain.Product{
			ID:                3,
			SKU:               "SKU-1234ABCD",
			Name:              "Martillo",
			SizeValue:         &size,
			MeasurementUnitID: &unitID,
			Price:             decimal.RequireFromString("19.90"),
			Cost:              decimal.RequireFromString("10.00"),
			StockQuantity:     4,
			LowStockThreshold: 5,
			BrandID:           1,
			CategoryID:        1,
			MaterialID:        1,
		},
	}
}

func TestSupplierCatalogListShapeFiltersAndPagination(t *testing.T) {
	service := &stubSupplierCatalogService{listResult: &inbound.SupplierCatalogList{
		Items:   []*domain.SupplierCatalog{catalogFixture()},
		HasNext: true,
		Skip:    4,
		Limit:   1,
	}}
	server := newSupplyChainTestServer(t, &stubProviderService{}, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/supplier-catalogs?skip=4&limit=1&provider_id=2&product_id=3", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.listFilter.Skip != 4 || service.listFilter.Limit != 1 ||
		service.listFilter.ProviderID == nil || *service.listFilter.ProviderID != 2 ||
		service.listFilter.ProductID == nil || *service.listFilter.ProductID != 3 {
		t.Fatalf("filter = %+v", service.listFilter)
	}

	var body struct {
		Items   []supplierCatalogJSON `json:"items"`
		HasNext bool                  `json:"has_next"`
		Skip    int                   `json:"skip"`
		Limit   int                   `json:"limit"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || !body.HasNext || body.Skip != 4 || body.Limit != 1 {
		t.Fatalf("body = %s", raw)
	}
	item := body.Items[0]
	if item.ID != 1 || item.ProductID != 3 || item.ProviderID != 1 {
		t.Fatalf("ids = %+v", item)
	}
	if item.SupplierSKU == nil || *item.SupplierSKU != "PROV-001" {
		t.Fatalf("supplier_sku = %v", item.SupplierSKU)
	}
	if item.LastQuotedCost.String() != "12.34" {
		t.Fatalf("last_quoted_cost = %s, want exact 12.34", item.LastQuotedCost)
	}
	if item.LeadTime == nil || *item.LeadTime != 7 {
		t.Fatalf("lead_time = %v", item.LeadTime)
	}
	if item.Provider == nil || item.Provider.Name != "Distribuidora Metálica" || item.Provider.ContactInfo == nil || *item.Provider.ContactInfo != "Juan Pérez" {
		t.Fatalf("provider = %+v", item.Provider)
	}
	if item.Products["id"] != float64(3) || item.Products["sku"] != "SKU-1234ABCD" || item.Products["price"] != 19.9 || item.Products["cost"] != 10.0 {
		t.Fatalf("products = %+v", item.Products)
	}
	for _, key := range []string{"category", "brand", "material", "measurement_unit"} {
		if _, ok := item.Products[key]; ok {
			t.Errorf("products must be plain and not include %q", key)
		}
	}

	for _, query := range []string{"", "provider_id=0", "provider_id=-1", "product_id=0"} {
		t.Run("filter "+query, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/supplier-catalogs?"+query, "user", nil)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s", status, raw)
			}
			if query == "" {
				if service.listFilter.ProviderID != nil || service.listFilter.ProductID != nil {
					t.Fatalf("empty query filter = %+v", service.listFilter)
				}
			} else if service.listFilter.ProviderID != nil || service.listFilter.ProductID != nil {
				t.Fatalf("non-positive filter should be ignored: %+v", service.listFilter)
			}
		})
	}

	service.listResult = nil
	status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/supplier-catalogs", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("defaults status = %d body = %s", status, raw)
	}
	if service.listFilter.Skip != 0 || service.listFilter.Limit != 100 {
		t.Fatalf("default filter = %+v", service.listFilter)
	}

	for _, query := range []string{"provider_id=abc", "product_id=1.5", "skip=-1", "limit=201"} {
		t.Run("invalid "+query, func(t *testing.T) {
			status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/supplier-catalogs?"+query, "user", nil)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d body = %s, want 422", status, raw)
			}
			if body := decodeUserError(t, raw); body.Code != "VALIDATION_ERROR" {
				t.Fatalf("code = %q", body.Code)
			}
		})
	}
}

func TestSupplierCatalogGetAndNotFound(t *testing.T) {
	service := &stubSupplierCatalogService{getResult: catalogFixture()}
	server := newSupplyChainTestServer(t, &stubProviderService{}, service)

	status, raw := userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/supplier-catalogs/1", "user", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	if service.getID != 1 {
		t.Fatalf("get id = %d", service.getID)
	}
	var catalog supplierCatalogJSON
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Provider == nil || catalog.Products == nil || catalog.LastQuotedCost.String() != "12.34" {
		t.Fatalf("catalog = %+v", catalog)
	}

	service.getErr = domain.ErrNotFound
	status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/supplier-catalogs/99", "user", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d body = %s, want 404", status, raw)
	}
	if body := decodeUserError(t, raw); body.Detail != "Supplier catalog not found" || body.Code != "" {
		t.Fatalf("error = %+v", body)
	}

	status, raw = userRequest(t, http.MethodGet, server.URL+"/api/v1/supply-chain/supplier-catalogs/0", "user", nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body = %s, want 422", status, raw)
	}
}
