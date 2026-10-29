package application

import (
	"context"
	"errors"
	"testing"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
)

type fakeSupplierCatalogStore struct {
	listItems    []*domain.SupplierCatalog
	listHasNext  bool
	listErr      error
	listSkip     int
	listLimit    int
	listProvider *int64
	listProduct  *int64
	getResult    *domain.SupplierCatalog
	getErr       error
	getID        int64
}

func (f *fakeSupplierCatalogStore) List(_ context.Context, skip, limit int, providerID, productID *int64) ([]*domain.SupplierCatalog, bool, error) {
	f.listSkip, f.listLimit, f.listProvider, f.listProduct = skip, limit, providerID, productID
	return f.listItems, f.listHasNext, f.listErr
}

func (f *fakeSupplierCatalogStore) Get(_ context.Context, id int64) (*domain.SupplierCatalog, error) {
	f.getID = id
	return f.getResult, f.getErr
}

func TestSupplierCatalogListPassesFiltersAndPagination(t *testing.T) {
	providerID, productID := int64(2), int64(4)
	store := &fakeSupplierCatalogStore{
		listItems:   []*domain.SupplierCatalog{{ID: 9}},
		listHasNext: true,
	}
	service := NewSupplierCatalogService(store)

	result, err := service.List(context.Background(), inbound.SupplierCatalogFilter{
		Skip:       3,
		Limit:      7,
		ProviderID: &providerID,
		ProductID:  &productID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.listSkip != 3 || store.listLimit != 7 || store.listProvider != &providerID || store.listProduct != &productID {
		t.Fatalf("repository args = %d/%d/%v/%v", store.listSkip, store.listLimit, store.listProvider, store.listProduct)
	}
	if !result.HasNext || result.Skip != 3 || result.Limit != 7 || len(result.Items) != 1 {
		t.Fatalf("result = %+v", result)
	}

	want := errors.New("list failed")
	store.listErr = want
	if _, err := service.List(context.Background(), inbound.SupplierCatalogFilter{}); !errors.Is(err, want) {
		t.Fatalf("list error = %v, want %v", err, want)
	}
}

func TestSupplierCatalogGetPropagatesResultAndError(t *testing.T) {
	store := &fakeSupplierCatalogStore{getResult: &domain.SupplierCatalog{ID: 5}}
	service := NewSupplierCatalogService(store)

	if got, err := service.Get(context.Background(), 5); err != nil || got.ID != 5 || store.getID != 5 {
		t.Fatalf("get = %+v / %v id %d", got, err, store.getID)
	}
	store.getErr = domain.ErrNotFound
	if _, err := service.Get(context.Background(), 6); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get error = %v, want ErrNotFound", err)
	}
}
