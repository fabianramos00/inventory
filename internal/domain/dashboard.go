package domain

import "github.com/shopspring/decimal"

type DashboardSummary struct {
	InventoryValue      decimal.Decimal
	TotalSalesToday     decimal.Decimal
	PendingPaymentSales decimal.Decimal
	PendingOrders       int64
	DeliveredOrders     int64
}

type SalesTrendPoint struct {
	Date        string
	SalesAmount decimal.Decimal
	OrdersCost  decimal.Decimal
}

type TopProduct struct {
	ProductID         int64
	ProductName       string
	ProductSKU        string
	BrandName         *string
	CategoryName      *string
	MaterialName      *string
	TotalQuantitySold int64
	TotalSalesAmount  decimal.Decimal
}

type OrderStatusCount struct {
	Status     string
	Count      int64
	Percentage float64
}
