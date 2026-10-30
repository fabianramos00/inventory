package gormstore

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/outbound"
)

type dashboardStore struct {
	db     *gorm.DB
	sqlite bool
}

var _ outbound.DashboardRepository = (*dashboardStore)(nil)

func NewDashboardStore(db *gorm.DB) outbound.DashboardRepository {
	return &dashboardStore{db: db, sqlite: db.Dialector.Name() == "sqlite"}
}

func (s *dashboardStore) Summary(ctx context.Context, todayStart time.Time) (*domain.DashboardSummary, error) {
	var inventoryRaw any
	if err := s.db.WithContext(ctx).Raw(
		`SELECT ` + moneySQL(s.sqlite, "COALESCE(SUM(price * stock_quantity), 0)") + ` FROM products`,
	).Row().Scan(&inventoryRaw); err != nil {
		return nil, fmt.Errorf("dashboard inventory value: %w", err)
	}

	var (
		todayRaw   any
		pendingRaw any
		delivered  int64
	)
	if err := s.db.WithContext(ctx).Raw(
		`SELECT
			`+moneySQL(s.sqlite, "COALESCE(SUM(CASE WHEN created_at >= ? THEN total_amount ELSE 0 END), 0)")+`,
			`+moneySQL(s.sqlite, "COALESCE(SUM(CASE WHEN payment_status <> 'PAID' THEN total_amount - amount_paid ELSE 0 END), 0)")+`,
			COALESCE(SUM(CASE WHEN delivery_status = 'DELIVERED' THEN 1 ELSE 0 END), 0)
		FROM sales`,
		Timestamp(todayStart),
	).Row().Scan(&todayRaw, &pendingRaw, &delivered); err != nil {
		return nil, fmt.Errorf("dashboard sales summary: %w", err)
	}

	var pendingOrders int64
	if err := s.db.WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM purchase_orders WHERE status = 'PENDING'",
	).Row().Scan(&pendingOrders); err != nil {
		return nil, fmt.Errorf("dashboard pending orders: %w", err)
	}

	inventory, err := moneyFromDB(s.sqlite, inventoryRaw)
	if err != nil {
		return nil, fmt.Errorf("dashboard inventory value: %w", err)
	}
	today, err := moneyFromDB(s.sqlite, todayRaw)
	if err != nil {
		return nil, fmt.Errorf("dashboard sales today: %w", err)
	}
	pending, err := moneyFromDB(s.sqlite, pendingRaw)
	if err != nil {
		return nil, fmt.Errorf("dashboard pending payment: %w", err)
	}
	return &domain.DashboardSummary{
		InventoryValue:      inventory,
		TotalSalesToday:     today,
		PendingPaymentSales: pending,
		PendingOrders:       pendingOrders,
		DeliveredOrders:     delivered,
	}, nil
}

func (s *dashboardStore) SalesTrend(ctx context.Context, start, end time.Time) ([]domain.SalesTrendPoint, error) {
	sales, err := s.trendSeries(ctx,
		"SELECT "+s.dateSQL("s.created_at")+", "+moneySQL(s.sqlite, "COALESCE(SUM(s.total_amount), 0)")+
			" FROM sales s WHERE s.created_at >= ? AND s.created_at < ? GROUP BY 1 ORDER BY 1",
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard sales trend: %w", err)
	}
	orders, err := s.trendSeries(ctx,
		"SELECT "+s.dateSQL("po.order_date")+", "+moneySQL(s.sqlite, "COALESCE(SUM(po.total_amount), 0)")+
			" FROM purchase_orders po WHERE po.order_date >= ? AND po.order_date < ? GROUP BY 1 ORDER BY 1",
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard orders trend: %w", err)
	}

	dates := make([]string, 0, len(sales)+len(orders))
	for date := range sales {
		dates = append(dates, date)
	}
	for date := range orders {
		if _, ok := sales[date]; !ok {
			dates = append(dates, date)
		}
	}
	sort.Strings(dates)

	points := make([]domain.SalesTrendPoint, 0, len(dates))
	for _, date := range dates {
		points = append(points, domain.SalesTrendPoint{
			Date:        date,
			SalesAmount: sales[date],
			OrdersCost:  orders[date],
		})
	}
	return points, nil
}

func (s *dashboardStore) trendSeries(ctx context.Context, query string, start, end time.Time) (map[string]decimal.Decimal, error) {
	rows, err := s.db.WithContext(ctx).Raw(query, Timestamp(start), Timestamp(end)).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]decimal.Decimal)
	for rows.Next() {
		var (
			date      string
			amountRaw any
		)
		if err := rows.Scan(&date, &amountRaw); err != nil {
			return nil, err
		}
		amount, err := moneyFromDB(s.sqlite, amountRaw)
		if err != nil {
			return nil, err
		}
		result[date] = amount
	}
	return result, rows.Err()
}

func (s *dashboardStore) TopProducts(ctx context.Context, since time.Time, limit int) ([]domain.TopProduct, error) {
	rows, err := s.db.WithContext(ctx).Raw(
		`SELECT
			p.id, p.name, p.sku, b.name, c.name, m.name,
			COALESCE(SUM(si.delivered_quantity), 0),
			`+moneySQL(s.sqlite, "COALESCE(SUM(si.subtotal), 0)")+`
		FROM sales s
		JOIN sale_items si ON si.sale_id = s.id
		JOIN products p ON p.id = si.product_id
		LEFT JOIN brands b ON b.id = p.brand_id
		LEFT JOIN categories c ON c.id = p.category_id
		LEFT JOIN materials m ON m.id = p.material_id
		WHERE s.created_at >= ?
		GROUP BY p.id, p.name, p.sku, b.name, c.name, m.name
		ORDER BY SUM(si.subtotal) DESC
		LIMIT ?`,
		Timestamp(since), limit,
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("dashboard top products: %w", err)
	}
	defer rows.Close()

	products := make([]domain.TopProduct, 0)
	for rows.Next() {
		var (
			productID    int64
			productName  string
			productSKU   string
			brandName    sql.NullString
			categoryName sql.NullString
			materialName sql.NullString
			quantity     int64
			amountRaw    any
		)
		if err := rows.Scan(
			&productID, &productName, &productSKU,
			&brandName, &categoryName, &materialName,
			&quantity, &amountRaw,
		); err != nil {
			return nil, err
		}
		amount, err := moneyFromDB(s.sqlite, amountRaw)
		if err != nil {
			return nil, fmt.Errorf("dashboard top product amount: %w", err)
		}
		products = append(products, domain.TopProduct{
			ProductID:         productID,
			ProductName:       productName,
			ProductSKU:        productSKU,
			BrandName:         nullString(brandName),
			CategoryName:      nullString(categoryName),
			MaterialName:      nullString(materialName),
			TotalQuantitySold: quantity,
			TotalSalesAmount:  amount,
		})
	}
	return products, rows.Err()
}

func (s *dashboardStore) OrderStatusDistribution(ctx context.Context) ([]domain.OrderStatusCount, error) {
	rows, err := s.db.WithContext(ctx).Raw(
		`SELECT status, COUNT(*)
		FROM purchase_orders
		GROUP BY status
		ORDER BY COUNT(*) DESC`,
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("dashboard order status distribution: %w", err)
	}
	defer rows.Close()

	result := make([]domain.OrderStatusCount, 0)
	for rows.Next() {
		var (
			status string
			count  int64
		)
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result = append(result, domain.OrderStatusCount{Status: apiOrderStatus(status), Count: count})
	}
	return result, rows.Err()
}

func (s *dashboardStore) dateSQL(column string) string {
	if s.sqlite {
		return "date(" + column + ", '-5 hours')"
	}
	return "((" + column + " AT TIME ZONE 'America/Bogota')::date)::text"
}
