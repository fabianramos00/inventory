package application

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

const maxSupplierSKU = 100

type PurchaseOrderService struct {
	repository outbound.PurchaseOrderRepository
	unitOfWork outbound.UnitOfWork
}

func NewPurchaseOrderService(repository outbound.PurchaseOrderRepository, unitOfWork outbound.UnitOfWork) *PurchaseOrderService {
	return &PurchaseOrderService{repository: repository, unitOfWork: unitOfWork}
}

var _ inbound.PurchaseOrderUseCase = (*PurchaseOrderService)(nil)

func (s *PurchaseOrderService) List(ctx context.Context, filter inbound.PurchaseOrderFilter) (*inbound.PurchaseOrderList, error) {
	if filter.Status != "" && !domain.ValidOrderStatus(filter.Status) {
		return nil, fmt.Errorf("%w: invalid status", domain.ErrValidation)
	}
	if filter.PaymentStatus != "" && !domain.ValidPurchasePaymentStatus(filter.PaymentStatus) {
		return nil, fmt.Errorf("%w: invalid payment_status", domain.ErrValidation)
	}
	items, hasNext, err := s.repository.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &inbound.PurchaseOrderList{Items: items, HasNext: hasNext, Skip: filter.Skip, Limit: filter.Limit}, nil
}

func (s *PurchaseOrderService) Get(ctx context.Context, id int64) (*domain.PurchaseOrder, error) {
	order, err := s.repository.Get(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, &domain.NotFoundError{Entity: "Purchase order", ID: id}
	}
	return order, err
}

func (s *PurchaseOrderService) Create(ctx context.Context, input inbound.CreatePurchaseOrderInput) (*domain.PurchaseOrder, error) {
	status := input.Status
	if status == "" {
		status = domain.OrderStatusPending
	}
	paymentStatus := input.PaymentStatus
	if paymentStatus == "" {
		paymentStatus = domain.PurchasePaymentStatusPending
	}
	if !domain.ValidOrderStatus(status) {
		return nil, fmt.Errorf("%w: invalid status", domain.ErrValidation)
	}
	if !domain.ValidPurchasePaymentStatus(paymentStatus) {
		return nil, fmt.Errorf("%w: invalid payment_status", domain.ErrValidation)
	}
	if len(input.Items) == 0 {
		return nil, fmt.Errorf("%w: purchase order must have at least one item", domain.ErrValidation)
	}
	seen := make(map[int64]struct{}, len(input.Items))
	for _, item := range input.Items {
		if err := validateOrderItem(item.ProductID, item.Quantity, item.UnitCost, item.SupplierSKU); err != nil {
			return nil, err
		}
		if _, ok := seen[item.ProductID]; ok {
			return nil, fmt.Errorf("%w: one item per product is allowed", domain.ErrValidation)
		}
		seen[item.ProductID] = struct{}{}
	}

	var result *domain.PurchaseOrder
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := repos.Providers.Get(ctx, input.ProviderID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return &domain.ProviderNotFoundError{ID: input.ProviderID}
			}
			return err
		}
		for productID := range seen {
			if _, err := repos.Products.Get(ctx, productID); err != nil {
				return referenceError(err, "Product", productID)
			}
		}

		items := make([]*domain.OrderItem, 0, len(input.Items))
		total := decimal.Zero
		for _, item := range input.Items {
			unitCost := item.UnitCost.Round(2)
			subtotal := unitCost.Mul(decimal.NewFromInt(int64(item.Quantity))).Round(2)
			total = total.Add(subtotal)
			items = append(items, &domain.OrderItem{
				ProductID:   item.ProductID,
				Quantity:    item.Quantity,
				UnitCost:    unitCost,
				Subtotal:    subtotal,
				SupplierSKU: clonePtr(item.SupplierSKU),
			})
		}

		order := &domain.PurchaseOrder{
			ProviderID:    input.ProviderID,
			Status:        status,
			PaymentStatus: paymentStatus,
			OrderDate:     nowUTC(),
			TotalAmount:   total.Round(2),
			CreatedByID:   input.CreatedByID,
		}
		created, err := repos.Orders.Create(ctx, order, items)
		if err != nil {
			return err
		}
		result, err = repos.Orders.Get(ctx, created.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PurchaseOrderService) Update(ctx context.Context, id int64, input inbound.UpdatePurchaseOrderInput) (*domain.PurchaseOrder, error) {
	if input.ProviderID.Set && (input.ProviderID.Value == nil || *input.ProviderID.Value <= 0) {
		return nil, requiredField("provider_id")
	}
	if input.Status.Set {
		if input.Status.Value == nil {
			return nil, requiredField("status")
		}
		if !domain.ValidOrderStatus(*input.Status.Value) {
			return nil, fmt.Errorf("%w: invalid status", domain.ErrValidation)
		}
	}
	if input.PaymentStatus.Set {
		if input.PaymentStatus.Value == nil {
			return nil, requiredField("payment_status")
		}
		if !domain.ValidPurchasePaymentStatus(*input.PaymentStatus.Value) {
			return nil, fmt.Errorf("%w: invalid payment_status", domain.ErrValidation)
		}
	}
	if !input.ProviderID.Set && !input.Status.Set && !input.PaymentStatus.Set {
		return s.Get(ctx, id)
	}

	var result *domain.PurchaseOrder
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		order, err := repos.Orders.GetForUpdate(ctx, id)
		if err != nil {
			return referenceError(err, "Purchase order", id)
		}
		oldStatus := order.Status

		if input.ProviderID.Set {
			providerID := *input.ProviderID.Value
			if _, err := repos.Providers.Get(ctx, providerID); err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return &domain.ProviderNotFoundError{ID: providerID}
				}
				return err
			}
			order.ProviderID = providerID
		}
		if input.Status.Set {
			order.Status = *input.Status.Value
		}
		if input.PaymentStatus.Set {
			order.PaymentStatus = *input.PaymentStatus.Value
		}

		if oldStatus != domain.OrderStatusReceived && order.Status == domain.OrderStatusReceived {
			if err := s.receive(ctx, repos, order); err != nil {
				return err
			}
		}

		if err := repos.Orders.Update(ctx, id, order); err != nil {
			return err
		}
		result, err = repos.Orders.Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PurchaseOrderService) receive(ctx context.Context, repos outbound.TxRepos, order *domain.PurchaseOrder) error {
	items, err := repos.Orders.ListAllItems(ctx, order.ID)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ProductID)
	}
	stock, err := repos.Stock.LockForUpdate(ctx, ids)
	if err != nil {
		return err
	}
	for _, id := range sortedIDs(ids) {
		if _, ok := stock[id]; !ok {
			return &domain.NotFoundError{Entity: "Product", ID: id}
		}
	}

	now := nowUTC()
	leadTime := int64(now.Sub(order.OrderDate) / (24 * time.Hour))
	for _, item := range items {
		if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(item.Quantity)); err != nil {
			return err
		}
		if err := repos.Catalogs.Upsert(ctx, &domain.SupplierCatalog{
			ProductID:      item.ProductID,
			ProviderID:     order.ProviderID,
			SupplierSKU:    clonePtr(item.SupplierSKU),
			LastQuotedCost: item.UnitCost.Round(2),
			LeadTime:       &leadTime,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *PurchaseOrderService) Delete(ctx context.Context, id int64) (*domain.PurchaseOrder, error) {
	var result *domain.PurchaseOrder
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		order, err := repos.Orders.Get(ctx, id)
		if err != nil {
			return referenceError(err, "Purchase order", id)
		}
		if err := repos.Orders.DeleteItems(ctx, id); err != nil {
			return err
		}
		if err := repos.Orders.Delete(ctx, id); err != nil {
			return err
		}
		result = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PurchaseOrderService) ListItems(ctx context.Context, orderID int64, skip, limit int) (*inbound.OrderItemList, error) {
	if _, err := s.Get(ctx, orderID); err != nil {
		return nil, err
	}
	items, hasNext, err := s.repository.ListItems(ctx, orderID, skip, limit)
	if err != nil {
		return nil, err
	}
	return &inbound.OrderItemList{Items: items, HasNext: hasNext, Skip: skip, Limit: limit}, nil
}

func (s *PurchaseOrderService) AddItem(ctx context.Context, orderID int64, input inbound.AddOrderItemInput) (*domain.PurchaseOrder, error) {
	if err := validateOrderItem(input.ProductID, input.Quantity, input.UnitCost, input.SupplierSKU); err != nil {
		return nil, err
	}

	var result *domain.PurchaseOrder
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := repos.Orders.Get(ctx, orderID); err != nil {
			return referenceError(err, "Purchase order", orderID)
		}
		if _, err := repos.Products.Get(ctx, input.ProductID); err != nil {
			return referenceError(err, "Product", input.ProductID)
		}
		if _, err := repos.Orders.GetItemByProduct(ctx, orderID, input.ProductID); err == nil {
			return fmt.Errorf("%w: product with id %d already exists in this order", domain.ErrConflict, input.ProductID)
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		unitCost := input.UnitCost.Round(2)
		item := &domain.OrderItem{
			OrderID:     orderID,
			ProductID:   input.ProductID,
			Quantity:    input.Quantity,
			UnitCost:    unitCost,
			Subtotal:    unitCost.Mul(decimal.NewFromInt(int64(input.Quantity))).Round(2),
			SupplierSKU: clonePtr(input.SupplierSKU),
		}
		if _, err := repos.Orders.CreateItem(ctx, item); err != nil {
			return err
		}
		order, err := s.recalculate(ctx, repos, orderID)
		if err != nil {
			return err
		}
		result = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PurchaseOrderService) UpdateItem(ctx context.Context, orderID, itemID int64, input inbound.UpdateOrderItemInput) (*domain.PurchaseOrder, error) {
	if input.Quantity.Set && (input.Quantity.Value == nil || *input.Quantity.Value <= 0) {
		return nil, fmt.Errorf("%w: quantity must be greater than zero", domain.ErrValidation)
	}
	if input.UnitCost.Set {
		if input.UnitCost.Value == nil {
			return nil, requiredField("unit_cost")
		}
		if input.UnitCost.Value.IsNegative() {
			return nil, nonNegative("unit_cost")
		}
	}
	if input.SupplierSKU.Set && input.SupplierSKU.Value != nil && utf8.RuneCountInString(*input.SupplierSKU.Value) > maxSupplierSKU {
		return nil, tooLong("supplier_sku", maxSupplierSKU)
	}

	var result *domain.PurchaseOrder
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := repos.Orders.Get(ctx, orderID); err != nil {
			return referenceError(err, "Purchase order", orderID)
		}
		item, err := repos.Orders.GetItem(ctx, orderID, itemID)
		if err != nil {
			return referenceError(err, "Order item", itemID)
		}
		if input.Quantity.Set {
			item.Quantity = *input.Quantity.Value
		}
		if input.UnitCost.Set {
			item.UnitCost = input.UnitCost.Value.Round(2)
		}
		if input.SupplierSKU.Set {
			item.SupplierSKU = clonePtr(input.SupplierSKU.Value)
		}
		item.Subtotal = item.UnitCost.Mul(decimal.NewFromInt(int64(item.Quantity))).Round(2)
		if err := repos.Orders.UpdateItem(ctx, item); err != nil {
			return err
		}
		order, err := s.recalculate(ctx, repos, orderID)
		if err != nil {
			return err
		}
		result = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PurchaseOrderService) DeleteItem(ctx context.Context, orderID, itemID int64) (*domain.PurchaseOrder, error) {
	var result *domain.PurchaseOrder
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := repos.Orders.Get(ctx, orderID); err != nil {
			return referenceError(err, "Purchase order", orderID)
		}
		if _, err := repos.Orders.GetItem(ctx, orderID, itemID); err != nil {
			return referenceError(err, "Order item", itemID)
		}
		if err := repos.Orders.DeleteItem(ctx, orderID, itemID); err != nil {
			return err
		}
		order, err := s.recalculate(ctx, repos, orderID)
		if err != nil {
			return err
		}
		result = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PurchaseOrderService) recalculate(ctx context.Context, repos outbound.TxRepos, orderID int64) (*domain.PurchaseOrder, error) {
	order, err := repos.Orders.Get(ctx, orderID)
	if err != nil {
		return nil, err
	}
	items, err := repos.Orders.ListAllItems(ctx, orderID)
	if err != nil {
		return nil, err
	}
	order.TotalAmount = orderTotal(items)
	if err := repos.Orders.Update(ctx, orderID, order); err != nil {
		return nil, err
	}
	return repos.Orders.Get(ctx, orderID)
}

func validateOrderItem(productID int64, quantity int, unitCost decimal.Decimal, sku *string) error {
	if productID <= 0 {
		return fmt.Errorf("%w: product_id must be greater than zero", domain.ErrValidation)
	}
	if quantity <= 0 {
		return fmt.Errorf("%w: quantity must be greater than zero", domain.ErrValidation)
	}
	if unitCost.IsNegative() {
		return nonNegative("unit_cost")
	}
	if sku != nil && utf8.RuneCountInString(*sku) > maxSupplierSKU {
		return tooLong("supplier_sku", maxSupplierSKU)
	}
	return nil
}

func orderTotal(items []*domain.OrderItem) decimal.Decimal {
	total := decimal.Zero
	for _, item := range items {
		total = total.Add(item.Subtotal)
	}
	return total.Round(2)
}
