package outbound

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
)

type SaleFilter struct {
	Skip           int
	Limit          int
	UserID         *int64
	ClientID       *int64
	NoClient       bool
	StartDate      *time.Time
	EndDate        *time.Time
	PaymentStatus  string
	DeliveryStatus string
	PaymentMethod  string
}

type SaleRepository interface {
	List(ctx context.Context, filter SaleFilter) ([]*domain.Sale, bool, error)
	Get(ctx context.Context, id int64) (*domain.Sale, error)
	Stats(ctx context.Context) (*domain.SaleStats, error)
	Create(ctx context.Context, sale *domain.Sale, items []*domain.SaleItem) (*domain.Sale, error)
	// Lock loads the sale and takes a write lock on its row so payment and
	// item mutations cannot overwrite each other's amount_paid.
	Lock(ctx context.Context, id int64) (*domain.Sale, error)
	Update(ctx context.Context, id int64, sale *domain.Sale) error
	Delete(ctx context.Context, id int64) error

	ListPayments(ctx context.Context, saleID int64, skip, limit int) ([]*domain.SalePayment, bool, error)
	GetPayment(ctx context.Context, saleID, paymentID int64) (*domain.SalePayment, error)
	CreatePayment(ctx context.Context, payment *domain.SalePayment) (*domain.SalePayment, error)
	DeletePayment(ctx context.Context, saleID, paymentID int64) error
	SumPayments(ctx context.Context, saleID int64) (decimal.Decimal, error)

	ListItems(ctx context.Context, saleID int64, skip, limit int) ([]*domain.SaleItem, bool, error)
	ListAllItems(ctx context.Context, saleID int64) ([]*domain.SaleItem, error)
	GetItem(ctx context.Context, saleID, itemID int64) (*domain.SaleItem, error)
	GetItemByProduct(ctx context.Context, saleID, productID int64) (*domain.SaleItem, error)
	CreateItem(ctx context.Context, item *domain.SaleItem) (*domain.SaleItem, error)
	UpdateItem(ctx context.Context, item *domain.SaleItem) error
	DeleteItem(ctx context.Context, saleID, itemID int64) error
	DeleteItems(ctx context.Context, saleID int64) error
}
