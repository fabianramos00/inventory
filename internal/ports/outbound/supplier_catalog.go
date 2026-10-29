package outbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type SupplierCatalogRepository interface {
	List(ctx context.Context, skip, limit int, providerID, productID *int64) ([]*domain.SupplierCatalog, bool, error)
	Get(ctx context.Context, id int64) (*domain.SupplierCatalog, error)
}
