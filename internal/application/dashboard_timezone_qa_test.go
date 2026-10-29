package application

import (
	"context"
	"testing"
	"time"

	"inventory-desktop/internal/domain"
)

// The service must derive the Bogota calendar day from the instant, never from
// the location attached to now(). Fixed offsets keep this test independent of
// host tzdata.
func TestDashboardUsesBogotaCalendarAcrossHostZones(t *testing.T) {
	instant := time.Date(2026, time.October, 24, 4, 30, 0, 0, time.UTC)
	zones := []struct {
		name string
		loc  *time.Location
	}{
		{"UTC", time.UTC},
		{"UTC+09", time.FixedZone("UTC+09", 9*60*60)},
		{"UTC-04", time.FixedZone("UTC-04", -4*60*60)},
		{"UTC+14", time.FixedZone("UTC+14", 14*60*60)},
	}

	for _, zone := range zones {
		t.Run(zone.name, func(t *testing.T) {
			repository := &fakeDashboardRepository{}
			service := dashboardServiceAt(repository, instant.In(zone.loc))

			if _, err := service.Summary(context.Background()); err != nil {
				t.Fatal(err)
			}
			wantToday := time.Date(2026, time.October, 23, 5, 0, 0, 0, time.UTC)
			if !repository.summaryToday.Equal(wantToday) {
				t.Fatalf("todayStart = %s, want %s", repository.summaryToday, wantToday)
			}

			points, err := service.SalesTrend(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(points) != dashboardDays {
				t.Fatalf("trend length = %d, want %d", len(points), dashboardDays)
			}
			if points[0].Date != "2026-10-17" || points[len(points)-1].Date != "2026-10-23" {
				t.Fatalf("trend dates = %s..%s, want 2026-10-17..2026-10-23",
					points[0].Date, points[len(points)-1].Date)
			}
			if want := time.Date(2026, time.October, 17, 5, 0, 0, 0, time.UTC); !repository.trendStart.Equal(want) {
				t.Fatalf("trend start = %s, want %s", repository.trendStart, want)
			}
			if want := time.Date(2026, time.October, 24, 5, 0, 0, 0, time.UTC); !repository.trendEnd.Equal(want) {
				t.Fatalf("trend end = %s, want %s", repository.trendEnd, want)
			}
		})
	}
}

func TestDashboardBogotaMidnightBoundary(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		wantToday time.Time
		wantLast  string
	}{
		{
			name:      "23:59:59 Bogota",
			now:       time.Date(2026, time.October, 24, 4, 59, 59, 0, time.UTC),
			wantToday: time.Date(2026, time.October, 23, 5, 0, 0, 0, time.UTC),
			wantLast:  "2026-10-23",
		},
		{
			name:      "00:00:01 Bogota",
			now:       time.Date(2026, time.October, 24, 5, 0, 1, 0, time.UTC),
			wantToday: time.Date(2026, time.October, 24, 5, 0, 0, 0, time.UTC),
			wantLast:  "2026-10-24",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &fakeDashboardRepository{}
			service := dashboardServiceAt(repository, tt.now)

			if _, err := service.Summary(context.Background()); err != nil {
				t.Fatal(err)
			}
			if !repository.summaryToday.Equal(tt.wantToday) {
				t.Fatalf("todayStart = %s, want %s", repository.summaryToday, tt.wantToday)
			}

			points, err := service.SalesTrend(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(points) != dashboardDays {
				t.Fatalf("trend length = %d, want %d", len(points), dashboardDays)
			}
			if points[len(points)-1].Date != tt.wantLast {
				t.Fatalf("last trend date = %s, want %s", points[len(points)-1].Date, tt.wantLast)
			}
		})
	}
}

// The empty database must still produce seven zero-filled days with the same
// ordering guarantees as a populated one.
func TestDashboardTrendEmptyProducesOrderedZeroDays(t *testing.T) {
	repository := &fakeDashboardRepository{trend: []domain.SalesTrendPoint{}}
	now := time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)

	points, err := dashboardServiceAt(repository, now).SalesTrend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != dashboardDays {
		t.Fatalf("trend length = %d, want %d", len(points), dashboardDays)
	}
	for i, point := range points {
		if i > 0 && point.Date <= points[i-1].Date {
			t.Fatalf("trend is not oldest-first at %d: %s after %s", i, point.Date, points[i-1].Date)
		}
		if !point.SalesAmount.IsZero() || !point.OrdersCost.IsZero() {
			t.Fatalf("point %s = %+v, want zeros", point.Date, point)
		}
	}
}
