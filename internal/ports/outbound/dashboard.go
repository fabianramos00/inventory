package outbound

import (
	"context"
	"time"

	"inventory-desktop/internal/domain"
)

type DashboardRepository interface {
	Summary(ctx context.Context, todayStart time.Time) (*domain.DashboardSummary, error)
	SalesTrend(ctx context.Context, start, end time.Time) ([]domain.SalesTrendPoint, error)
	TopProducts(ctx context.Context, since time.Time, limit int) ([]domain.TopProduct, error)
	OrderStatusDistribution(ctx context.Context) ([]domain.OrderStatusCount, error)
}
