package application

import (
	"context"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

type SupplierCatalogService struct {
	repository outbound.SupplierCatalogRepository
}

func NewSupplierCatalogService(repository outbound.SupplierCatalogRepository) *SupplierCatalogService {
	return &SupplierCatalogService{repository: repository}
}

var _ inbound.SupplierCatalogUseCase = (*SupplierCatalogService)(nil)

func (s *SupplierCatalogService) List(ctx context.Context, filter inbound.SupplierCatalogFilter) (*inbound.SupplierCatalogList, error) {
	items, hasNext, err := s.repository.List(ctx, filter.Skip, filter.Limit, filter.ProviderID, filter.ProductID)
	if err != nil {
		return nil, err
	}
	return &inbound.SupplierCatalogList{Items: items, HasNext: hasNext, Skip: filter.Skip, Limit: filter.Limit}, nil
}

func (s *SupplierCatalogService) Get(ctx context.Context, id int64) (*domain.SupplierCatalog, error) {
	return s.repository.Get(ctx, id)
}
