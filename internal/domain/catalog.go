package domain

import "github.com/shopspring/decimal"

const (
	StockStatusInStock    = "in_stock"
	StockStatusLowStock   = "low_stock"
	StockStatusOutOfStock = "out_of_stock"
)

type Category struct {
	ID          int64
	Name        string
	Code        *string
	Description *string
}

type Brand struct {
	ID      int64
	Name    string
	LogoURL *string
}

type Material struct {
	ID   int64
	Name string
}

type MeasurementUnit struct {
	ID           int64
	Name         string
	Abbreviation string
}

type Product struct {
	ID                int64
	SKU               string
	Name              string
	SizeValue         *string
	MeasurementUnitID *int64
	Price             decimal.Decimal
	Cost              decimal.Decimal
	StockQuantity     int
	LowStockThreshold int
	BrandID           int64
	CategoryID        int64
	MaterialID        int64
}

// ProductDetail adds the catalog relations the legacy detail endpoints return.
// PriceHistory is intentionally absent: the legacy API reads and writes the
// table asymmetrically and never records product price changes.
type ProductDetail struct {
	Product
	Category        *Category
	Material        *Material
	Brand           *Brand
	MeasurementUnit *MeasurementUnit
}

type ProductStats struct {
	TotalProducts       int64
	TotalStock          int64
	LowStockCount       int64
	TotalInventoryValue decimal.Decimal
}

func ValidStockStatus(value string) bool {
	switch value {
	case "", StockStatusInStock, StockStatusLowStock, StockStatusOutOfStock:
		return true
	default:
		return false
	}
}
