package application

import (
	"context"
	"fmt"
	"math"
	"time"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

const dashboardDays = 7

// Colombia has no DST, so a fixed offset keeps Bogota calendar days stable
// without depending on tzdata being installed on the host.
var bogotaLocation = time.FixedZone("America/Bogota", -5*60*60)

type DashboardService struct {
	repository outbound.DashboardRepository
	now        func() time.Time
}

func NewDashboardService(repository outbound.DashboardRepository) *DashboardService {
	return &DashboardService{repository: repository, now: time.Now}
}

var _ inbound.DashboardUseCase = (*DashboardService)(nil)

func (s *DashboardService) Summary(ctx context.Context) (*domain.DashboardSummary, error) {
	today := bogotaDayStart(s.now())
	return s.repository.Summary(ctx, today.UTC())
}

func (s *DashboardService) SalesTrend(ctx context.Context) ([]domain.SalesTrendPoint, error) {
	today := bogotaDayStart(s.now())
	start := today.AddDate(0, 0, -(dashboardDays - 1))
	end := today.AddDate(0, 0, 1)

	points, err := s.repository.SalesTrend(ctx, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	byDate := make(map[string]domain.SalesTrendPoint, len(points))
	for _, point := range points {
		byDate[point.Date] = point
	}

	result := make([]domain.SalesTrendPoint, 0, dashboardDays)
	for i := 0; i < dashboardDays; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		point, ok := byDate[date]
		if !ok {
			point = domain.SalesTrendPoint{Date: date}
		}
		result = append(result, point)
	}
	return result, nil
}

func (s *DashboardService) TopProducts(ctx context.Context, limit int) ([]domain.TopProduct, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 100", domain.ErrValidation)
	}
	since := s.now().UTC().AddDate(0, 0, -dashboardDays)
	products, err := s.repository.TopProducts(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	if products == nil {
		return []domain.TopProduct{}, nil
	}
	return products, nil
}

func (s *DashboardService) OrderStatusDistribution(ctx context.Context) ([]domain.OrderStatusCount, error) {
	rows, err := s.repository.OrderStatusDistribution(ctx)
	if err != nil {
		return nil, err
	}
	var total int64
	for _, row := range rows {
		total += row.Count
	}
	result := make([]domain.OrderStatusCount, 0, len(rows))
	for _, row := range rows {
		if total > 0 {
			row.Percentage = math.Round(float64(row.Count)/float64(total)*10000) / 100
		}
		result = append(result, row)
	}
	return result, nil
}

func bogotaDayStart(now time.Time) time.Time {
	local := now.In(bogotaLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, bogotaLocation)
}
