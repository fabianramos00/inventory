package domain

import "github.com/shopspring/decimal"

type SupplierCatalog struct {
	ID             int64
	ProductID      int64
	ProviderID     int64
	SupplierSKU    *string
	LastQuotedCost decimal.Decimal
	LeadTime       *int64
	Provider       *Provider
	Product        *Product
}
