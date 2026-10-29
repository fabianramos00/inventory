package inbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type SupplierCatalogFilter struct {
	Skip       int
	Limit      int
	ProviderID *int64
	ProductID  *int64
}

type SupplierCatalogList struct {
	Items   []*domain.SupplierCatalog
	HasNext bool
	Skip    int
	Limit   int
}

type SupplierCatalogUseCase interface {
	List(ctx context.Context, filter SupplierCatalogFilter) (*SupplierCatalogList, error)
	Get(ctx context.Context, id int64) (*domain.SupplierCatalog, error)
}
