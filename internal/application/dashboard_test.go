package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
)

type fakeDashboardRepository struct {
	summary         *domain.DashboardSummary
	summaryErr      error
	summaryToday    time.Time
	trend           []domain.SalesTrendPoint
	trendErr        error
	trendStart      time.Time
	trendEnd        time.Time
	top             []domain.TopProduct
	topErr          error
	topSince        time.Time
	topLimit        int
	distribution    []domain.OrderStatusCount
	distributionErr error
}

func (f *fakeDashboardRepository) Summary(_ context.Context, todayStart time.Time) (*domain.DashboardSummary, error) {
	f.summaryToday = todayStart
	if f.summaryErr != nil {
		return nil, f.summaryErr
	}
	if f.summary == nil {
		return &domain.DashboardSummary{}, nil
	}
	return f.summary, nil
}

func (f *fakeDashboardRepository) SalesTrend(_ context.Context, start, end time.Time) ([]domain.SalesTrendPoint, error) {
	f.trendStart, f.trendEnd = start, end
	return f.trend, f.trendErr
}

func (f *fakeDashboardRepository) TopProducts(_ context.Context, since time.Time, limit int) ([]domain.TopProduct, error) {
	f.topSince, f.topLimit = since, limit
	return f.top, f.topErr
}

func (f *fakeDashboardRepository) OrderStatusDistribution(_ context.Context) ([]domain.OrderStatusCount, error) {
	return f.distribution, f.distributionErr
}

func dashboardServiceAt(repository *fakeDashboardRepository, now time.Time) *DashboardService {
	service := NewDashboardService(repository)
	service.now = func() time.Time { return now }
	return service
}

func TestDashboardSummaryUsesBogotaDayStart(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "afternoon",
			now:  time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC),
			want: time.Date(2026, time.October, 24, 5, 0, 0, 0, time.UTC),
		},
		{
			name: "before Bogota midnight",
			now:  time.Date(2026, time.October, 24, 4, 30, 0, 0, time.UTC),
			want: time.Date(2026, time.October, 23, 5, 0, 0, 0, time.UTC),
		},
		{
			name: "exactly Bogota midnight",
			now:  time.Date(2026, time.October, 24, 5, 0, 0, 0, time.UTC),
			want: time.Date(2026, time.October, 24, 5, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeDashboardRepository{summary: &domain.DashboardSummary{
				InventoryValue:      decimal.RequireFromString("40.00"),
				TotalSalesToday:     decimal.RequireFromString("28.50"),
				PendingPaymentSales: decimal.RequireFromString("20.00"),
				PendingOrders:       3,
				DeliveredOrders:     1,
			}}
			summary, err := dashboardServiceAt(repository, tt.now).Summary(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !repository.summaryToday.Equal(tt.want) {
				t.Fatalf("todayStart = %s, want %s", repository.summaryToday, tt.want)
			}
			if !summary.InventoryValue.Equal(decimal.RequireFromString("40.00")) ||
				!summary.TotalSalesToday.Equal(decimal.RequireFromString("28.50")) ||
				!summary.PendingPaymentSales.Equal(decimal.RequireFromString("20.00")) ||
				summary.PendingOrders != 3 || summary.DeliveredOrders != 1 {
				t.Fatalf("summary = %+v", summary)
			}
		})
	}
}

func TestDashboardSalesTrendZeroFillsSevenDays(t *testing.T) {
	now := time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)
	repository := &fakeDashboardRepository{trend: []domain.SalesTrendPoint{
		{Date: "2026-10-23", SalesAmount: decimal.RequireFromString("300.00"), OrdersCost: decimal.RequireFromString("90.00")},
		{Date: "2026-10-24", SalesAmount: decimal.RequireFromString("28.50"), OrdersCost: decimal.RequireFromString("60.00")},
	}}

	points, err := dashboardServiceAt(repository, now).SalesTrend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, time.October, 18, 5, 0, 0, 0, time.UTC); !repository.trendStart.Equal(want) {
		t.Fatalf("trend start = %s, want %s", repository.trendStart, want)
	}
	if want := time.Date(2026, time.October, 25, 5, 0, 0, 0, time.UTC); !repository.trendEnd.Equal(want) {
		t.Fatalf("trend end = %s, want %s", repository.trendEnd, want)
	}
	if len(points) != 7 {
		t.Fatalf("trend length = %d, want 7", len(points))
	}
	wantDates := []string{"2026-10-18", "2026-10-19", "2026-10-20", "2026-10-21", "2026-10-22", "2026-10-23", "2026-10-24"}
	for i, want := range wantDates {
		if points[i].Date != want {
			t.Fatalf("day %d = %q, want %q", i, points[i].Date, want)
		}
	}
	for i := 0; i < 5; i++ {
		if !points[i].SalesAmount.IsZero() || !points[i].OrdersCost.IsZero() {
			t.Fatalf("day %d = %+v, want zeros", i, points[i])
		}
	}
	if !points[5].SalesAmount.Equal(decimal.RequireFromString("300.00")) ||
		!points[5].OrdersCost.Equal(decimal.RequireFromString("90.00")) {
		t.Fatalf("yesterday = %+v", points[5])
	}
	if !points[6].SalesAmount.Equal(decimal.RequireFromString("28.50")) ||
		!points[6].OrdersCost.Equal(decimal.RequireFromString("60.00")) {
		t.Fatalf("today = %+v", points[6])
	}
}

func TestDashboardSalesTrendEmptyRepositoryReturnsSevenZeroDays(t *testing.T) {
	now := time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)
	points, err := dashboardServiceAt(&fakeDashboardRepository{}, now).SalesTrend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 7 {
		t.Fatalf("trend length = %d, want 7", len(points))
	}
	for _, point := range points {
		if !point.SalesAmount.IsZero() || !point.OrdersCost.IsZero() {
			t.Fatalf("point = %+v, want zeros", point)
		}
	}
}

func TestDashboardTopProductsLimitAndWindow(t *testing.T) {
	now := time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)
	repository := &fakeDashboardRepository{}

	for _, limit := range []int{0, -1, 101} {
		if _, err := dashboardServiceAt(repository, now).TopProducts(context.Background(), limit); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("limit %d error = %v, want ErrValidation", limit, err)
		}
	}

	products, err := dashboardServiceAt(repository, now).TopProducts(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if products == nil {
		t.Fatal("products = nil, want an empty slice")
	}
	if repository.topLimit != 10 {
		t.Fatalf("repository limit = %d, want 10", repository.topLimit)
	}
	if want := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.UTC); !repository.topSince.Equal(want) {
		t.Fatalf("top products since = %s, want %s", repository.topSince, want)
	}
}

func TestDashboardOrderStatusDistribution(t *testing.T) {
	repository := &fakeDashboardRepository{distribution: []domain.OrderStatusCount{
		{Status: "pending", Count: 3},
		{Status: "sent", Count: 2},
		{Status: "received", Count: 1},
	}}
	rows, err := dashboardServiceAt(repository, time.Now()).OrderStatusDistribution(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	want := []float64{50, 33.33, 16.67}
	for i, percentage := range want {
		if rows[i].Percentage != percentage {
			t.Fatalf("row %d percentage = %v, want %v", i, rows[i].Percentage, percentage)
		}
	}

	empty, err := dashboardServiceAt(&fakeDashboardRepository{}, time.Now()).OrderStatusDistribution(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("empty distribution = %#v, want an empty slice", empty)
	}
}
