package outbound

import (
	"context"
	"time"

	"inventory-desktop/internal/domain"
)

type PurchaseOrderFilter struct {
	Skip          int
	Limit         int
	ProviderID    *int64
	Status        string
	PaymentStatus string
	MinDate       *time.Time
	MaxDate       *time.Time
}

type PurchaseOrderRepository interface {
	List(ctx context.Context, filter PurchaseOrderFilter) ([]*domain.PurchaseOrder, bool, error)
	Get(ctx context.Context, id int64) (*domain.PurchaseOrder, error)
	GetForUpdate(ctx context.Context, id int64) (*domain.PurchaseOrder, error)
	Create(ctx context.Context, order *domain.PurchaseOrder, items []*domain.OrderItem) (*domain.PurchaseOrder, error)
	Update(ctx context.Context, id int64, order *domain.PurchaseOrder) error
	Delete(ctx context.Context, id int64) error

	ListItems(ctx context.Context, orderID int64, skip, limit int) ([]*domain.OrderItem, bool, error)
	ListAllItems(ctx context.Context, orderID int64) ([]*domain.OrderItem, error)
	GetItem(ctx context.Context, orderID, itemID int64) (*domain.OrderItem, error)
	GetItemByProduct(ctx context.Context, orderID, productID int64) (*domain.OrderItem, error)
	CreateItem(ctx context.Context, item *domain.OrderItem) (*domain.OrderItem, error)
	UpdateItem(ctx context.Context, item *domain.OrderItem) error
	DeleteItem(ctx context.Context, orderID, itemID int64) error
	DeleteItems(ctx context.Context, orderID int64) error
}
