package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"inventory-desktop/internal/adapters/inbound/httpapi"
	"inventory-desktop/internal/adapters/outbound/persistence/gormstore"
	"inventory-desktop/internal/adapters/outbound/security"
	"inventory-desktop/internal/application"
	"inventory-desktop/internal/db"
	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
	"inventory-desktop/tests/testdb"
)

// This file is an independent, HTTP-level verification of the products module.
// It runs the real router, service, and GORM repositories against both SQLite
// and PostgreSQL instead of reusing the implementation-focused unit/contract
// fixtures.

type piFixture struct {
	CategoryID  int64
	BrandID     int64
	MaterialID  int64
	UnitID      int64
	Category2ID int64
	Brand2ID    int64
	Material2ID int64
	Unit2ID     int64
}

type piBackend struct {
	name   string
	gorm   *gorm.DB
	conn   *sql.DB
	server *httptest.Server
	fix    piFixture

	users      outbound.UserRepository
	tokens     outbound.RefreshTokenRepository
	categories outbound.CatalogRepository[domain.Category]
	brands     outbound.CatalogRepository[domain.Brand]
	materials  outbound.CatalogRepository[domain.Material]
	units      outbound.CatalogRepository[domain.MeasurementUnit]
	products   outbound.ProductRepository

	validToken   string
	expiredToken string
	userID       int64
}

type piProduct struct {
	ID                int64       `json:"id"`
	SKU               string      `json:"sku"`
	Name              string      `json:"name"`
	SizeValue         *string     `json:"size_value"`
	MeasurementUnitID *int64      `json:"measurement_unit_id"`
	Price             json.Number `json:"price"`
	Cost              json.Number `json:"cost"`
	StockQuantity     int         `json:"stock_quantity"`
	LowStockThreshold int         `json:"low_stock_threshold"`
	BrandID           int64       `json:"brand_id"`
	CategoryID        int64       `json:"category_id"`
	MaterialID        int64       `json:"material_id"`
}

type piCategory struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Code        *string `json:"code"`
	Description *string `json:"description"`
}

type piMaterial struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type piBrand struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	LogoURL *string `json:"logo_url"`
}

type piUnit struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
}

type piProductDetail struct {
	piProduct
	Category        *piCategory `json:"category"`
	Material        *piMaterial `json:"material"`
	Brand           *piBrand    `json:"brand"`
	MeasurementUnit *piUnit     `json:"measurement_unit"`
}

type piList struct {
	Items   []piProduct `json:"items"`
	HasNext bool        `json:"has_next"`
	Skip    int         `json:"skip"`
	Limit   int         `json:"limit"`
}

type piStats struct {
	TotalProducts       int64       `json:"total_products"`
	TotalStock          int64       `json:"total_stock"`
	LowStockCount       int64       `json:"low_stock_count"`
	TotalInventoryValue json.Number `json:"total_inventory_value"`
}

type piError struct {
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func TestProductEndpointsAgainstBothBackends(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		backend := newPISQLiteBackend(t)
		runProductBackendSuite(t, backend)
	})
	t.Run("postgres", func(t *testing.T) {
		backend := newPIPostgresBackend(t)
		runProductBackendSuite(t, backend)
	})
}

func runProductBackendSuite(t *testing.T, backend *piBackend) {
	t.Helper()

	t.Run("auth on all six routes", func(t *testing.T) { testPIAuth(t, backend) })
	t.Run("empty stats", func(t *testing.T) { testPIEmptyStats(t, backend) })
	t.Run("create defaults and money", func(t *testing.T) { testPICreateDefaults(t, backend) })
	t.Run("create validation conflicts and foreign keys", func(t *testing.T) { testPICreateValidation(t, backend) })
	t.Run("get relations and errors", func(t *testing.T) { testPIGet(t, backend) })
	t.Run("list search filters stock status and pagination", func(t *testing.T) { testPIList(t, backend) })
	t.Run("update every field and null semantics", func(t *testing.T) { testPIUpdate(t, backend) })
	t.Run("delete and foreign key conflicts", func(t *testing.T) { testPIDelete(t, backend) })
	t.Run("stats exact value and low stock boundary", func(t *testing.T) { testPIStats(t, backend) })
	t.Run("price history is never written", func(t *testing.T) { testPIPriceHistory(t, backend) })
	t.Run("unknown JSON fields", func(t *testing.T) { testPIUnknownFields(t, backend) })
}

func newPISQLiteBackend(t *testing.T) *piBackend {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inventory.sqlite")
	gormDB, conn, err := db.OpenSQLiteGORM(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.MigrateSQLite(ctx, conn); err != nil {
		t.Fatal(err)
	}
	return newPIBackend(t, "sqlite", gormDB, conn)
}

func newPIPostgresBackend(t *testing.T) *piBackend {
	t.Helper()
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the postgres product integration suite")
	}
	ctx := context.Background()
	release, err := testdb.Lock(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	gormDB, conn, err := db.OpenPostgresGORM(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.MigratePostgres(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `TRUNCATE
		refresh_tokens, users, categories, materials, brands, measurement_units,
		products, price_history, clients, providers, supplier_catalogs,
		purchase_orders, order_items, sales, sale_items
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return newPIBackend(t, "postgres", gormDB, conn)
}

func newPIBackend(t *testing.T, name string, gormDB *gorm.DB, conn *sql.DB) *piBackend {
	t.Helper()
	ctx := context.Background()
	b := &piBackend{
		name:       name,
		gorm:       gormDB,
		conn:       conn,
		users:      gormstore.NewUserStore(gormDB),
		tokens:     gormstore.NewRefreshTokenStore(gormDB),
		categories: gormstore.NewCategoryStore(gormDB),
		brands:     gormstore.NewBrandStore(gormDB),
		materials:  gormstore.NewMaterialStore(gormDB),
		units:      gormstore.NewMeasurementUnitStore(gormDB),
		products:   gormstore.NewProductStore(gormDB),
	}

	createCatalog := func() (int64, int64, int64, int64) {
		t.Helper()
		category, err := b.categories.Create(ctx, &domain.Category{Name: name + " category"})
		if err != nil {
			t.Fatal(err)
		}
		brand, err := b.brands.Create(ctx, &domain.Brand{Name: name + " brand"})
		if err != nil {
			t.Fatal(err)
		}
		material, err := b.materials.Create(ctx, &domain.Material{Name: name + " material"})
		if err != nil {
			t.Fatal(err)
		}
		unit, err := b.units.Create(ctx, &domain.MeasurementUnit{Name: name + " unit", Abbreviation: "u1"})
		if err != nil {
			t.Fatal(err)
		}
		return category.ID, brand.ID, material.ID, unit.ID
	}
	b.fix.CategoryID, b.fix.BrandID, b.fix.MaterialID, b.fix.UnitID = createCatalog()

	secondCategory, err := b.categories.Create(ctx, &domain.Category{Name: name + " category two"})
	if err != nil {
		t.Fatal(err)
	}
	secondBrand, err := b.brands.Create(ctx, &domain.Brand{Name: name + " brand two"})
	if err != nil {
		t.Fatal(err)
	}
	secondMaterial, err := b.materials.Create(ctx, &domain.Material{Name: name + " material two"})
	if err != nil {
		t.Fatal(err)
	}
	secondUnit, err := b.units.Create(ctx, &domain.MeasurementUnit{Name: name + " unit two", Abbreviation: "u2"})
	if err != nil {
		t.Fatal(err)
	}
	b.fix.Category2ID = secondCategory.ID
	b.fix.Brand2ID = secondBrand.ID
	b.fix.Material2ID = secondMaterial.ID
	b.fix.Unit2ID = secondUnit.ID

	hasher := security.NewPasswordHasher(4)
	hash, err := hasher.Hash("product-test-password")
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{
		FullName:       "Product Tester",
		Email:          name + "-products@example.com",
		HashedPassword: hash,
		IsActive:       true,
		IsSuperuser:    false,
	}
	if err := b.users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	b.userID = user.ID

	const secret = "product-integration-secret"
	issuerName := "inventory-desktop/" + name
	issuer := security.NewAccessTokenIssuer(secret, time.Hour, issuerName)
	b.validToken, err = issuer.Issue(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	expired := security.NewAccessTokenIssuer(secret, -time.Minute, issuerName)
	b.expiredToken, err = expired.Issue(user.ID)
	if err != nil {
		t.Fatal(err)
	}

	authService := application.NewAuthService(
		b.users,
		b.tokens,
		hasher,
		issuer,
		security.NewRefreshTokenService(),
		time.Hour,
	)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b.server = httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Auth:             authService,
		Users:            application.NewUserService(b.users, hasher),
		Categories:       application.NewCategoryService(b.categories),
		Brands:           application.NewBrandService(b.brands),
		Materials:        application.NewMaterialService(b.materials),
		MeasurementUnits: application.NewMeasurementUnitService(b.units),
		Products:         application.NewProductService(b.products),
		Logger:           logger,
		Backend:          name,
	}))
	t.Cleanup(b.server.Close)
	return b
}

func (b *piBackend) request(t *testing.T, method, path string, body any, token *string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(raw))
	}
	request, err := http.NewRequest(method, b.server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != nil {
		request.Header.Set("Authorization", "Bearer "+*token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, raw
}

func (b *piBackend) authRequest(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	return b.request(t, method, path, body, &b.validToken)
}

func piStringPtr(value string) *string { return &value }

func piNumber(value string) json.Number { return json.Number(value) }

func (b *piBackend) baseBody(name, sku string) map[string]any {
	return map[string]any{
		"name":        name,
		"sku":         sku,
		"price":       piNumber("10"),
		"cost":        piNumber("5"),
		"brand_id":    b.fix.BrandID,
		"category_id": b.fix.CategoryID,
		"material_id": b.fix.MaterialID,
	}
}

func (b *piBackend) createProduct(t *testing.T, body map[string]any) (piProduct, map[string]json.RawMessage) {
	t.Helper()
	status, raw := b.authRequest(t, http.MethodPost, "/api/v1/inventory/products", body)
	if status != http.StatusOK {
		t.Fatalf("create status = %d body = %s", status, raw)
	}
	return piDecodeProduct(t, raw)
}

func piDecodeProduct(t *testing.T, raw []byte) (piProduct, map[string]json.RawMessage) {
	t.Helper()
	var product piProduct
	if err := json.Unmarshal(raw, &product); err != nil {
		t.Fatalf("decode product: %v body = %s", err, raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode product fields: %v body = %s", err, raw)
	}
	return product, fields
}

func piDecodeDetail(t *testing.T, raw []byte) (piProductDetail, map[string]json.RawMessage) {
	t.Helper()
	var detail piProductDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("decode product detail: %v body = %s", err, raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode product detail fields: %v body = %s", err, raw)
	}
	return detail, fields
}

func piDecodeList(t *testing.T, raw []byte) piList {
	t.Helper()
	var list piList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode list: %v body = %s", err, raw)
	}
	return list
}

func piDecodeStats(t *testing.T, raw []byte) (piStats, map[string]json.RawMessage) {
	t.Helper()
	var stats piStats
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatalf("decode stats: %v body = %s", err, raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode stats fields: %v body = %s", err, raw)
	}
	return stats, fields
}

func piAssertError(t *testing.T, status int, raw []byte, wantStatus int, wantCode string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("status = %d body = %s, want %d", status, raw, wantStatus)
	}
	var body piError
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode error: %v body = %s", err, raw)
	}
	if body.Code != wantCode {
		t.Fatalf("code = %q body = %s, want %q", body.Code, raw, wantCode)
	}
}

func piAssertMoney(t *testing.T, got json.Number, want string) {
	t.Helper()
	value, err := decimal.NewFromString(got.String())
	if err != nil {
		t.Fatalf("money %q is not a decimal: %v", got, err)
	}
	if !value.Equal(decimal.RequireFromString(want)) {
		t.Fatalf("money = %s, want %s", value, want)
	}
}

func piAssertNoRelations(t *testing.T, fields map[string]json.RawMessage) {
	t.Helper()
	for _, key := range []string{"category", "material", "brand", "measurement_unit"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("plain product response must not include %q: %v", key, fields)
		}
	}
}

func piAssertJSONNumber(t *testing.T, raw json.RawMessage) {
	t.Helper()
	if len(raw) == 0 || raw[0] == '"' {
		t.Fatalf("value = %s, want a JSON number", raw)
	}
}

func (b *piBackend) assertStoredMoney(t *testing.T, productID int64, wantPrice, wantCost string) {
	t.Helper()
	wantPrice = decimal.RequireFromString(wantPrice).StringFixed(2)
	wantCost = decimal.RequireFromString(wantCost).StringFixed(2)
	if b.name == "sqlite" {
		var price, cost int64
		if err := b.gorm.Raw("SELECT price, cost FROM products WHERE id = ?", productID).Row().Scan(&price, &cost); err != nil {
			t.Fatal(err)
		}
		gotPrice := decimal.New(price, -2)
		gotCost := decimal.New(cost, -2)
		if !gotPrice.Equal(decimal.RequireFromString(wantPrice)) || !gotCost.Equal(decimal.RequireFromString(wantCost)) {
			t.Fatalf("sqlite stored money = %s/%s, want %s/%s", gotPrice, gotCost, wantPrice, wantCost)
		}
		return
	}

	var price, cost string
	if err := b.gorm.Raw("SELECT price::text, cost::text FROM products WHERE id = ?", productID).Row().Scan(&price, &cost); err != nil {
		t.Fatal(err)
	}
	if price != wantPrice || cost != wantCost {
		t.Fatalf("postgres stored money = %s/%s, want %s/%s", price, cost, wantPrice, wantCost)
	}
}

func testPIAuth(t *testing.T, b *piBackend) {
	t.Helper()
	routes := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/inventory/products", nil},
		{http.MethodPost, "/api/v1/inventory/products", map[string]any{}},
		{http.MethodGet, "/api/v1/inventory/products/999999", nil},
		{http.MethodPut, "/api/v1/inventory/products/999999", map[string]any{}},
		{http.MethodDelete, "/api/v1/inventory/products/999999", nil},
		{http.MethodGet, "/api/v1/inventory/products/stats/overview", nil},
	}

	invalid := "not-a-valid-token"
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			status, raw := b.request(t, route.method, route.path, route.body, nil)
			piAssertError(t, status, raw, http.StatusUnauthorized, "NOT_AUTHENTICATED")

			status, raw = b.request(t, route.method, route.path, route.body, &invalid)
			piAssertError(t, status, raw, http.StatusUnauthorized, "INVALID_TOKEN")

			status, raw = b.request(t, route.method, route.path, route.body, &b.expiredToken)
			piAssertError(t, status, raw, http.StatusUnauthorized, "INVALID_TOKEN")

			status, raw = b.authRequest(t, route.method, route.path, route.body)
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				t.Fatalf("active non-superuser got %d on %s %s: %s", status, route.method, route.path, raw)
			}
		})
	}

	for _, path := range []string{"/api/v1/inventory/products", "/api/v1/inventory/products/stats/overview"} {
		status, raw := b.authRequest(t, http.MethodGet, path, nil)
		if status != http.StatusOK {
			t.Fatalf("GET %s with active user = %d body = %s, want 200", path, status, raw)
		}
	}
}

func testPIEmptyStats(t *testing.T, b *piBackend) {
	t.Helper()
	status, raw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/stats/overview", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	stats, fields := piDecodeStats(t, raw)
	if stats.TotalProducts != 0 || stats.TotalStock != 0 || stats.LowStockCount != 0 {
		t.Fatalf("empty stats = %+v, want zero counts", stats)
	}
	if stats.TotalInventoryValue.String() != "0" {
		t.Fatalf("empty inventory value = %s, want 0", stats.TotalInventoryValue)
	}
	piAssertJSONNumber(t, fields["total_inventory_value"])
}

func testPICreateDefaults(t *testing.T, b *piBackend) {
	t.Helper()

	body := b.baseBody("PI Create Defaults", "")
	delete(body, "sku")
	body["price"] = piNumber("10.25")
	body["cost"] = piNumber("4.10")
	product, fields := b.createProduct(t, body)
	if !regexp.MustCompile(`^SKU-[0-9A-F]{8}$`).MatchString(product.SKU) {
		t.Fatalf("generated sku = %q, want SKU-XXXXXXXX", product.SKU)
	}
	if product.StockQuantity != 0 || product.LowStockThreshold != 10 {
		t.Fatalf("defaults = stock %d threshold %d, want 0/10", product.StockQuantity, product.LowStockThreshold)
	}
	piAssertMoney(t, product.Price, "10.25")
	piAssertMoney(t, product.Cost, "4.1")
	piAssertJSONNumber(t, fields["price"])
	piAssertJSONNumber(t, fields["cost"])
	piAssertNoRelations(t, fields)
	b.assertStoredMoney(t, product.ID, "10.25", "4.10")

	rounded := b.baseBody("PI Create Rounded", "PI-ROUND-1")
	rounded["price"] = piNumber("10.005")
	rounded["cost"] = piNumber("5.678")
	rounded["stock_quantity"] = 7
	rounded["low_stock_threshold"] = 2
	rounded["size_value"] = ""
	rounded["measurement_unit_id"] = b.fix.UnitID
	roundedProduct, roundedFields := b.createProduct(t, rounded)
	piAssertMoney(t, roundedProduct.Price, "10.01")
	piAssertMoney(t, roundedProduct.Cost, "5.68")
	piAssertJSONNumber(t, roundedFields["price"])
	piAssertJSONNumber(t, roundedFields["cost"])
	if roundedProduct.SizeValue != nil || roundedProduct.MeasurementUnitID == nil || *roundedProduct.MeasurementUnitID != b.fix.UnitID {
		t.Fatalf("optional fields = %v/%v", roundedProduct.SizeValue, roundedProduct.MeasurementUnitID)
	}
	if roundedProduct.StockQuantity != 7 || roundedProduct.LowStockThreshold != 2 {
		t.Fatalf("stock = %d/%d, want 7/2", roundedProduct.StockQuantity, roundedProduct.LowStockThreshold)
	}
	b.assertStoredMoney(t, roundedProduct.ID, "10.01", "5.68")

	supplied := b.baseBody("PI Create Supplied", "PI-SUPPLIED-1")
	suppliedProduct, _ := b.createProduct(t, supplied)
	if suppliedProduct.SKU != "PI-SUPPLIED-1" {
		t.Fatalf("supplied sku = %q", suppliedProduct.SKU)
	}

	emptySKU := b.baseBody("PI Create Empty SKU", "")
	emptySKUProduct, _ := b.createProduct(t, emptySKU)
	if !regexp.MustCompile(`^SKU-[0-9A-F]{8}$`).MatchString(emptySKUProduct.SKU) {
		t.Fatalf("empty sku generated %q", emptySKUProduct.SKU)
	}

	for _, tt := range []struct {
		name string
		body func() map[string]any
	}{
		{"explicit null stock", func() map[string]any {
			body := b.baseBody("PI Null Stock", "PI-NULL-STOCK")
			body["stock_quantity"] = nil
			return body
		}},
		{"explicit null threshold", func() map[string]any {
			body := b.baseBody("PI Null Threshold", "PI-NULL-THRESHOLD")
			body["low_stock_threshold"] = nil
			return body
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodPost, "/api/v1/inventory/products", tt.body())
			piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		})
	}
}

func testPICreateValidation(t *testing.T, b *piBackend) {
	t.Helper()

	missing := []struct {
		name string
		key  string
	}{
		{"name", "name"},
		{"price", "price"},
		{"cost", "cost"},
		{"brand", "brand_id"},
		{"category", "category_id"},
		{"material", "material_id"},
	}
	for _, tt := range missing {
		t.Run("missing "+tt.name, func(t *testing.T) {
			body := b.baseBody("PI Missing "+tt.name, "PI-MISSING-"+tt.name)
			delete(body, tt.key)
			status, raw := b.authRequest(t, http.MethodPost, "/api/v1/inventory/products", body)
			piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		})
	}

	for _, tt := range []struct {
		name  string
		key   string
		value any
	}{
		{"zero price", "price", piNumber("0")},
		{"negative price", "price", piNumber("-1")},
		{"zero cost", "cost", piNumber("0")},
		{"negative stock", "stock_quantity", -1},
		{"negative threshold", "low_stock_threshold", -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := b.baseBody("PI Invalid "+tt.name, "PI-INVALID-"+tt.name)
			body[tt.key] = tt.value
			status, raw := b.authRequest(t, http.MethodPost, "/api/v1/inventory/products", body)
			piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		})
	}

	duplicateBody := b.baseBody("PI Duplicate One", "PI-DUP-1")
	b.createProduct(t, duplicateBody)
	duplicateBody["name"] = "PI Duplicate Two"
	status, raw := b.authRequest(t, http.MethodPost, "/api/v1/inventory/products", duplicateBody)
	piAssertError(t, status, raw, http.StatusBadRequest, "UNIQUE_VIOLATION")

	for _, field := range []string{"brand_id", "category_id", "material_id", "measurement_unit_id"} {
		t.Run("missing fk "+field, func(t *testing.T) {
			body := b.baseBody("PI Missing FK "+field, "PI-FK-"+field)
			body[field] = int64(999999)
			status, raw := b.authRequest(t, http.MethodPost, "/api/v1/inventory/products", body)
			piAssertError(t, status, raw, http.StatusBadRequest, "FOREIGN_KEY_VIOLATION")
		})
	}
}

func testPIGet(t *testing.T, b *piBackend) {
	t.Helper()

	body := b.baseBody("PI Get Hammer", "PI-GET-1")
	body["size_value"] = "M"
	body["measurement_unit_id"] = b.fix.UnitID
	created, _ := b.createProduct(t, body)

	status, raw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/"+decimal.NewFromInt(created.ID).String(), nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d body = %s", status, raw)
	}
	detail, fields := piDecodeDetail(t, raw)
	for _, key := range []string{"category", "material", "brand", "measurement_unit"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("detail response is missing %q: %s", key, raw)
		}
	}
	if detail.Category == nil || detail.Category.ID != b.fix.CategoryID ||
		detail.Material == nil || detail.Material.ID != b.fix.MaterialID ||
		detail.Brand == nil || detail.Brand.ID != b.fix.BrandID ||
		detail.MeasurementUnit == nil || detail.MeasurementUnit.ID != b.fix.UnitID {
		t.Fatalf("relations = %+v/%+v/%+v/%+v", detail.Category, detail.Material, detail.Brand, detail.MeasurementUnit)
	}
	if detail.SizeValue == nil || *detail.SizeValue != "M" {
		t.Fatalf("size = %v, want M", detail.SizeValue)
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/999999", nil)
	piAssertError(t, status, raw, http.StatusNotFound, "")
	var notFound piError
	if err := json.Unmarshal(raw, &notFound); err != nil {
		t.Fatal(err)
	}
	if notFound.Detail != "Product not found" {
		t.Fatalf("detail = %q, want Product not found", notFound.Detail)
	}

	// Current Go behavior: zero/negative ids are rejected as input validation.
	// Legacy FastAPI passes them to the repository and returns 404. Recorded as
	// a code-level parity difference, not a crash.
	for _, id := range []string{"0", "-1"} {
		status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/"+id, nil)
		piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
	}
}

func testPIList(t *testing.T, b *piBackend) {
	t.Helper()

	create := func(name, sku string, mutate func(map[string]any)) piProduct {
		t.Helper()
		body := b.baseBody(name, sku)
		if mutate != nil {
			mutate(body)
		}
		product, _ := b.createProduct(t, body)
		return product
	}

	create("PI Search Árbol", "PI-SEARCH-ACCENT", nil)
	create("PI Search Plain", "PI-SEARCH-PLAIN", nil)
	create("PI 100% Steel", "PI-SEARCH-PERCENT", nil)
	create("PI 100 Steel", "PI-SEARCH-NO-PERCENT", nil)
	create("PI a_b Steel", "PI-SEARCH-UNDERSCORE", nil)
	create("PI axb Steel", "PI-SEARCH-AXB", nil)

	for _, tt := range []struct {
		name  string
		query string
		want  string
	}{
		{"accent lowercase", "árbol", "PI Search Árbol"},
		{"accent uppercase", "ÁRBOL", "PI Search Árbol"},
		{"sku case insensitive", "pi-search-accent", "PI Search Árbol"},
		{"percent literal", "%", "PI 100% Steel"},
		{"underscore literal", "_", "PI a_b Steel"},
	} {
		t.Run("search "+tt.name, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?limit=200&search="+urlQuery(tt.query), nil)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s", status, raw)
			}
			list := piDecodeList(t, raw)
			if list.HasNext || len(list.Items) != 1 || list.Items[0].Name != tt.want {
				t.Fatalf("search %q = %+v has_next=%v, want only %q", tt.query, list.Items, list.HasNext, tt.want)
			}
		})
	}

	create("PI Filter A", "PI-FILTER-A", func(body map[string]any) {
		body["category_id"] = b.fix.CategoryID
		body["brand_id"] = b.fix.BrandID
		body["material_id"] = b.fix.MaterialID
	})
	create("PI Filter B", "PI-FILTER-B", func(body map[string]any) {
		body["category_id"] = b.fix.Category2ID
		body["brand_id"] = b.fix.Brand2ID
		body["material_id"] = b.fix.Material2ID
	})

	for _, tt := range []struct {
		name  string
		query string
		want  string
	}{
		{"category", "category_id=" + decimal.NewFromInt(b.fix.CategoryID).String(), "PI Filter A"},
		{"brand", "brand_id=" + decimal.NewFromInt(b.fix.Brand2ID).String(), "PI Filter B"},
		{"material", "material_id=" + decimal.NewFromInt(b.fix.MaterialID).String(), "PI Filter A"},
	} {
		t.Run("filter "+tt.name, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?limit=200&search=PI+Filter&"+tt.query, nil)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s", status, raw)
			}
			list := piDecodeList(t, raw)
			if len(list.Items) != 1 || list.Items[0].Name != tt.want {
				t.Fatalf("%s filter = %+v, want only %q", tt.name, list.Items, tt.want)
			}
		})
	}

	create("PI Stock In", "PI-STOCK-IN", func(body map[string]any) {
		body["stock_quantity"] = 10
		body["low_stock_threshold"] = 5
	})
	create("PI Stock Equal", "PI-STOCK-EQUAL", func(body map[string]any) {
		body["stock_quantity"] = 5
		body["low_stock_threshold"] = 5
	})
	create("PI Stock Low", "PI-STOCK-LOW", func(body map[string]any) {
		body["stock_quantity"] = 3
		body["low_stock_threshold"] = 5
	})
	create("PI Stock Out", "PI-STOCK-OUT", func(body map[string]any) {
		body["stock_quantity"] = 0
		body["low_stock_threshold"] = 5
	})

	for _, tt := range []struct {
		status string
		want   []string
		absent []string
	}{
		{"in_stock", []string{"PI Stock In", "PI Stock Equal", "PI Stock Low"}, []string{"PI Stock Out"}},
		{"low_stock", []string{"PI Stock Equal", "PI Stock Low"}, []string{"PI Stock In", "PI Stock Out"}},
		{"out_of_stock", []string{"PI Stock Out"}, []string{"PI Stock In", "PI Stock Equal", "PI Stock Low"}},
	} {
		t.Run("stock status "+tt.status, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?limit=200&search=PI+Stock&stock_status="+tt.status, nil)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s", status, raw)
			}
			list := piDecodeList(t, raw)
			names := piNames(list.Items)
			for _, want := range tt.want {
				if !names[want] {
					t.Errorf("stock_status=%s missing %q in %v", tt.status, want, names)
				}
			}
			for _, absent := range tt.absent {
				if names[absent] {
					t.Errorf("stock_status=%s must not include %q", tt.status, absent)
				}
			}
		})
	}

	status, raw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?stock_status=bogus", nil)
	piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")

	pageIDs := make([]int64, 0, 4)
	for _, name := range []string{"PI Page Alpha", "PI Page Beta", "PI Page Gamma", "PI Page Delta"} {
		product := create(name, "PI-PAGE-"+strings.ToUpper(strings.TrimPrefix(name, "PI Page ")), nil)
		pageIDs = append(pageIDs, product.ID)
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?search=PI+Page&limit=2", nil)
	if status != http.StatusOK {
		t.Fatalf("first page status = %d body = %s", status, raw)
	}
	first := piDecodeList(t, raw)
	if !first.HasNext || len(first.Items) != 2 ||
		first.Items[0].ID != pageIDs[3] || first.Items[1].ID != pageIDs[2] {
		t.Fatalf("first page = %+v has_next=%v, want ids %d/%d and true", first.Items, first.HasNext, pageIDs[3], pageIDs[2])
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?search=PI+Page&limit=2&skip=2", nil)
	if status != http.StatusOK {
		t.Fatalf("second page status = %d body = %s", status, raw)
	}
	last := piDecodeList(t, raw)
	if last.HasNext || len(last.Items) != 2 ||
		last.Items[0].ID != pageIDs[1] || last.Items[1].ID != pageIDs[0] {
		t.Fatalf("second page = %+v has_next=%v, want ids %d/%d and false", last.Items, last.HasNext, pageIDs[1], pageIDs[0])
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?search=PI+Page&limit=2&skip=4", nil)
	if status != http.StatusOK {
		t.Fatalf("past-end status = %d body = %s", status, raw)
	}
	past := piDecodeList(t, raw)
	if past.HasNext || len(past.Items) != 0 {
		t.Fatalf("past-end page = %+v, want empty", past)
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?limit=200", nil)
	if status != http.StatusOK {
		t.Fatalf("full list status = %d body = %s", status, raw)
	}
	all := piDecodeList(t, raw)
	for i := 1; i < len(all.Items); i++ {
		if all.Items[i-1].ID <= all.Items[i].ID {
			t.Fatalf("list is not id DESC: %d then %d", all.Items[i-1].ID, all.Items[i].ID)
		}
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products", nil)
	if status != http.StatusOK {
		t.Fatalf("default list status = %d body = %s", status, raw)
	}
	defaults := piDecodeList(t, raw)
	if defaults.Skip != 0 || defaults.Limit != 100 {
		t.Fatalf("defaults = skip %d limit %d, want 0/100", defaults.Skip, defaults.Limit)
	}

	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?limit=200", nil)
	if status != http.StatusOK {
		t.Fatalf("limit cap status = %d body = %s", status, raw)
	}

	for _, query := range []string{
		"skip=-1",
		"limit=0",
		"limit=201",
		"limit=abc",
		"search=" + strings.Repeat("a", 257),
		"category_id=0",
		"brand_id=-1",
		"material_id=abc",
	} {
		t.Run("invalid query "+query, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?"+query, nil)
			piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		})
	}
}

func piNames(items []piProduct) map[string]bool {
	names := make(map[string]bool, len(items))
	for _, item := range items {
		names[item.Name] = true
	}
	return names
}

func urlQuery(value string) string {
	return url.QueryEscape(value)
}

func testPIUpdate(t *testing.T, b *piBackend) {
	t.Helper()

	newUpdateFixture := func(t *testing.T, sku string) piProduct {
		t.Helper()
		body := b.baseBody("PI Update Base", sku)
		body["size_value"] = "M"
		body["measurement_unit_id"] = b.fix.UnitID
		body["price"] = piNumber("10.00")
		body["cost"] = piNumber("5.00")
		body["stock_quantity"] = 3
		body["low_stock_threshold"] = 2
		product, _ := b.createProduct(t, body)
		return product
	}

	base := newUpdateFixture(t, "PI-UPD-BASE")
	status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(base.ID).String(), map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("empty update status = %d body = %s", status, raw)
	}
	updated, fields := piDecodeProduct(t, raw)
	piAssertNoRelations(t, fields)
	if updated.Name != base.Name || updated.SKU != base.SKU || updated.StockQuantity != 3 || updated.LowStockThreshold != 2 {
		t.Fatalf("empty update changed fields: %+v", updated)
	}

	type updateCase struct {
		name  string
		body  map[string]any
		check func(t *testing.T, got piProductDetail)
	}
	cases := []updateCase{
		{"name", map[string]any{"name": "PI Updated Name"}, func(t *testing.T, got piProductDetail) {
			if got.Name != "PI Updated Name" {
				t.Fatalf("name = %q", got.Name)
			}
		}},
		{"sku", map[string]any{"sku": "PI-UPD-NEW-SKU"}, func(t *testing.T, got piProductDetail) {
			if got.SKU != "PI-UPD-NEW-SKU" {
				t.Fatalf("sku = %q", got.SKU)
			}
		}},
		{"size_value", map[string]any{"size_value": "L"}, func(t *testing.T, got piProductDetail) {
			if got.SizeValue == nil || *got.SizeValue != "L" {
				t.Fatalf("size = %v", got.SizeValue)
			}
		}},
		{"measurement_unit_id", map[string]any{"measurement_unit_id": b.fix.Unit2ID}, func(t *testing.T, got piProductDetail) {
			if got.MeasurementUnitID == nil || *got.MeasurementUnitID != b.fix.Unit2ID {
				t.Fatalf("unit = %v", got.MeasurementUnitID)
			}
		}},
		{"price", map[string]any{"price": piNumber("12.345")}, func(t *testing.T, got piProductDetail) {
			piAssertMoney(t, got.Price, "12.35")
		}},
		{"cost", map[string]any{"cost": piNumber("2.345")}, func(t *testing.T, got piProductDetail) {
			piAssertMoney(t, got.Cost, "2.35")
		}},
		{"stock_quantity", map[string]any{"stock_quantity": 9}, func(t *testing.T, got piProductDetail) {
			if got.StockQuantity != 9 {
				t.Fatalf("stock = %d", got.StockQuantity)
			}
		}},
		{"low_stock_threshold", map[string]any{"low_stock_threshold": 7}, func(t *testing.T, got piProductDetail) {
			if got.LowStockThreshold != 7 {
				t.Fatalf("threshold = %d", got.LowStockThreshold)
			}
		}},
		{"brand_id", map[string]any{"brand_id": b.fix.Brand2ID}, func(t *testing.T, got piProductDetail) {
			if got.BrandID != b.fix.Brand2ID {
				t.Fatalf("brand = %d", got.BrandID)
			}
		}},
		{"category_id", map[string]any{"category_id": b.fix.Category2ID}, func(t *testing.T, got piProductDetail) {
			if got.CategoryID != b.fix.Category2ID {
				t.Fatalf("category = %d", got.CategoryID)
			}
		}},
		{"material_id", map[string]any{"material_id": b.fix.Material2ID}, func(t *testing.T, got piProductDetail) {
			if got.MaterialID != b.fix.Material2ID {
				t.Fatalf("material = %d", got.MaterialID)
			}
		}},
	}

	for i, tt := range cases {
		t.Run("update "+tt.name, func(t *testing.T) {
			product := newUpdateFixture(t, "PI-UPD-"+decimal.NewFromInt(int64(i)).String())
			status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), tt.body)
			if status != http.StatusOK {
				t.Fatalf("status = %d body = %s", status, raw)
			}
			_, fields := piDecodeProduct(t, raw)
			piAssertNoRelations(t, fields)

			status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), nil)
			if status != http.StatusOK {
				t.Fatalf("get status = %d body = %s", status, raw)
			}
			got, _ := piDecodeDetail(t, raw)
			tt.check(t, got)
			if tt.name == "price" {
				b.assertStoredMoney(t, product.ID, "12.35", "5.00")
			}
			if tt.name == "cost" {
				b.assertStoredMoney(t, product.ID, "10.00", "2.35")
			}
		})
	}

	t.Run("explicit null clears only size and unit", func(t *testing.T) {
		product := newUpdateFixture(t, "PI-UPD-NULL")
		status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), map[string]any{
			"size_value":          nil,
			"measurement_unit_id": nil,
		})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s", status, raw)
		}
		_, fields := piDecodeProduct(t, raw)
		piAssertNoRelations(t, fields)

		status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), nil)
		if status != http.StatusOK {
			t.Fatalf("get status = %d body = %s", status, raw)
		}
		got, _ := piDecodeDetail(t, raw)
		if got.SizeValue != nil || got.MeasurementUnitID != nil {
			t.Fatalf("cleared fields = %v/%v, want nil", got.SizeValue, got.MeasurementUnitID)
		}
	})

	t.Run("empty size clears", func(t *testing.T) {
		product := newUpdateFixture(t, "PI-UPD-EMPTY-SIZE")
		status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), map[string]any{"size_value": ""})
		if status != http.StatusOK {
			t.Fatalf("status = %d body = %s", status, raw)
		}
		status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), nil)
		if status != http.StatusOK {
			t.Fatalf("get status = %d body = %s", status, raw)
		}
		got, _ := piDecodeDetail(t, raw)
		if got.SizeValue != nil {
			t.Fatalf("size = %v, want nil", got.SizeValue)
		}
	})

	required := newUpdateFixture(t, "PI-UPD-REQUIRED")
	for _, field := range []string{"name", "sku", "price", "cost", "stock_quantity", "low_stock_threshold", "brand_id", "category_id", "material_id"} {
		t.Run("null required "+field, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(required.ID).String(), map[string]any{field: nil})
			piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		})
	}

	for _, tt := range []struct {
		name string
		body map[string]any
	}{
		{"empty name", map[string]any{"name": ""}},
		{"blank name", map[string]any{"name": "   "}},
		{"empty sku", map[string]any{"sku": ""}},
		{"blank sku", map[string]any{"sku": "   "}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(required.ID).String(), tt.body)
			piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		})
	}

	t.Run("duplicate sku", func(t *testing.T) {
		first := newUpdateFixture(t, "PI-UPD-DUP-A")
		second := newUpdateFixture(t, "PI-UPD-DUP-B")
		_ = first
		status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(second.ID).String(), map[string]any{"sku": "PI-UPD-DUP-A"})
		piAssertError(t, status, raw, http.StatusBadRequest, "UNIQUE_VIOLATION")
	})

	t.Run("missing product", func(t *testing.T) {
		status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/999999", map[string]any{"name": "Ghost"})
		piAssertError(t, status, raw, http.StatusNotFound, "")
	})
}

func testPIDelete(t *testing.T, b *piBackend) {
	t.Helper()

	body := b.baseBody("PI Delete Plain", "PI-DEL-1")
	product, _ := b.createProduct(t, body)
	status, raw := b.authRequest(t, http.MethodDelete, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), nil)
	if status != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", status, raw)
	}
	deleted, fields := piDecodeProduct(t, raw)
	piAssertNoRelations(t, fields)
	if deleted.ID != product.ID || deleted.SKU != product.SKU {
		t.Fatalf("deleted = %+v, want %+v", deleted, product)
	}
	status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), nil)
	piAssertError(t, status, raw, http.StatusNotFound, "")
	status, raw = b.authRequest(t, http.MethodDelete, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), nil)
	piAssertError(t, status, raw, http.StatusNotFound, "")

	for _, kind := range []string{"supplier_catalog", "purchase_order", "sale"} {
		t.Run("fk conflict "+kind, func(t *testing.T) {
			body := b.baseBody("PI Delete FK "+kind, "PI-DEL-FK-"+kind)
			referenced, _ := b.createProduct(t, body)
			b.seedProductReference(t, referenced.ID, kind)

			status, raw := b.authRequest(t, http.MethodDelete, "/api/v1/inventory/products/"+decimal.NewFromInt(referenced.ID).String(), nil)
			piAssertError(t, status, raw, http.StatusBadRequest, "FOREIGN_KEY_VIOLATION")

			status, raw = b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/"+decimal.NewFromInt(referenced.ID).String(), nil)
			if status != http.StatusOK {
				t.Fatalf("referenced product must survive: status = %d body = %s", status, raw)
			}
		})
	}
}

func (b *piBackend) seedProductReference(t *testing.T, productID int64, kind string) {
	t.Helper()
	money := func(cents int64) any {
		if b.name == "sqlite" {
			return cents
		}
		return decimal.New(cents, -2).StringFixed(2)
	}
	const timestamp = "2026-01-01T00:00:00.000000000Z"

	switch kind {
	case "supplier_catalog":
		if err := b.gorm.Exec("INSERT INTO supplier_catalogs (product_id, last_quoted_cost) VALUES (?, ?)", productID, money(100)).Error; err != nil {
			t.Fatal(err)
		}
	case "purchase_order":
		var orderID int64
		if err := b.gorm.Raw(
			"INSERT INTO purchase_orders (order_date, total_amount) VALUES (?, ?) RETURNING id",
			timestamp, money(0),
		).Scan(&orderID).Error; err != nil {
			t.Fatal(err)
		}
		if err := b.gorm.Exec(
			"INSERT INTO order_items (order_id, product_id, quantity, unit_cost, subtotal) VALUES (?, ?, ?, ?, ?)",
			orderID, productID, 1, money(100), money(100),
		).Error; err != nil {
			t.Fatal(err)
		}
	case "sale":
		var saleID int64
		if err := b.gorm.Raw(
			"INSERT INTO sales (created_at, total_amount, amount_paid) VALUES (?, ?, ?) RETURNING id",
			timestamp, money(0), money(0),
		).Scan(&saleID).Error; err != nil {
			t.Fatal(err)
		}
		if err := b.gorm.Exec(
			"INSERT INTO sale_items (sale_id, product_id, quantity, unit_price, subtotal, delivered_quantity) VALUES (?, ?, ?, ?, ?, ?)",
			saleID, productID, 1, money(100), money(100), 1,
		).Error; err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown reference kind %q", kind)
	}
}

func testPIStats(t *testing.T, b *piBackend) {
	t.Helper()

	beforeStatus, beforeRaw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/stats/overview", nil)
	if beforeStatus != http.StatusOK {
		t.Fatalf("before status = %d body = %s", beforeStatus, beforeRaw)
	}
	before, _ := piDecodeStats(t, beforeRaw)

	create := func(name, sku, price string, stock, threshold int) piProduct {
		t.Helper()
		body := b.baseBody(name, sku)
		body["price"] = piNumber(price)
		body["cost"] = piNumber("0.05")
		body["stock_quantity"] = stock
		body["low_stock_threshold"] = threshold
		product, _ := b.createProduct(t, body)
		return product
	}
	equal := create("PI Stats Equal", "PI-STATS-EQUAL", "0.10", 5, 5)
	zero := create("PI Stats Zero", "PI-STATS-ZERO", "0.20", 0, 5)
	high := create("PI Stats High", "PI-STATS-HIGH", "1.10", 2, 1)
	create("PI Stats Low", "PI-STATS-LOW", "0.20", 3, 5)
	_ = high

	afterStatus, afterRaw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products/stats/overview", nil)
	if afterStatus != http.StatusOK {
		t.Fatalf("after status = %d body = %s", afterStatus, afterRaw)
	}
	after, fields := piDecodeStats(t, afterRaw)

	if after.TotalProducts-before.TotalProducts != 4 {
		t.Errorf("total_products delta = %d, want 4", after.TotalProducts-before.TotalProducts)
	}
	if after.TotalStock-before.TotalStock != 10 {
		t.Errorf("total_stock delta = %d, want 10", after.TotalStock-before.TotalStock)
	}
	if after.LowStockCount-before.LowStockCount != 2 {
		t.Errorf("low_stock_count delta = %d, want 2", after.LowStockCount-before.LowStockCount)
	}
	gotValue, err := decimal.NewFromString(after.TotalInventoryValue.String())
	if err != nil {
		t.Fatal(err)
	}
	beforeValue, err := decimal.NewFromString(before.TotalInventoryValue.String())
	if err != nil {
		t.Fatal(err)
	}
	if !gotValue.Sub(beforeValue).Equal(decimal.RequireFromString("3.30")) {
		t.Errorf("inventory value delta = %s, want 3.30", gotValue.Sub(beforeValue))
	}
	piAssertJSONNumber(t, fields["total_inventory_value"])

	b.assertStoredMoney(t, equal.ID, "0.10", "0.05")
	b.assertStoredMoney(t, zero.ID, "0.20", "0.05")

	status, raw := b.authRequest(t, http.MethodGet, "/api/v1/inventory/products?limit=200&search=PI+Stats&stock_status=low_stock", nil)
	if status != http.StatusOK {
		t.Fatalf("low stock list status = %d body = %s", status, raw)
	}
	lowNames := piNames(piDecodeList(t, raw).Items)
	if !lowNames["PI Stats Equal"] || !lowNames["PI Stats Low"] {
		t.Fatalf("low stock list = %v, want Equal and Low", lowNames)
	}
	if lowNames["PI Stats Zero"] {
		t.Fatalf("zero stock must not count as low: %v", lowNames)
	}
}

func testPIPriceHistory(t *testing.T, b *piBackend) {
	t.Helper()
	var before int64
	if err := b.gorm.Raw("SELECT COUNT(*) FROM price_history").Row().Scan(&before); err != nil {
		t.Fatal(err)
	}

	body := b.baseBody("PI History Product", "PI-HIST-1")
	product, _ := b.createProduct(t, body)
	status, raw := b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), map[string]any{"price": piNumber("11.00")})
	if status != http.StatusOK {
		t.Fatalf("update status = %d body = %s", status, raw)
	}
	status, raw = b.authRequest(t, http.MethodDelete, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), nil)
	if status != http.StatusOK {
		t.Fatalf("delete status = %d body = %s", status, raw)
	}

	var after int64
	if err := b.gorm.Raw("SELECT COUNT(*) FROM price_history").Row().Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("price_history rows = %d, want %d (create/update/delete must not write)", after, before)
	}
}

func testPIUnknownFields(t *testing.T, b *piBackend) {
	t.Helper()

	// Current Go behavior differs from legacy Pydantic, which ignores unknown
	// fields. The frontend currently sends min_stock_quantity, so this is
	// recorded as a parity/integration defect rather than treated as success.
	body := b.baseBody("PI Unknown Field", "PI-UNKNOWN-1")
	body["min_stock_quantity"] = 4
	status, raw := b.authRequest(t, http.MethodPost, "/api/v1/inventory/products", body)
	piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
	t.Log("Go rejects unknown fields with 422; legacy Pydantic ignores them")

	product, _ := b.createProduct(t, b.baseBody("PI Unknown Update", "PI-UNKNOWN-2"))
	status, raw = b.authRequest(t, http.MethodPut, "/api/v1/inventory/products/"+decimal.NewFromInt(product.ID).String(), map[string]any{"min_stock_quantity": 4})
	piAssertError(t, status, raw, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
}
