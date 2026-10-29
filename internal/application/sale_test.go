package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type fakeSaleEnv struct {
	sales        map[int64]*domain.Sale
	items        map[int64][]*domain.SaleItem
	stock        map[int64]int64
	products     map[int64]*domain.ProductDetail
	clients      map[int64]*domain.Client
	users        map[int64]*domain.User
	nextSaleID   int64
	nextItemID   int64
	createdSales int
}

func newFakeSaleEnv() *fakeSaleEnv {
	return &fakeSaleEnv{
		sales:    map[int64]*domain.Sale{},
		items:    map[int64][]*domain.SaleItem{},
		stock:    map[int64]int64{},
		products: map[int64]*domain.ProductDetail{},
		clients:  map[int64]*domain.Client{},
		users:    map[int64]*domain.User{},
	}
}

func (e *fakeSaleEnv) addProduct(id int64, sku string, price string, stock int64) {
	e.products[id] = &domain.ProductDetail{Product: domain.Product{
		ID: id, SKU: sku, Name: "Product " + sku, Price: decimal.RequireFromString(price),
		Cost: decimal.RequireFromString("1"), StockQuantity: int(stock),
	}}
	e.stock[id] = stock
}

func (e *fakeSaleEnv) addClient(id int64, name string) {
	e.clients[id] = &domain.Client{ID: id, Name: name}
}

func (e *fakeSaleEnv) addUser(id int64, email string) {
	e.users[id] = &domain.User{ID: id, FullName: "User " + email, Email: email, IsActive: true}
}

func (e *fakeSaleEnv) service() *SaleService {
	return NewSaleService(&fakeSaleRepo{env: e}, &fakeUnitOfWork{env: e})
}

type fakeUnitOfWork struct {
	env *fakeSaleEnv
}

func (u *fakeUnitOfWork) WithinTx(ctx context.Context, fn func(context.Context, outbound.TxRepos) error) error {
	snapshot := u.env.snapshot()
	err := fn(ctx, outbound.TxRepos{
		Sales:    &fakeSaleRepo{env: u.env},
		Products: &fakeProductRepo{env: u.env},
		Clients:  &fakeClientRepo{env: u.env},
		Stock:    &fakeStockRepo{env: u.env},
		// Sale use cases never touch the purchase-order stores.
		Orders:    nil,
		Catalogs:  nil,
		Providers: nil,
	})
	if err != nil {
		u.env.restore(snapshot)
	}
	return err
}

type saleEnvSnapshot struct {
	sales        map[int64]*domain.Sale
	items        map[int64][]*domain.SaleItem
	stock        map[int64]int64
	nextSaleID   int64
	nextItemID   int64
	createdSales int
}

func (e *fakeSaleEnv) snapshot() saleEnvSnapshot {
	snapshot := saleEnvSnapshot{
		sales:        make(map[int64]*domain.Sale, len(e.sales)),
		items:        make(map[int64][]*domain.SaleItem, len(e.items)),
		stock:        make(map[int64]int64, len(e.stock)),
		nextSaleID:   e.nextSaleID,
		nextItemID:   e.nextItemID,
		createdSales: e.createdSales,
	}
	for id, sale := range e.sales {
		copied := *sale
		snapshot.sales[id] = &copied
	}
	for id, items := range e.items {
		copied := make([]*domain.SaleItem, len(items))
		for i, item := range items {
			cloned := *item
			copied[i] = &cloned
		}
		snapshot.items[id] = copied
	}
	for id, quantity := range e.stock {
		snapshot.stock[id] = quantity
	}
	return snapshot
}

func (e *fakeSaleEnv) restore(snapshot saleEnvSnapshot) {
	e.sales = snapshot.sales
	e.items = snapshot.items
	e.stock = snapshot.stock
	e.nextSaleID = snapshot.nextSaleID
	e.nextItemID = snapshot.nextItemID
	e.createdSales = snapshot.createdSales
}

type fakeSaleRepo struct {
	env *fakeSaleEnv
}

var _ outbound.SaleRepository = (*fakeSaleRepo)(nil)

func (r *fakeSaleRepo) List(_ context.Context, filter outbound.SaleFilter) ([]*domain.Sale, bool, error) {
	items := make([]*domain.Sale, 0, len(r.env.sales))
	for id := int64(1); id <= r.env.nextSaleID; id++ {
		if sale, ok := r.env.sales[id]; ok {
			items = append(items, r.copySale(sale))
		}
	}
	return items, false, nil
}

func (r *fakeSaleRepo) Get(_ context.Context, id int64) (*domain.Sale, error) {
	sale, ok := r.env.sales[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return r.copySale(sale), nil
}

func (r *fakeSaleRepo) Stats(context.Context) (*domain.SaleStats, error) {
	stats := &domain.SaleStats{}
	stats.TotalAmountSum = decimal.Zero
	stats.AmountPaidSum = decimal.Zero
	for _, sale := range r.env.sales {
		if sale.PaymentStatus != domain.PaymentStatusPaid {
			stats.UnpaidCount++
		}
		if sale.DeliveryStatus != domain.DeliveryStatusDelivered {
			stats.UndeliveredCount++
		}
		stats.TotalAmountSum = stats.TotalAmountSum.Add(sale.TotalAmount)
		stats.AmountPaidSum = stats.AmountPaidSum.Add(sale.AmountPaid)
	}
	return stats, nil
}

func (r *fakeSaleRepo) Create(_ context.Context, sale *domain.Sale, items []*domain.SaleItem) (*domain.Sale, error) {
	r.env.nextSaleID++
	sale.ID = r.env.nextSaleID
	r.env.sales[sale.ID] = r.copySale(sale)
	for _, item := range items {
		item.SaleID = sale.ID
		if _, err := r.CreateItem(context.Background(), item); err != nil {
			return nil, err
		}
	}
	r.env.createdSales++
	return r.copySale(sale), nil
}

func (r *fakeSaleRepo) Update(_ context.Context, id int64, sale *domain.Sale) error {
	if _, ok := r.env.sales[id]; !ok {
		return domain.ErrNotFound
	}
	r.env.sales[id] = r.copySale(sale)
	return nil
}

func (r *fakeSaleRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.env.sales[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.env.sales, id)
	delete(r.env.items, id)
	return nil
}

func (r *fakeSaleRepo) ListItems(_ context.Context, saleID int64, skip, limit int) ([]*domain.SaleItem, bool, error) {
	items := r.env.items[saleID]
	if skip >= len(items) {
		return nil, false, nil
	}
	items = items[skip:]
	hasNext := len(items) > limit
	if hasNext {
		items = items[:limit]
	}
	return r.copyItems(items), hasNext, nil
}

func (r *fakeSaleRepo) ListAllItems(_ context.Context, saleID int64) ([]*domain.SaleItem, error) {
	return r.copyItems(r.env.items[saleID]), nil
}

func (r *fakeSaleRepo) GetItem(_ context.Context, saleID, itemID int64) (*domain.SaleItem, error) {
	for _, item := range r.env.items[saleID] {
		if item.ID == itemID {
			cloned := *item
			return &cloned, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeSaleRepo) GetItemByProduct(_ context.Context, saleID, productID int64) (*domain.SaleItem, error) {
	for _, item := range r.env.items[saleID] {
		if item.ProductID == productID {
			cloned := *item
			return &cloned, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *fakeSaleRepo) CreateItem(_ context.Context, item *domain.SaleItem) (*domain.SaleItem, error) {
	r.env.nextItemID++
	item.ID = r.env.nextItemID
	if product, ok := r.env.products[item.ProductID]; ok {
		item.ProductName = product.Name
		item.ProductSKU = product.SKU
	}
	cloned := *item
	r.env.items[item.SaleID] = append(r.env.items[item.SaleID], &cloned)
	return &cloned, nil
}

func (r *fakeSaleRepo) UpdateItem(_ context.Context, item *domain.SaleItem) error {
	items := r.env.items[item.SaleID]
	for i, existing := range items {
		if existing.ID == item.ID {
			cloned := *item
			items[i] = &cloned
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeSaleRepo) DeleteItem(_ context.Context, saleID, itemID int64) error {
	items := r.env.items[saleID]
	for i, item := range items {
		if item.ID == itemID {
			r.env.items[saleID] = append(items[:i], items[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *fakeSaleRepo) DeleteItems(_ context.Context, saleID int64) error {
	delete(r.env.items, saleID)
	return nil
}

func (r *fakeSaleRepo) copySale(sale *domain.Sale) *domain.Sale {
	copied := *sale
	copied.Client = nil
	copied.CreatedBy = nil
	if sale.ClientID != nil {
		if client, ok := r.env.clients[*sale.ClientID]; ok {
			clientCopy := *client
			copied.Client = &clientCopy
		}
	}
	if user, ok := r.env.users[sale.CreatedByID]; ok {
		userCopy := *user
		copied.CreatedBy = &userCopy
	}
	return &copied
}

func (r *fakeSaleRepo) copyItems(items []*domain.SaleItem) []*domain.SaleItem {
	result := make([]*domain.SaleItem, len(items))
	for i, item := range items {
		cloned := *item
		result[i] = &cloned
	}
	return result
}

type fakeProductRepo struct {
	env *fakeSaleEnv
}

func (r *fakeProductRepo) List(context.Context, outbound.ProductFilter) ([]*domain.ProductDetail, bool, error) {
	return nil, false, nil
}

func (r *fakeProductRepo) Get(_ context.Context, id int64) (*domain.ProductDetail, error) {
	product, ok := r.env.products[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copied := *product
	return &copied, nil
}

func (r *fakeProductRepo) Create(context.Context, *domain.Product) (*domain.Product, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeProductRepo) Update(context.Context, int64, *domain.Product) error {
	return errors.New("not implemented")
}

func (r *fakeProductRepo) Delete(context.Context, int64) (*domain.Product, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeProductRepo) Stats(context.Context) (*domain.ProductStats, error) {
	return nil, errors.New("not implemented")
}

type fakeClientRepo struct {
	env *fakeSaleEnv
}

func (r *fakeClientRepo) List(context.Context, int, int, string) ([]*domain.Client, bool, error) {
	return nil, false, nil
}

func (r *fakeClientRepo) Get(_ context.Context, id int64) (*domain.Client, error) {
	client, ok := r.env.clients[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copied := *client
	return &copied, nil
}

func (r *fakeClientRepo) Create(context.Context, *domain.Client) (*domain.Client, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeClientRepo) Update(context.Context, int64, *domain.Client) error {
	return errors.New("not implemented")
}

func (r *fakeClientRepo) Delete(context.Context, int64) (*domain.Client, error) {
	return nil, errors.New("not implemented")
}

type fakeStockRepo struct {
	env *fakeSaleEnv
}

func (r *fakeStockRepo) LockForUpdate(_ context.Context, ids []int64) (map[int64]int64, error) {
	result := map[int64]int64{}
	for _, id := range ids {
		if quantity, ok := r.env.stock[id]; ok {
			result[id] = quantity
		}
	}
	return result, nil
}

func (r *fakeStockRepo) ApplyDelta(_ context.Context, id int64, delta int64) error {
	next := r.env.stock[id] + delta
	if next < 0 {
		return domain.ErrInsufficientStock
	}
	r.env.stock[id] = next
	return nil
}

func requireSaleTime(t *testing.T, value time.Time) {
	t.Helper()
	if value.IsZero() {
		t.Fatal("timestamp is zero")
	}
	if value.Location() != time.UTC {
		t.Fatalf("timestamp location = %s, want UTC", value.Location())
	}
}
