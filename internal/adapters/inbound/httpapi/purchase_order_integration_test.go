package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/domain"
)

type poBackend struct {
	name       string
	gorm       *gorm.DB
	server     *httptest.Server
	userID     int64
	providerID int64
	productIDs [4]int64
	token      string
}

type poOrder struct {
	ID            int64           `json:"id"`
	ProviderID    int64           `json:"provider_id"`
	Status        string          `json:"status"`
	PaymentStatus string          `json:"payment_status"`
	OrderDate     string          `json:"order_date"`
	TotalAmount   json.Number     `json:"total_amount"`
	Provider      *map[string]any `json:"provider"`
}

type poItem struct {
	ID           int64       `json:"id"`
	OrderID      int64       `json:"order_id"`
	ProductID    int64       `json:"product_id"`
	Quantity     int         `json:"quantity"`
	UnitCost     json.Number `json:"unit_cost"`
	Subtotal     json.Number `json:"subtotal"`
	SupplierSKU  *string     `json:"supplier_sku"`
	ProductName  string      `json:"product_name"`
	ProductSKU   string      `json:"product_sku"`
	BrandName    *string     `json:"brand_name"`
	MaterialName *string     `json:"material_name"`
	CategoryName *string     `json:"category_name"`
}

type poCatalog struct {
	ID             int64       `json:"id"`
	ProductID      int64       `json:"product_id"`
	ProviderID     int64       `json:"provider_id"`
	SupplierSKU    *string     `json:"supplier_sku"`
	LastQuotedCost json.Number `json:"last_quoted_cost"`
	LeadTime       *int64      `json:"lead_time"`
}

func TestPurchaseOrderEndpointsAgainstBothBackends(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		runPurchaseOrderBackendSuite(t, newPOSQLiteBackend(t))
	})
	t.Run("postgres", func(t *testing.T) {
		runPurchaseOrderBackendSuite(t, newPOPostgresBackend(t))
	})
}

func runPurchaseOrderBackendSuite(t *testing.T, backend *poBackend) {
	t.Helper()

	status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
		"provider_id": backend.providerID,
		"items": []map[string]any{
			{"product_id": backend.productIDs[0], "quantity": 2, "unit_cost": 10.25, "supplier_sku": "PROV-1"},
			{"product_id": backend.productIDs[1], "quantity": 3, "unit_cost": 1.25},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("create status = %d body = %s", status, raw)
	}
	order := siDecode[poOrder](t, raw)
	if order.Status != domain.OrderStatusPending || order.PaymentStatus != domain.PurchasePaymentStatusPending {
		t.Fatalf("statuses = %q/%q", order.Status, order.PaymentStatus)
	}
	if order.Provider == nil || order.ProviderID != backend.providerID {
		t.Fatalf("provider = %+v id %d", order.Provider, order.ProviderID)
	}
	assertNumber(t, order.TotalAmount, "24.25")
	if _, err := time.Parse(time.RFC3339, order.OrderDate); err != nil {
		t.Fatalf("order_date = %q: %v", order.OrderDate, err)
	}
	for _, key := range []string{"id", "provider_id", "status", "payment_status", "order_date", "total_amount", "provider"} {
		if _, ok := siDecode[map[string]json.RawMessage](t, raw)[key]; !ok {
			t.Fatalf("missing order key %q in %s", key, raw)
		}
	}
	if stock := backend.stock(t, backend.productIDs[0]); stock != 10 {
		t.Fatalf("stock after create = %d, want 10", stock)
	}

	t.Run("get list filters and pagination", func(t *testing.T) {
		path := "/api/v1/supply-chain/purchase-orders?provider_id=" + itoa(backend.providerID) +
			"&status=pending&payment_status=pending&limit=1"
		status, raw := backend.request(t, http.MethodGet, path, nil)
		if status != http.StatusOK {
			t.Fatalf("list status = %d body = %s", status, raw)
		}
		list := siDecode[siList[poOrder]](t, raw)
		if len(list.Items) != 1 || list.Items[0].ID != order.ID || list.HasNext {
			t.Fatalf("filtered list = %+v", list)
		}

		status, raw = backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), nil)
		if status != http.StatusOK || siDecode[poOrder](t, raw).ID != order.ID {
			t.Fatalf("get status = %d body = %s", status, raw)
		}
		status, _ = backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders/999999", nil)
		if status != http.StatusNotFound {
			t.Fatalf("missing get status = %d, want 404", status)
		}
	})

	t.Run("items shape and mutations", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items", nil)
		if status != http.StatusOK {
			t.Fatalf("items status = %d body = %s", status, raw)
		}
		items := siDecode[siList[poItem]](t, raw)
		if len(items.Items) != 2 || items.Items[0].ID >= items.Items[1].ID {
			t.Fatalf("items = %+v, want id ASC", items.Items)
		}
		first := items.Items[0]
		if first.ProductName == "" || first.ProductSKU == "" || first.BrandName == nil ||
			first.MaterialName == nil || first.CategoryName == nil {
			t.Fatalf("item relations = %+v", first)
		}
		assertNumber(t, first.UnitCost, "10.25")
		assertNumber(t, first.Subtotal, "20.50")
		if first.SupplierSKU == nil || *first.SupplierSKU != "PROV-1" {
			t.Fatalf("supplier sku = %v", first.SupplierSKU)
		}

		status, raw = backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items", map[string]any{
			"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1,
		})
		if status != http.StatusBadRequest || !contains(raw, "already exists in this order") {
			t.Fatalf("duplicate status = %d body = %s", status, raw)
		}

		status, raw = backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items", map[string]any{
			"product_id": backend.productIDs[2], "quantity": 2, "unit_cost": 1.50,
		})
		if status != http.StatusOK {
			t.Fatalf("add status = %d body = %s", status, raw)
		}
		assertNumber(t, siDecode[poOrder](t, raw).TotalAmount, "27.25")
		if stock := backend.stock(t, backend.productIDs[2]); stock != 10 {
			t.Fatalf("stock after add = %d, want 10", stock)
		}

		status, raw = backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items", nil)
		if status != http.StatusOK {
			t.Fatalf("items status = %d body = %s", status, raw)
		}
		items = siDecode[siList[poItem]](t, raw)
		var added poItem
		for _, item := range items.Items {
			if item.ProductID == backend.productIDs[2] {
				added = item
			}
		}
		if added.ID == 0 {
			t.Fatal("added item not found")
		}

		status, raw = backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items/"+itoa(added.ID), map[string]any{
			"quantity": 1, "unit_cost": 2.00,
		})
		if status != http.StatusOK {
			t.Fatalf("update item status = %d body = %s", status, raw)
		}
		assertNumber(t, siDecode[poOrder](t, raw).TotalAmount, "26.25")

		status, raw = backend.request(t, http.MethodDelete, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items/"+itoa(added.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete item status = %d body = %s", status, raw)
		}
		assertNumber(t, siDecode[poOrder](t, raw).TotalAmount, "24.25")

		status, _ = backend.request(t, http.MethodDelete, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items/"+itoa(added.ID), nil)
		if status != http.StatusNotFound {
			t.Fatalf("missing item delete status = %d, want 404", status)
		}
	})

	t.Run("receiving stock and catalog", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), map[string]any{
			"status": "received",
		})
		if status != http.StatusOK {
			t.Fatalf("receive status = %d body = %s", status, raw)
		}
		if got := siDecode[poOrder](t, raw); got.Status != domain.OrderStatusReceived {
			t.Fatalf("status = %q", got.Status)
		}
		if stock := backend.stock(t, backend.productIDs[0]); stock != 12 {
			t.Fatalf("product 1 stock = %d, want 12", stock)
		}
		if stock := backend.stock(t, backend.productIDs[1]); stock != 13 {
			t.Fatalf("product 2 stock = %d, want 13", stock)
		}

		catalogs := backend.catalogs(t, backend.providerID)
		if len(catalogs) != 2 {
			t.Fatalf("catalogs = %+v, want 2", catalogs)
		}
		var hammer poCatalog
		for _, catalog := range catalogs {
			if catalog.ProductID == backend.productIDs[0] {
				hammer = catalog
			}
		}
		assertNumber(t, hammer.LastQuotedCost, "10.25")
		if hammer.SupplierSKU == nil || *hammer.SupplierSKU != "PROV-1" || hammer.LeadTime == nil {
			t.Fatalf("catalog = %+v", hammer)
		}

		status, raw = backend.request(t, http.MethodGet, "/api/v1/supply-chain/supplier-catalogs?provider_id="+itoa(backend.providerID), nil)
		if status != http.StatusOK {
			t.Fatalf("catalogs endpoint status = %d body = %s", status, raw)
		}
		if got := siDecode[siList[poCatalog]](t, raw); len(got.Items) != 2 {
			t.Fatalf("catalog list = %+v", got)
		}

		status, raw = backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), map[string]any{
			"status": "received",
		})
		if status != http.StatusOK {
			t.Fatalf("second receive status = %d body = %s", status, raw)
		}
		if stock := backend.stock(t, backend.productIDs[0]); stock != 12 {
			t.Fatalf("product 1 stock after second receive = %d, want 12", stock)
		}
		if got := backend.catalogs(t, backend.providerID); len(got) != 2 {
			t.Fatalf("catalogs after second receive = %d, want 2", len(got))
		}

		// Item mutations on a received order keep stock and catalogs unchanged.
		status, raw = backend.request(t, http.MethodPut, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID)+"/items/1", map[string]any{
			"quantity": 5,
		})
		if status != http.StatusOK {
			t.Fatalf("received item update status = %d body = %s", status, raw)
		}
		if stock := backend.stock(t, backend.productIDs[0]); stock != 12 {
			t.Fatalf("product 1 stock after item update = %d, want 12", stock)
		}
	})

	t.Run("delete does not reverse stock", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodDelete, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete status = %d body = %s", status, raw)
		}
		if stock := backend.stock(t, backend.productIDs[0]); stock != 12 {
			t.Fatalf("stock after delete = %d, want 12", stock)
		}
		status, _ = backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders/"+itoa(order.ID), nil)
		if status != http.StatusNotFound {
			t.Fatalf("get deleted status = %d, want 404", status)
		}
	})

	t.Run("create received gap and validation", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"status":      "received",
			"items": []map[string]any{
				{"product_id": backend.productIDs[3], "quantity": 2, "unit_cost": 3},
			},
		})
		if status != http.StatusOK {
			t.Fatalf("create received status = %d body = %s", status, raw)
		}
		gap := siDecode[poOrder](t, raw)
		if gap.Status != domain.OrderStatusReceived {
			t.Fatalf("status = %q", gap.Status)
		}
		if stock := backend.stock(t, backend.productIDs[3]); stock != 10 {
			t.Fatalf("stock after create-received = %d, want 10", stock)
		}
		if got := backend.catalogsForProduct(t, backend.providerID, backend.productIDs[3]); len(got) != 0 {
			t.Fatalf("catalogs after create-received = %+v", got)
		}
		if status, _ := backend.request(t, http.MethodDelete, "/api/v1/supply-chain/purchase-orders/"+itoa(gap.ID), nil); status != http.StatusOK {
			t.Fatalf("delete gap status = %d", status)
		}

		status, raw = backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": 999999,
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusBadRequest || !contains(raw, "Provider with id 999999 not found") {
			t.Fatalf("missing provider status = %d body = %s", status, raw)
		}
		status, raw = backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"items":       []map[string]any{{"product_id": 999999, "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusNotFound || !contains(raw, "Product with id 999999 not found") {
			t.Fatalf("missing product status = %d body = %s", status, raw)
		}
		status, _ = backend.request(t, http.MethodPost, "/api/v1/supply-chain/purchase-orders", map[string]any{
			"provider_id": backend.providerID,
			"status":      "unknown",
			"items":       []map[string]any{{"product_id": backend.productIDs[0], "quantity": 1, "unit_cost": 1}},
		})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("invalid enum status = %d, want 422", status)
		}
		status, _ = backend.request(t, http.MethodGet, "/api/v1/supply-chain/purchase-orders?status=unknown", nil)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("invalid filter status = %d, want 422", status)
		}
	})
}

func newPOSQLiteBackend(t *testing.T) *poBackend {
	t.Helper()
	gormDB, conn := openTestDB(t, "sqlite")
	return newPOBackend(t, "sqlite", gormDB, conn)
}

func newPOPostgresBackend(t *testing.T) *poBackend {
	t.Helper()
	gormDB, conn := openTestDB(t, "postgres")
	return newPOBackend(t, "postgres", gormDB, conn)
}

func newPOBackend(t *testing.T, name string, gormDB *gorm.DB, conn *sql.DB) *poBackend {
	t.Helper()
	ctx := context.Background()
	b := &poBackend{name: name, gorm: gormDB}

	categories := gormstore.NewCategoryStore(gormDB)
	brands := gormstore.NewBrandStore(gormDB)
	materials := gormstore.NewMaterialStore(gormDB)
	category, err := categories.Create(ctx, &domain.Category{Name: name + " order category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := brands.Create(ctx, &domain.Brand{Name: name + " order brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := materials.Create(ctx, &domain.Material{Name: name + " order material"})
	if err != nil {
		t.Fatal(err)
	}
	products := gormstore.NewProductStore(gormDB)
	for i := range b.productIDs {
		product, err := products.Create(ctx, &domain.Product{
			SKU:               name + "-order-sku-" + itoa(int64(i+1)),
			Name:              name + " order product " + itoa(int64(i+1)),
			Price:             decimal.RequireFromString("10"),
			Cost:              decimal.RequireFromString("1"),
			StockQuantity:     10,
			LowStockThreshold: 0,
			BrandID:           brand.ID,
			CategoryID:        category.ID,
			MaterialID:        material.ID,
		})
		if err != nil {
			t.Fatal(err)
		}
		b.productIDs[i] = product.ID
	}

	providers := gormstore.NewProviderStore(gormDB)
	provider, err := providers.Create(ctx, &domain.Provider{Name: name + " order provider"})
	if err != nil {
		t.Fatal(err)
	}
	b.providerID = provider.ID

	users := gormstore.NewUserStore(gormDB)
	hasher := security.NewPasswordHasher(4)
	hash, err := hasher.Hash("purchase-integration-password")
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{
		FullName:       "Purchase Integration User",
		Email:          "purchase-" + name + "@example.com",
		HashedPassword: hash,
		IsActive:       true,
	}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	b.userID = user.ID

	const secret = "purchase-integration-secret"
	issuer := security.NewAccessTokenIssuer(secret, time.Hour, "inventory-desktop/"+name)
	b.token, err = issuer.Issue(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	authService := application.NewAuthService(
		users,
		gormstore.NewRefreshTokenStore(gormDB),
		hasher,
		issuer,
		security.NewRefreshTokenService(),
		time.Hour,
	)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b.server = httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:             authService,
		Users:            application.NewUserService(users, gormstore.NewRefreshTokenStore(gormDB), hasher),
		Products:         application.NewProductService(products),
		Providers:        application.NewProviderService(providers),
		SupplierCatalogs: application.NewSupplierCatalogService(gormstore.NewSupplierCatalogStore(gormDB)),
		PurchaseOrders:   application.NewPurchaseOrderService(gormstore.NewPurchaseOrderStore(gormDB), gormstore.NewUnitOfWork(gormDB)),
		Logger:           logger,
		Backend:          name,
	}))
	t.Cleanup(b.server.Close)
	return b
}

func (b *poBackend) request(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	return doRequest(t, b.server, method, path, body, &b.token)
}

func (b *poBackend) stock(t *testing.T, productID int64) int {
	t.Helper()
	var stock int
	if err := b.gorm.Raw("SELECT stock_quantity FROM products WHERE id = ?", productID).Row().Scan(&stock); err != nil {
		t.Fatal(err)
	}
	return stock
}

func (b *poBackend) catalogs(t *testing.T, providerID int64) []poCatalog {
	t.Helper()
	status, raw := b.request(t, http.MethodGet, "/api/v1/supply-chain/supplier-catalogs?provider_id="+itoa(providerID), nil)
	if status != http.StatusOK {
		t.Fatalf("catalogs status = %d body = %s", status, raw)
	}
	return siDecode[siList[poCatalog]](t, raw).Items
}

func (b *poBackend) catalogsForProduct(t *testing.T, providerID, productID int64) []poCatalog {
	t.Helper()
	var items []poCatalog
	for _, catalog := range b.catalogs(t, providerID) {
		if catalog.ProductID == productID {
			items = append(items, catalog)
		}
	}
	return items
}
