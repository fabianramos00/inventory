package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

type fakePurchaseEnv struct {
	orders    map[int64]*domain.PurchaseOrder
	items     map[int64][]*domain.OrderItem
	products  map[int64]*domain.ProductDetail
	providers map[int64]*domain.Provider
	catalogs  []*domain.SupplierCatalog
	stock     map[int64]int64
	nextOrder int64
	nextItem  int64
}

func newFakePurchaseEnv() *fakePurchaseEnv {
	return &fakePurchaseEnv{
		orders:    map[int64]*domain.PurchaseOrder{},
		items:     map[int64][]*domain.OrderItem{},
		products:  map[int64]*domain.ProductDetail{},
		providers: map[int64]*domain.Provider{},
		stock:     map[int64]int64{},
	}
}

func (e *fakePurchaseEnv) addProvider(id int64, name string) {
	e.providers[id] = &domain.Provider{ID: id, Name: name}
}

func (e *fakePurchaseEnv) addProduct(id int64, sku, name string, stock int64) {
	e.products[id] = &domain.ProductDetail{Product: domain.Product{
		ID: id, SKU: sku, Name: name, Price: decimal.RequireFromString("10"),
		Cost: decimal.RequireFromString("1"), StockQuantity: int(stock),
	}}
	e.stock[id] = stock
}

func (e *fakePurchaseEnv) service() *PurchaseOrderService {
	return NewPurchaseOrderService(&fakeOrderRepo{env: e}, &fakePurchaseUoW{env: e})
}

func (e *fakePurchaseEnv) catalog(productID, providerID int64) *domain.SupplierCatalog {
	for _, catalog := range e.catalogs {
		if catalog.ProductID == productID && catalog.ProviderID == providerID {
			return catalog
		}
	}
	return nil
}

type fakePurchaseUoW struct {
	env *fakePurchaseEnv
}

func (u *fakePurchaseUoW) WithinTx(ctx context.Context, fn func(context.Context, outbound.TxRepos) error) error {
	return fn(ctx, outbound.TxRepos{
		Orders:    &fakeOrderRepo{env: u.env},
		Catalogs:  &fakeCatalogRepo{env: u.env},
		Providers: &fakeProviderRepo{env: u.env},
		Products:  &fakePurchaseProductRepo{env: u.env},
		Stock:     &fakePurchaseStock{env: u.env},
	})
}

type fakeOrderRepo struct {
	env *fakePurchaseEnv
}

var _ outbound.PurchaseOrderRepository = (*fakeOrderRepo)(nil)

func (r *fakeOrderRepo) List(_ context.Context, filter outbound.PurchaseOrderFilter) ([]*domain.PurchaseOrder, bool, error) {
	items := make([]*domain.PurchaseOrder, 0, len(r.env.orders))
	for id := r.env.nextOrder; id >= 1; id-- {
		if order, ok := r.env.orders[id]; ok {
			items = append(items, copyPurchaseOrder(order))
		}
	}
	if filter.Limit > 0 && len(items) > filter.Limit {
		return items[:filter.Limit], true, nil
	}
	return items, false, nil
}

func (r *fakeOrderRepo) Get(_ context.Context, id int64) (*domain.PurchaseOrder, error) {
	order, ok := r.env.orders[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copied := copyPurchaseOrder(order)
	if provider, ok := r.env.providers[copied.ProviderID]; ok {
		copied.Provider = provider
	}
	return copied, nil
}

func (r *fakeOrderRepo) GetForUpdate(ctx context.Context, id int64) (*domain.PurchaseOrder, error) {
	return r.Get(ctx, id)
}

func (r *fakeOrderRepo) Create(_ context.Context, order *domain.PurchaseOrder, items []*domain.OrderItem) (*domain.PurchaseOrder, error) {
	r.env.nextOrder++
	order.ID = r.env.nextOrder
	copy := copyPurchaseOrder(order)
	if provider, ok := r.env.providers[order.ProviderID]; ok {
		copy.Provider = provider
	}
	r.env.orders[order.ID] = copy
	for _, item := range items {
		item.OrderID = order.ID
		if _, err := r.CreateItem(context.Background(), item); err != nil {
			return nil, err
		}
	}
	return copyPurchaseOrder(copy), nil
}

func (r *fakeOrderRepo) Update(_ context.Context, id int64, order *domain.PurchaseOrder) error {
	if _, ok := r.env.orders[id]; !ok {
		return domain.ErrNotFound
	}
	copy := copyPurchaseOrder(order)
	copy.ID = id
	r.env.orders[id] = copy
	return nil
}

func (r *fakeOrderRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.env.orders[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.env.orders, id)
	delete(r.env.items, id)
	return nil
}

func (r *fakeOrderRepo) ListItems(_ context.Context, orderID int64, skip, limit int) ([]*domain.OrderItem, bool, error) {
	items := r.env.items[orderID]
	if skip >= len(items) {
		return nil, false, nil
	}
	items = items[skip:]
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	return copyOrderItems(items), hasNext, nil
}

func (r *fakeOrderRepo) ListAllItems(_ context.Context, orderID int64) ([]*domain.OrderItem, error) {
	return copyOrderItems(r.env.items[orderID]), nil
}

func (r *fakeOrderRepo) GetItem(_ context.Context, orderID, itemID int64) (*domain.OrderItem, error) {
	for _, item := range r.env.items[orderID] {
		if item.ID == itemID {
			return copyOrderItem(item), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeOrderRepo) GetItemByProduct(_ context.Context, orderID, productID int64) (*domain.OrderItem, error) {
	for _, item := range r.env.items[orderID] {
		if item.ProductID == productID {
			return copyOrderItem(item), nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeOrderRepo) CreateItem(_ context.Context, item *domain.OrderItem) (*domain.OrderItem, error) {
	r.env.nextItem++
	item.ID = r.env.nextItem
	r.env.items[item.OrderID] = append(r.env.items[item.OrderID], copyOrderItem(item))
	return copyOrderItem(item), nil
}

func (r *fakeOrderRepo) UpdateItem(_ context.Context, item *domain.OrderItem) error {
	items := r.env.items[item.OrderID]
	for i := range items {
		if items[i].ID == item.ID {
			items[i] = copyOrderItem(item)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeOrderRepo) DeleteItem(_ context.Context, orderID, itemID int64) error {
	items := r.env.items[orderID]
	for i := range items {
		if items[i].ID == itemID {
			r.env.items[orderID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeOrderRepo) DeleteItems(_ context.Context, orderID int64) error {
	delete(r.env.items, orderID)
	return nil
}

type fakeCatalogRepo struct {
	env *fakePurchaseEnv
}

var _ outbound.SupplierCatalogRepository = (*fakeCatalogRepo)(nil)

func (f *fakeCatalogRepo) List(context.Context, int, int, *int64, *int64) ([]*domain.SupplierCatalog, bool, error) {
	return f.env.catalogs, false, nil
}

func (f *fakeCatalogRepo) Get(context.Context, int64) (*domain.SupplierCatalog, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeCatalogRepo) Upsert(_ context.Context, catalog *domain.SupplierCatalog) error {
	if existing := f.env.catalog(catalog.ProductID, catalog.ProviderID); existing != nil {
		existing.LastQuotedCost = catalog.LastQuotedCost
		existing.LeadTime = catalog.LeadTime
		if catalog.SupplierSKU != nil && *catalog.SupplierSKU != "" {
			existing.SupplierSKU = catalog.SupplierSKU
		}
		catalog.ID = existing.ID
		return nil
	}
	catalog.ID = int64(len(f.env.catalogs) + 1)
	if catalog.SupplierSKU != nil && *catalog.SupplierSKU == "" {
		catalog.SupplierSKU = nil
	}
	f.env.catalogs = append(f.env.catalogs, catalog)
	return nil
}

type fakeProviderRepo struct {
	env *fakePurchaseEnv
}

var _ outbound.ProviderRepository = (*fakeProviderRepo)(nil)

func (f *fakeProviderRepo) List(context.Context, int, int, string) ([]*domain.Provider, bool, error) {
	return nil, false, nil
}

func (f *fakeProviderRepo) Get(_ context.Context, id int64) (*domain.Provider, error) {
	provider, ok := f.env.providers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return provider, nil
}

func (f *fakeProviderRepo) Create(context.Context, *domain.Provider) (*domain.Provider, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProviderRepo) Update(context.Context, int64, *domain.Provider) error {
	return errors.New("not implemented")
}

func (f *fakeProviderRepo) Delete(context.Context, int64) (*domain.Provider, error) {
	return nil, errors.New("not implemented")
}

type fakePurchaseProductRepo struct {
	env *fakePurchaseEnv
}

var _ outbound.ProductRepository = (*fakePurchaseProductRepo)(nil)

func (f *fakePurchaseProductRepo) List(context.Context, outbound.ProductFilter) ([]*domain.ProductDetail, bool, error) {
	return nil, false, nil
}

func (f *fakePurchaseProductRepo) Get(_ context.Context, id int64) (*domain.ProductDetail, error) {
	product, ok := f.env.products[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return product, nil
}

func (f *fakePurchaseProductRepo) Create(context.Context, *domain.Product) (*domain.Product, error) {
	return nil, errors.New("not implemented")
}

func (f *fakePurchaseProductRepo) Update(context.Context, int64, *domain.Product) error {
	return errors.New("not implemented")
}

func (f *fakePurchaseProductRepo) Delete(context.Context, int64) (*domain.Product, error) {
	return nil, errors.New("not implemented")
}

func (f *fakePurchaseProductRepo) Stats(context.Context) (*domain.ProductStats, error) {
	return nil, errors.New("not implemented")
}

type fakePurchaseStock struct {
	env *fakePurchaseEnv
}

var _ outbound.StockRepository = (*fakePurchaseStock)(nil)

func (f *fakePurchaseStock) LockForUpdate(_ context.Context, ids []int64) (map[int64]int64, error) {
	locked := make(map[int64]int64, len(ids))
	for _, id := range ids {
		if quantity, ok := f.env.stock[id]; ok {
			locked[id] = quantity
		}
	}
	return locked, nil
}

func (f *fakePurchaseStock) ApplyDelta(_ context.Context, id int64, delta int64) error {
	quantity, ok := f.env.stock[id]
	if !ok {
		return domain.ErrNotFound
	}
	if quantity+delta < 0 {
		return domain.ErrInsufficientStock
	}
	f.env.stock[id] = quantity + delta
	f.env.products[id].StockQuantity = int(quantity + delta)
	return nil
}

func copyPurchaseOrder(order *domain.PurchaseOrder) *domain.PurchaseOrder {
	copied := *order
	return &copied
}

func copyOrderItem(item *domain.OrderItem) *domain.OrderItem {
	copied := *item
	return &copied
}

func copyOrderItems(items []*domain.OrderItem) []*domain.OrderItem {
	copied := make([]*domain.OrderItem, len(items))
	for i, item := range items {
		copied[i] = copyOrderItem(item)
	}
	return copied
}

func purchaseTestEnv(t *testing.T) (*fakePurchaseEnv, *PurchaseOrderService) {
	t.Helper()
	env := newFakePurchaseEnv()
	env.addProvider(1, "Provider One")
	env.addProvider(2, "Provider Two")
	env.addProduct(1, "SKU-1", "Hammer", 5)
	env.addProduct(2, "SKU-2", "Nails", 10)
	return env, env.service()
}

func TestPurchaseOrderCreateValidation(t *testing.T) {
	_, service := purchaseTestEnv(t)
	cases := []struct {
		name  string
		input inbound.CreatePurchaseOrderInput
		want  error
	}{
		{"no items", inbound.CreatePurchaseOrderInput{ProviderID: 1}, domain.ErrValidation},
		{"duplicate product", inbound.CreatePurchaseOrderInput{ProviderID: 1, Items: []inbound.CreateOrderItemInput{
			{ProductID: 1, Quantity: 1, UnitCost: decimal.Zero},
			{ProductID: 1, Quantity: 1, UnitCost: decimal.Zero},
		}}, domain.ErrValidation},
		{"zero quantity", inbound.CreatePurchaseOrderInput{ProviderID: 1, Items: []inbound.CreateOrderItemInput{
			{ProductID: 1, Quantity: 0, UnitCost: decimal.Zero},
		}}, domain.ErrValidation},
		{"negative cost", inbound.CreatePurchaseOrderInput{ProviderID: 1, Items: []inbound.CreateOrderItemInput{
			{ProductID: 1, Quantity: 1, UnitCost: decimal.RequireFromString("-1")},
		}}, domain.ErrValidation},
		{"long sku", inbound.CreatePurchaseOrderInput{ProviderID: 1, Items: []inbound.CreateOrderItemInput{
			{ProductID: 1, Quantity: 1, UnitCost: decimal.Zero, SupplierSKU: purchaseString(strings.Repeat("x", 101))},
		}}, domain.ErrValidation},
		{"invalid status", inbound.CreatePurchaseOrderInput{ProviderID: 1, Status: "unknown", Items: []inbound.CreateOrderItemInput{
			{ProductID: 1, Quantity: 1, UnitCost: decimal.Zero},
		}}, domain.ErrValidation},
		{"invalid payment", inbound.CreatePurchaseOrderInput{ProviderID: 1, PaymentStatus: "unknown", Items: []inbound.CreateOrderItemInput{
			{ProductID: 1, Quantity: 1, UnitCost: decimal.Zero},
		}}, domain.ErrValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Create(context.Background(), tc.input); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}

	var missingProvider *domain.ProviderNotFoundError
	if _, err := service.Create(context.Background(), inbound.CreatePurchaseOrderInput{
		ProviderID: 99,
		Items:      []inbound.CreateOrderItemInput{{ProductID: 1, Quantity: 1, UnitCost: decimal.Zero}},
	}); !errors.As(err, &missingProvider) || missingProvider.ID != 99 {
		t.Fatalf("missing provider error = %v", err)
	}

	var missingProduct *domain.NotFoundError
	if _, err := service.Create(context.Background(), inbound.CreatePurchaseOrderInput{
		ProviderID: 1,
		Items:      []inbound.CreateOrderItemInput{{ProductID: 99, Quantity: 1, UnitCost: decimal.Zero}},
	}); !errors.As(err, &missingProduct) || missingProduct.Entity != "Product" || missingProduct.ID != 99 {
		t.Fatalf("missing product error = %v", err)
	}
}

func TestPurchaseOrderCreateTotalsDefaultsAndReceivedGap(t *testing.T) {
	env, service := purchaseTestEnv(t)
	before := env.stock[1]

	order, err := service.Create(context.Background(), inbound.CreatePurchaseOrderInput{
		ProviderID:  1,
		CreatedByID: 7,
		Items: []inbound.CreateOrderItemInput{
			{ProductID: 1, Quantity: 2, UnitCost: decimal.RequireFromString("10.25"), SupplierSKU: purchaseString("PROV-1")},
			{ProductID: 2, Quantity: 1, UnitCost: decimal.RequireFromString("0.50")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != domain.OrderStatusPending || order.PaymentStatus != domain.PurchasePaymentStatusPending {
		t.Fatalf("statuses = %q/%q", order.Status, order.PaymentStatus)
	}
	if !order.TotalAmount.Equal(decimal.RequireFromString("21.00")) {
		t.Fatalf("total = %s, want 21.00", order.TotalAmount)
	}
	if order.CreatedByID != 7 || order.ProviderID != 1 || order.Provider == nil || order.Provider.Name != "Provider One" {
		t.Fatalf("order = %+v", order)
	}
	if time.Since(order.OrderDate) > time.Minute {
		t.Fatalf("order_date = %v, want now", order.OrderDate)
	}
	items, err := service.ListItems(context.Background(), order.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(items.Items) != 2 || !items.Items[0].Subtotal.Equal(decimal.RequireFromString("20.50")) ||
		items.Items[0].SupplierSKU == nil || *items.Items[0].SupplierSKU != "PROV-1" {
		t.Fatalf("items = %+v", items.Items)
	}

	received, err := service.Create(context.Background(), inbound.CreatePurchaseOrderInput{
		ProviderID: 1,
		Status:     domain.OrderStatusReceived,
		Items:      []inbound.CreateOrderItemInput{{ProductID: 1, Quantity: 3, UnitCost: decimal.RequireFromString("2")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if received.Status != domain.OrderStatusReceived {
		t.Fatalf("status = %q", received.Status)
	}
	if env.stock[1] != before {
		t.Fatalf("stock = %d, want unchanged %d", env.stock[1], before)
	}
	if len(env.catalogs) != 0 {
		t.Fatalf("catalogs = %+v, want none for create-received gap", env.catalogs)
	}
}

func TestPurchaseOrderReceiveIncrementsStockAndUpsertsCatalog(t *testing.T) {
	env, service := purchaseTestEnv(t)
	order, err := service.Create(context.Background(), inbound.CreatePurchaseOrderInput{
		ProviderID: 1,
		Items: []inbound.CreateOrderItemInput{
			{ProductID: 2, Quantity: 4, UnitCost: decimal.RequireFromString("3.25")},
			{ProductID: 1, Quantity: 2, UnitCost: decimal.RequireFromString("10.25"), SupplierSKU: purchaseString("NEW-SKU")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Make the lead time predictable without depending on real elapsed days.
	env.orders[order.ID].OrderDate = time.Now().UTC().Add(-50 * time.Hour)

	status := domain.OrderStatusReceived
	updated, err := service.Update(context.Background(), order.ID, inbound.UpdatePurchaseOrderInput{
		Status: inbound.Optional[string]{Set: true, Value: &status},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.OrderStatusReceived {
		t.Fatalf("status = %q", updated.Status)
	}
	if env.stock[1] != 7 || env.stock[2] != 14 {
		t.Fatalf("stock = %d/%d, want 7/14", env.stock[1], env.stock[2])
	}
	catalog := env.catalog(1, 1)
	if catalog == nil || !catalog.LastQuotedCost.Equal(decimal.RequireFromString("10.25")) ||
		catalog.SupplierSKU == nil || *catalog.SupplierSKU != "NEW-SKU" ||
		catalog.LeadTime == nil || *catalog.LeadTime != 2 {
		t.Fatalf("catalog = %+v", catalog)
	}

	// Repeating the received transition is a no-op.
	var providerID int64 = 1
	updated, err = service.Update(context.Background(), order.ID, inbound.UpdatePurchaseOrderInput{
		Status:     inbound.Optional[string]{Set: true, Value: &status},
		ProviderID: inbound.Optional[int64]{Set: true, Value: &providerID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.OrderStatusReceived || env.stock[1] != 7 || env.stock[2] != 14 {
		t.Fatalf("idempotent receive changed state: status %q stock %d/%d", updated.Status, env.stock[1], env.stock[2])
	}
	if len(env.catalogs) != 2 {
		t.Fatalf("catalogs = %d, want 2", len(env.catalogs))
	}

	// Empty supplier SKU keeps the catalog value stored by the previous receive.
	env.catalogs[0].SupplierSKU = purchaseString("KEEP")
	sku := ""
	itemID := env.items[order.ID][0].ID
	if _, err := service.UpdateItem(context.Background(), order.ID, itemID, inbound.UpdateOrderItemInput{
		Quantity:    inbound.Optional[int]{Set: true, Value: purchaseInt(5)},
		SupplierSKU: inbound.Optional[string]{Set: true, Value: &sku},
	}); err != nil {
		t.Fatal(err)
	}
	status2 := domain.OrderStatusSent
	status3 := domain.OrderStatusReceived
	if _, err := service.Update(context.Background(), order.ID, inbound.UpdatePurchaseOrderInput{
		Status: inbound.Optional[string]{Set: true, Value: &status2},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), order.ID, inbound.UpdatePurchaseOrderInput{
		Status: inbound.Optional[string]{Set: true, Value: &status3},
	}); err != nil {
		t.Fatal(err)
	}
	if env.stock[2] != 19 || env.stock[1] != 9 {
		t.Fatalf("second receive stock = %d/%d, want 9/19", env.stock[1], env.stock[2])
	}
	if got := env.catalog(2, 1); got == nil || got.SupplierSKU == nil || *got.SupplierSKU != "KEEP" {
		t.Fatalf("catalog without sku = %+v", got)
	}
	if got := env.catalog(1, 1); got == nil || got.SupplierSKU == nil || *got.SupplierSKU != "NEW-SKU" {
		t.Fatalf("catalog with sku = %+v", got)
	}
}

func TestPurchaseOrderUpdateValidationAndProvider(t *testing.T) {
	env, service := purchaseTestEnv(t)
	order, err := service.Create(context.Background(), inbound.CreatePurchaseOrderInput{
		ProviderID: 1,
		Items:      []inbound.CreateOrderItemInput{{ProductID: 1, Quantity: 1, UnitCost: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		input inbound.UpdatePurchaseOrderInput
	}{
		{"null provider", inbound.UpdatePurchaseOrderInput{ProviderID: inbound.Optional[int64]{Set: true}}},
		{"zero provider", inbound.UpdatePurchaseOrderInput{ProviderID: inbound.Optional[int64]{Set: true, Value: purchaseInt64(0)}}},
		{"null status", inbound.UpdatePurchaseOrderInput{Status: inbound.Optional[string]{Set: true}}},
		{"invalid status", inbound.UpdatePurchaseOrderInput{Status: inbound.Optional[string]{Set: true, Value: purchaseString("nope")}}},
		{"null payment", inbound.UpdatePurchaseOrderInput{PaymentStatus: inbound.Optional[string]{Set: true}}},
		{"invalid payment", inbound.UpdatePurchaseOrderInput{PaymentStatus: inbound.Optional[string]{Set: true, Value: purchaseString("nope")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Update(context.Background(), order.ID, tc.input); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want validation", err)
			}
		})
	}

	var missingProvider *domain.ProviderNotFoundError
	if _, err := service.Update(context.Background(), order.ID, inbound.UpdatePurchaseOrderInput{
		ProviderID: inbound.Optional[int64]{Set: true, Value: purchaseInt64(99)},
	}); !errors.As(err, &missingProvider) || missingProvider.ID != 99 {
		t.Fatalf("missing provider error = %v", err)
	}

	paid := domain.PurchasePaymentStatusPaid
	providerID := int64(2)
	updated, err := service.Update(context.Background(), order.ID, inbound.UpdatePurchaseOrderInput{
		ProviderID:    inbound.Optional[int64]{Set: true, Value: &providerID},
		PaymentStatus: inbound.Optional[string]{Set: true, Value: &paid},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ProviderID != 2 || updated.Provider.Name != "Provider Two" || updated.PaymentStatus != domain.PurchasePaymentStatusPaid {
		t.Fatalf("updated = %+v", updated)
	}
	if env.stock[1] != 5 {
		t.Fatalf("stock = %d, want unchanged 5", env.stock[1])
	}
}

func TestPurchaseOrderItemMutations(t *testing.T) {
	env, service := purchaseTestEnv(t)
	order, err := service.Create(context.Background(), inbound.CreatePurchaseOrderInput{
		ProviderID: 1,
		Items:      []inbound.CreateOrderItemInput{{ProductID: 1, Quantity: 2, UnitCost: decimal.RequireFromString("10")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !order.TotalAmount.Equal(decimal.RequireFromString("20")) {
		t.Fatalf("initial total = %s", order.TotalAmount)
	}

	added, err := service.AddItem(context.Background(), order.ID, inbound.AddOrderItemInput{
		ProductID: 2, Quantity: 3, UnitCost: decimal.RequireFromString("1.25"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !added.TotalAmount.Equal(decimal.RequireFromString("23.75")) {
		t.Fatalf("total after add = %s", added.TotalAmount)
	}
	if env.stock[2] != 10 {
		t.Fatalf("stock changed on add = %d", env.stock[2])
	}

	if _, err := service.AddItem(context.Background(), order.ID, inbound.AddOrderItemInput{
		ProductID: 2, Quantity: 1, UnitCost: decimal.RequireFromString("1"),
	}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate error = %v, want conflict", err)
	}

	items, err := service.ListItems(context.Background(), order.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	secondID := items.Items[1].ID
	updated, err := service.UpdateItem(context.Background(), order.ID, secondID, inbound.UpdateOrderItemInput{
		Quantity: inbound.Optional[int]{Set: true, Value: purchaseInt(1)},
		UnitCost: inbound.Optional[decimal.Decimal]{Set: true, Value: purchaseDecimal("2.50")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.TotalAmount.Equal(decimal.RequireFromString("22.50")) {
		t.Fatalf("total after update = %s", updated.TotalAmount)
	}

	updated, err = service.DeleteItem(context.Background(), order.ID, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.TotalAmount.Equal(decimal.RequireFromString("20")) {
		t.Fatalf("total after delete = %s", updated.TotalAmount)
	}

	var missingItem *domain.NotFoundError
	if _, err := service.DeleteItem(context.Background(), order.ID, secondID); !errors.As(err, &missingItem) || missingItem.Entity != "Order item" {
		t.Fatalf("missing item error = %v, want Order item not found", err)
	}

	// Received item mutations for legacy parity do not touch stock or catalogs.
	status := domain.OrderStatusReceived
	if _, err := service.Update(context.Background(), order.ID, inbound.UpdatePurchaseOrderInput{
		Status: inbound.Optional[string]{Set: true, Value: &status},
	}); err != nil {
		t.Fatal(err)
	}
	stockAfterReceive := env.stock[1]
	catalogCount := len(env.catalogs)
	firstID := env.items[order.ID][0].ID
	if _, err := service.UpdateItem(context.Background(), order.ID, firstID, inbound.UpdateOrderItemInput{
		Quantity: inbound.Optional[int]{Set: true, Value: purchaseInt(5)},
	}); err != nil {
		t.Fatal(err)
	}
	if env.stock[1] != stockAfterReceive || len(env.catalogs) != catalogCount {
		t.Fatalf("received item mutation changed stock/catalog: stock %d catalogs %d", env.stock[1], len(env.catalogs))
	}
}

func TestPurchaseOrderDeleteNeverReversesReceivedStock(t *testing.T) {
	env, service := purchaseTestEnv(t)
	order, err := service.Create(context.Background(), inbound.CreatePurchaseOrderInput{
		ProviderID: 1,
		Items:      []inbound.CreateOrderItemInput{{ProductID: 1, Quantity: 2, UnitCost: decimal.RequireFromString("5")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status := domain.OrderStatusReceived
	if _, err := service.Update(context.Background(), order.ID, inbound.UpdatePurchaseOrderInput{
		Status: inbound.Optional[string]{Set: true, Value: &status},
	}); err != nil {
		t.Fatal(err)
	}
	stockAfterReceive := env.stock[1]

	if _, err := service.Delete(context.Background(), order.ID); err != nil {
		t.Fatal(err)
	}
	if env.stock[1] != stockAfterReceive {
		t.Fatalf("stock after delete = %d, want %d", env.stock[1], stockAfterReceive)
	}
	if _, err := service.Get(context.Background(), order.ID); err == nil {
		t.Fatal("deleted order still exists")
	}
}

func purchaseString(value string) *string { return &value }

func purchaseInt(value int) *int { return &value }

func purchaseInt64(value int64) *int64 { return &value }

func purchaseDecimal(value string) *decimal.Decimal {
	parsed := decimal.RequireFromString(value)
	return &parsed
}
