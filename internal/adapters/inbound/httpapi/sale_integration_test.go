package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

type siBackend struct {
	name       string
	gorm       *gorm.DB
	conn       *sql.DB
	server     *httptest.Server
	userID     int64
	clientID   int64
	productIDs [3]int64
	token      string
	sales      outbound.SaleRepository
	products   outbound.ProductRepository
}

type siSale struct {
	ID             int64       `json:"id"`
	TotalAmount    json.Number `json:"total_amount"`
	DebtAmount     json.Number `json:"debt_amount"`
	PaymentStatus  string      `json:"payment_status"`
	DeliveryStatus string      `json:"delivery_status"`
	PaymentMethod  *string     `json:"payment_method"`
	AmountPaid     json.Number `json:"amount_paid"`
	ClientID       *int64      `json:"client_id"`
	CreatedAt      string      `json:"created_at"`
	UpdatedAt      *string     `json:"updated_at"`
	CreatedByID    int64       `json:"created_by_id"`
	CreatedBy      *authUser   `json:"created_by"`
	Client         *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"client"`
}

type siItem struct {
	ID                int64       `json:"id"`
	SaleID            int64       `json:"sale_id"`
	ProductID         int64       `json:"product_id"`
	Quantity          int         `json:"quantity"`
	DeliveredQuantity int         `json:"delivered_quantity"`
	UnitPrice         json.Number `json:"unit_price"`
	Subtotal          json.Number `json:"subtotal"`
	ProductName       string      `json:"product_name"`
	ProductSKU        string      `json:"product_sku"`
	BrandName         *string     `json:"brand_name"`
	MaterialName      *string     `json:"material_name"`
	CategoryName      *string     `json:"category_name"`
}

type siList[T any] struct {
	Items   []T  `json:"items"`
	HasNext bool `json:"has_next"`
	Skip    int  `json:"skip"`
	Limit   int  `json:"limit"`
}

type siStats struct {
	UnpaidCount      int64       `json:"unpaid_count"`
	UndeliveredCount int64       `json:"undelivered_count"`
	TotalAmountSum   json.Number `json:"total_amount_sum"`
	AmountPaidSum    json.Number `json:"amount_paid_sum"`
}

func TestSaleEndpointsAgainstBothBackends(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		backend := newSISQLiteBackend(t)
		runSaleBackendSuite(t, backend)
	})
	t.Run("postgres", func(t *testing.T) {
		backend := newSIPostgresBackend(t)
		runSaleBackendSuite(t, backend)
	})
}

func runSaleBackendSuite(t *testing.T, backend *siBackend) {
	t.Helper()

	createdAt := time.Now().UTC().Add(-time.Minute)
	status, raw := backend.request(t, http.MethodPost, "/api/v1/sales", map[string]any{
		"payment_method": "cash",
		"amount_paid":    5.25,
		"client_id":      backend.clientID,
		"items": []map[string]any{
			{"product_id": backend.productIDs[0], "quantity": 2, "delivered_quantity": 1},
			{"product_id": backend.productIDs[1], "quantity": 1},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("create status = %d body = %s", status, raw)
	}
	sale := siDecode[siSale](t, raw)
	if sale.CreatedByID != backend.userID || sale.CreatedBy == nil || sale.CreatedBy.Email != "sales-"+backend.name+"@example.com" {
		t.Fatalf("created_by = %+v id %d", sale.CreatedBy, sale.CreatedByID)
	}
	if sale.Client == nil || sale.Client.ID != backend.clientID {
		t.Fatalf("client = %+v", sale.Client)
	}
	if sale.PaymentStatus != domain.PaymentStatusPartial || sale.DeliveryStatus != domain.DeliveryStatusPartial {
		t.Fatalf("statuses = %q/%q", sale.PaymentStatus, sale.DeliveryStatus)
	}
	assertNumber(t, sale.TotalAmount, "21.20")
	assertNumber(t, sale.AmountPaid, "5.25")
	assertNumber(t, sale.DebtAmount, "15.95")
	if sale.UpdatedAt != nil {
		t.Fatalf("updated_at = %v, want nil", sale.UpdatedAt)
	}
	created, err := time.Parse(time.RFC3339, sale.CreatedAt)
	if err != nil || created.IsZero() {
		t.Fatalf("created_at = %q: %v", sale.CreatedAt, err)
	}
	if created.Before(createdAt) {
		t.Fatalf("created_at = %s, want recent", created)
	}
	if stock := backend.stock(t, backend.productIDs[0]); stock != 8 {
		t.Fatalf("product 1 stock = %d, want 8", stock)
	}
	if stock := backend.stock(t, backend.productIDs[1]); stock != 9 {
		t.Fatalf("product 2 stock = %d, want 9", stock)
	}

	t.Run("get and list filters", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodGet, "/api/v1/sales/"+itoa(sale.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("get status = %d body = %s", status, raw)
		}
		got := siDecode[siSale](t, raw)
		if got.ID != sale.ID || got.CreatedBy == nil || got.Client == nil {
			t.Fatalf("get sale = %+v", got)
		}

		start := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
		end := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
		path := "/api/v1/sales?client_id=" + itoa(backend.clientID) +
			"&payment_status=partial&delivery_status=partial&payment_method=cash" +
			"&start_date=" + start + "&end_date=" + end
		status, raw = backend.request(t, http.MethodGet, path, nil)
		if status != http.StatusOK {
			t.Fatalf("list status = %d body = %s", status, raw)
		}
		list := siDecode[siList[siSale]](t, raw)
		if len(list.Items) != 1 || list.Items[0].ID != sale.ID || list.HasNext {
			t.Fatalf("filtered list = %+v", list)
		}

		status, raw = backend.request(t, http.MethodGet, "/api/v1/sales?no_client=true", nil)
		if status != http.StatusOK {
			t.Fatalf("no_client status = %d body = %s", status, raw)
		}
		if empty := siDecode[siList[siSale]](t, raw); len(empty.Items) != 0 {
			t.Fatalf("no_client returned %+v", empty.Items)
		}
	})

	t.Run("stats", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodGet, "/api/v1/sales/stats", nil)
		if status != http.StatusOK {
			t.Fatalf("stats status = %d body = %s", status, raw)
		}
		stats := siDecode[siStats](t, raw)
		if stats.UnpaidCount != 1 || stats.UndeliveredCount != 1 {
			t.Fatalf("stats counts = %+v", stats)
		}
		assertNumber(t, stats.TotalAmountSum, "21.20")
		assertNumber(t, stats.AmountPaidSum, "5.25")
	})

	t.Run("update payment", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodPut, "/api/v1/sales/"+itoa(sale.ID), map[string]any{
			"amount_paid": 21.20,
		})
		if status != http.StatusOK {
			t.Fatalf("update status = %d body = %s", status, raw)
		}
		updated := siDecode[siSale](t, raw)
		if updated.PaymentStatus != domain.PaymentStatusPaid {
			t.Fatalf("payment status = %q, want paid", updated.PaymentStatus)
		}
		assertNumber(t, updated.DebtAmount, "0")
		if updated.UpdatedAt == nil {
			t.Fatal("updated_at is nil")
		}
		if _, err := time.Parse(time.RFC3339, *updated.UpdatedAt); err != nil {
			t.Fatalf("updated_at = %q: %v", *updated.UpdatedAt, err)
		}
		sale = updated
	})

	t.Run("items", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodGet, "/api/v1/sales/"+itoa(sale.ID)+"/items", nil)
		if status != http.StatusOK {
			t.Fatalf("items status = %d body = %s", status, raw)
		}
		list := siDecode[siList[siItem]](t, raw)
		if len(list.Items) != 2 || list.Items[0].ID >= list.Items[1].ID {
			t.Fatalf("items = %+v, want id ASC", list.Items)
		}
		if list.Items[0].ProductName == "" || list.Items[0].ProductSKU == "" ||
			list.Items[0].BrandName == nil || list.Items[0].MaterialName == nil || list.Items[0].CategoryName == nil {
			t.Fatalf("item relations = %+v", list.Items[0])
		}
		assertNumber(t, list.Items[0].UnitPrice, "10.50")
		assertNumber(t, list.Items[0].Subtotal, "21.00")

		status, raw = backend.request(t, http.MethodPost, "/api/v1/sales/"+itoa(sale.ID)+"/items", map[string]any{
			"product_id": backend.productIDs[0], "quantity": 1,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("duplicate item status = %d body = %s", status, raw)
		}
		if !contains(raw, "already exists in this sale") {
			t.Fatalf("duplicate detail = %s", raw)
		}

		status, raw = backend.request(t, http.MethodPost, "/api/v1/sales/"+itoa(sale.ID)+"/items", map[string]any{
			"product_id": backend.productIDs[2], "quantity": 2,
		})
		if status != http.StatusOK {
			t.Fatalf("add item status = %d body = %s", status, raw)
		}
		added := siDecode[siSale](t, raw)
		assertNumber(t, added.TotalAmount, "23.70")
		if stock := backend.stock(t, backend.productIDs[2]); stock != 8 {
			t.Fatalf("product 3 stock = %d, want 8", stock)
		}

		status, raw = backend.request(t, http.MethodGet, "/api/v1/sales/"+itoa(sale.ID)+"/items", nil)
		if status != http.StatusOK {
			t.Fatalf("items status = %d body = %s", status, raw)
		}
		list = siDecode[siList[siItem]](t, raw)
		var third siItem
		for _, item := range list.Items {
			if item.ProductID == backend.productIDs[2] {
				third = item
			}
		}
		if third.ID == 0 {
			t.Fatal("added item not found")
		}

		status, raw = backend.request(t, http.MethodPut, "/api/v1/sales/"+itoa(sale.ID)+"/items/"+itoa(third.ID), map[string]any{
			"quantity": 1,
		})
		if status != http.StatusOK {
			t.Fatalf("update item status = %d body = %s", status, raw)
		}
		updated := siDecode[siSale](t, raw)
		assertNumber(t, updated.TotalAmount, "22.45")
		if stock := backend.stock(t, backend.productIDs[2]); stock != 9 {
			t.Fatalf("product 3 stock = %d, want 9", stock)
		}

		status, raw = backend.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(sale.ID)+"/items/"+itoa(third.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete item status = %d body = %s", status, raw)
		}
		deleted := siDecode[siSale](t, raw)
		assertNumber(t, deleted.TotalAmount, "21.20")
		if stock := backend.stock(t, backend.productIDs[2]); stock != 10 {
			t.Fatalf("product 3 stock = %d, want 10", stock)
		}
	})

	t.Run("delete restores stock", func(t *testing.T) {
		status, raw := backend.request(t, http.MethodDelete, "/api/v1/sales/"+itoa(sale.ID), nil)
		if status != http.StatusOK {
			t.Fatalf("delete status = %d body = %s", status, raw)
		}
		deleted := siDecode[siSale](t, raw)
		if deleted.ID != sale.ID {
			t.Fatalf("deleted id = %d, want %d", deleted.ID, sale.ID)
		}
		if stock := backend.stock(t, backend.productIDs[0]); stock != 10 {
			t.Fatalf("product 1 stock = %d, want 10", stock)
		}
		if stock := backend.stock(t, backend.productIDs[1]); stock != 10 {
			t.Fatalf("product 2 stock = %d, want 10", stock)
		}
		status, _ = backend.request(t, http.MethodGet, "/api/v1/sales/"+itoa(sale.ID), nil)
		if status != http.StatusNotFound {
			t.Fatalf("get deleted status = %d, want 404", status)
		}
	})
}

func newSISQLiteBackend(t *testing.T) *siBackend {
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
	return newSIBackend(t, "sqlite", gormDB, conn)
}

func newSIPostgresBackend(t *testing.T) *siBackend {
	t.Helper()
	dsn := os.Getenv("INVENTORY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DATABASE_URL to run the postgres sale integration suite")
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
	return newSIBackend(t, "postgres", gormDB, conn)
}

func newSIBackend(t *testing.T, name string, gormDB *gorm.DB, conn *sql.DB) *siBackend {
	t.Helper()
	ctx := context.Background()
	b := &siBackend{
		name:     name,
		gorm:     gormDB,
		conn:     conn,
		sales:    gormstore.NewSaleStore(gormDB),
		products: gormstore.NewProductStore(gormDB),
	}

	categories := gormstore.NewCategoryStore(gormDB)
	brands := gormstore.NewBrandStore(gormDB)
	materials := gormstore.NewMaterialStore(gormDB)
	category, err := categories.Create(ctx, &domain.Category{Name: name + " sale category"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := brands.Create(ctx, &domain.Brand{Name: name + " sale brand"})
	if err != nil {
		t.Fatal(err)
	}
	material, err := materials.Create(ctx, &domain.Material{Name: name + " sale material"})
	if err != nil {
		t.Fatal(err)
	}
	prices := []string{"10.50", "0.20", "1.25"}
	for i := range b.productIDs {
		product, err := b.products.Create(ctx, &domain.Product{
			SKU:               name + "-sale-sku-" + itoa(int64(i+1)),
			Name:              name + " sale product " + itoa(int64(i+1)),
			Price:             decimal.RequireFromString(prices[i]),
			Cost:              decimal.RequireFromString("0.10"),
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

	client, err := gormstore.NewClientStore(gormDB).Create(ctx, &domain.Client{
		Name:      name + " sale client",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	b.clientID = client.ID

	users := gormstore.NewUserStore(gormDB)
	hasher := security.NewPasswordHasher(4)
	hash, err := hasher.Hash("sale-integration-password")
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{
		FullName:       "Sale Integration User",
		Email:          "sales-" + name + "@example.com",
		HashedPassword: hash,
		IsActive:       true,
	}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	b.userID = user.ID

	const secret = "sale-integration-secret"
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
		Auth:    authService,
		Users:   application.NewUserService(users, hasher),
		Sales:   application.NewSaleService(b.sales, gormstore.NewUnitOfWork(gormDB)),
		Logger:  logger,
		Backend: name,
	}))
	t.Cleanup(b.server.Close)
	return b
}

func (b *siBackend) request(t *testing.T, method, path string, body any) (int, []byte) {
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
	request.Header.Set("Authorization", "Bearer "+b.token)
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

func (b *siBackend) stock(t *testing.T, productID int64) int {
	t.Helper()
	var stock int
	if err := b.gorm.Raw("SELECT stock_quantity FROM products WHERE id = ?", productID).Row().Scan(&stock); err != nil {
		t.Fatal(err)
	}
	return stock
}

func siDecode[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %T: %v body = %s", value, err, raw)
	}
	return value
}

func assertNumber(t *testing.T, value json.Number, want string) {
	t.Helper()
	got := decimal.RequireFromString(string(value))
	expected := decimal.RequireFromString(want)
	if !got.Equal(expected) {
		t.Fatalf("number = %s, want %s", got, expected)
	}
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}
