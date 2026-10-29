package inbound

import (
	"context"

	"inventory-desktop/internal/domain"
)

type DashboardUseCase interface {
	Summary(ctx context.Context) (*domain.DashboardSummary, error)
	SalesTrend(ctx context.Context) ([]domain.SalesTrendPoint, error)
	TopProducts(ctx context.Context, limit int) ([]domain.TopProduct, error)
	OrderStatusDistribution(ctx context.Context) ([]domain.OrderStatusCount, error)
}
