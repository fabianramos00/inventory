package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"inventory-desktop/internal/domain"
	"inventory-desktop/internal/ports/inbound"
	"inventory-desktop/internal/ports/outbound"
)

type SaleService struct {
	repository outbound.SaleRepository
	unitOfWork outbound.UnitOfWork
}

func NewSaleService(repository outbound.SaleRepository, unitOfWork outbound.UnitOfWork) *SaleService {
	return &SaleService{repository: repository, unitOfWork: unitOfWork}
}

var _ inbound.SaleUseCase = (*SaleService)(nil)

func (s *SaleService) List(ctx context.Context, filter inbound.SaleFilter) (*inbound.SaleList, error) {
	if !validPaymentStatus(filter.PaymentStatus) {
		return nil, fmt.Errorf("%w: invalid payment_status", domain.ErrValidation)
	}
	if !validDeliveryStatus(filter.DeliveryStatus) {
		return nil, fmt.Errorf("%w: invalid delivery_status", domain.ErrValidation)
	}
	if !validPaymentMethodFilter(filter.PaymentMethod) {
		return nil, fmt.Errorf("%w: invalid payment_method", domain.ErrValidation)
	}
	items, hasNext, err := s.repository.List(ctx, outbound.SaleFilter{
		Skip:           filter.Skip,
		Limit:          filter.Limit,
		UserID:         filter.UserID,
		ClientID:       filter.ClientID,
		NoClient:       filter.NoClient,
		StartDate:      filter.StartDate,
		EndDate:        filter.EndDate,
		PaymentStatus:  filter.PaymentStatus,
		DeliveryStatus: filter.DeliveryStatus,
		PaymentMethod:  filter.PaymentMethod,
	})
	if err != nil {
		return nil, err
	}
	return &inbound.SaleList{Items: items, HasNext: hasNext, Skip: filter.Skip, Limit: filter.Limit}, nil
}

func (s *SaleService) Get(ctx context.Context, id int64) (*domain.Sale, error) {
	sale, err := s.repository.Get(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, &domain.NotFoundError{Entity: "Sale", ID: id}
	}
	return sale, err
}

func (s *SaleService) Stats(ctx context.Context) (*domain.SaleStats, error) {
	return s.repository.Stats(ctx)
}

func (s *SaleService) Create(ctx context.Context, input inbound.CreateSaleInput) (*domain.Sale, error) {
	if err := validateCreateSale(input); err != nil {
		return nil, err
	}

	var result *domain.Sale
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if input.ClientID != nil {
			if _, err := repos.Clients.Get(ctx, *input.ClientID); err != nil {
				return referenceError(err, "Client", *input.ClientID)
			}
		}

		ids := saleProductIDs(input.Items)
		stock, err := repos.Stock.LockForUpdate(ctx, ids)
		if err != nil {
			return err
		}
		products := make(map[int64]*domain.ProductDetail, len(ids))
		for _, id := range ids {
			if _, ok := stock[id]; !ok {
				return &domain.NotFoundError{Entity: "Product", ID: id}
			}
			product, err := repos.Products.Get(ctx, id)
			if err != nil {
				return referenceError(err, "Product", id)
			}
			products[id] = product
		}

		items := make([]*domain.SaleItem, 0, len(input.Items))
		total := decimal.Zero
		for _, item := range input.Items {
			available := int(stock[item.ProductID])
			if available < item.Quantity {
				return &domain.InsufficientStockError{
					SKU:       products[item.ProductID].SKU,
					Available: available,
					Requested: item.Quantity,
				}
			}
			unitPrice := products[item.ProductID].Price
			if item.UnitPrice != nil {
				unitPrice = *item.UnitPrice
			}
			unitPrice = unitPrice.Round(2)
			subtotal := unitPrice.Mul(decimal.NewFromInt(int64(item.Quantity))).Round(2)
			total = total.Add(subtotal)
			items = append(items, &domain.SaleItem{
				ProductID:         item.ProductID,
				Quantity:          item.Quantity,
				DeliveredQuantity: item.DeliveredQuantity,
				UnitPrice:         unitPrice,
				Subtotal:          subtotal,
			})
		}

		for _, item := range items {
			if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(-item.Quantity)); err != nil {
				return stockError(err, products[item.ProductID].SKU, stock[item.ProductID], item.Quantity)
			}
		}

		now := saleNow()
		paid := input.AmountPaid.Round(2)
		sale := &domain.Sale{
			CreatedAt:      now,
			TotalAmount:    total,
			AmountPaid:     paid,
			DebtAmount:     debtAmount(total, paid),
			PaymentStatus:  derivePaymentStatus(total, paid),
			DeliveryStatus: deriveDeliveryStatus(items),
			PaymentMethod:  input.PaymentMethod,
			ClientID:       input.ClientID,
			CreatedByID:    input.CreatedByID,
		}
		created, err := repos.Sales.Create(ctx, sale, items)
		if err != nil {
			return err
		}
		result, err = repos.Sales.Get(ctx, created.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) Update(ctx context.Context, id int64, input inbound.UpdateSaleInput) (*domain.Sale, error) {
	if input.PaymentMethod.Set && input.PaymentMethod.Value != nil && !validPaymentMethod(*input.PaymentMethod.Value) {
		return nil, fmt.Errorf("%w: invalid payment_method", domain.ErrValidation)
	}
	if input.AmountPaid.Set {
		if input.AmountPaid.Value == nil {
			return nil, requiredField("amount_paid")
		}
		if input.AmountPaid.Value.IsNegative() {
			return nil, nonNegative("amount_paid")
		}
	}
	if input.ClientID.Set && input.ClientID.Value != nil && *input.ClientID.Value <= 0 {
		return nil, fmt.Errorf("%w: client_id must be greater than zero", domain.ErrValidation)
	}
	if !input.PaymentMethod.Set && !input.AmountPaid.Set && !input.ClientID.Set {
		return s.Get(ctx, id)
	}

	var result *domain.Sale
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		sale, err := repos.Sales.Get(ctx, id)
		if err != nil {
			return referenceError(err, "Sale", id)
		}
		if input.PaymentMethod.Set {
			sale.PaymentMethod = cloneOptionalString(input.PaymentMethod.Value)
		}
		if input.AmountPaid.Set {
			sale.AmountPaid = input.AmountPaid.Value.Round(2)
		}
		if input.ClientID.Set {
			if input.ClientID.Value == nil {
				sale.ClientID = nil
			} else {
				if _, err := repos.Clients.Get(ctx, *input.ClientID.Value); err != nil {
					return referenceError(err, "Client", *input.ClientID.Value)
				}
				sale.ClientID = input.ClientID.Value
			}
		}
		sale.PaymentStatus = derivePaymentStatus(sale.TotalAmount, sale.AmountPaid)
		sale.DebtAmount = debtAmount(sale.TotalAmount, sale.AmountPaid)
		now := saleNow()
		sale.UpdatedAt = &now
		if err := repos.Sales.Update(ctx, id, sale); err != nil {
			return err
		}
		result, err = repos.Sales.Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) Delete(ctx context.Context, id int64) (*domain.Sale, error) {
	var result *domain.Sale
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		sale, err := repos.Sales.Get(ctx, id)
		if err != nil {
			return referenceError(err, "Sale", id)
		}
		items, err := repos.Sales.ListAllItems(ctx, id)
		if err != nil {
			return err
		}
		ids := make([]int64, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.ProductID)
		}
		if _, err := repos.Stock.LockForUpdate(ctx, ids); err != nil {
			return err
		}
		for _, item := range items {
			if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(item.Quantity)); err != nil {
				return err
			}
		}
		if err := repos.Sales.DeleteItems(ctx, id); err != nil {
			return err
		}
		if err := repos.Sales.Delete(ctx, id); err != nil {
			return err
		}
		result = sale
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) ListItems(ctx context.Context, saleID int64, skip, limit int) (*inbound.SaleItemList, error) {
	if _, err := s.Get(ctx, saleID); err != nil {
		return nil, err
	}
	items, hasNext, err := s.repository.ListItems(ctx, saleID, skip, limit)
	if err != nil {
		return nil, err
	}
	return &inbound.SaleItemList{Items: items, HasNext: hasNext, Skip: skip, Limit: limit}, nil
}

func (s *SaleService) AddItem(ctx context.Context, saleID int64, input inbound.AddSaleItemInput) (*domain.Sale, error) {
	if err := validateSaleItem(input.ProductID, input.Quantity, input.DeliveredQuantity, input.UnitPrice); err != nil {
		return nil, err
	}

	var result *domain.Sale
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := repos.Sales.Get(ctx, saleID); err != nil {
			return referenceError(err, "Sale", saleID)
		}
		product, err := repos.Products.Get(ctx, input.ProductID)
		if err != nil {
			return referenceError(err, "Product", input.ProductID)
		}
		if _, err := repos.Sales.GetItemByProduct(ctx, saleID, input.ProductID); err == nil {
			return fmt.Errorf("%w: product with id %d already exists in this sale", domain.ErrConflict, input.ProductID)
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		stock, err := repos.Stock.LockForUpdate(ctx, []int64{input.ProductID})
		if err != nil {
			return err
		}
		available := int(stock[input.ProductID])
		if available < input.Quantity {
			return &domain.InsufficientStockError{
				SKU:       product.SKU,
				Available: available,
				Requested: input.Quantity,
			}
		}
		if err := repos.Stock.ApplyDelta(ctx, input.ProductID, int64(-input.Quantity)); err != nil {
			return stockError(err, product.SKU, stock[input.ProductID], input.Quantity)
		}

		unitPrice := product.Price
		if input.UnitPrice != nil {
			unitPrice = *input.UnitPrice
		}
		unitPrice = unitPrice.Round(2)
		item := &domain.SaleItem{
			SaleID:            saleID,
			ProductID:         input.ProductID,
			Quantity:          input.Quantity,
			DeliveredQuantity: input.DeliveredQuantity,
			UnitPrice:         unitPrice,
			Subtotal:          unitPrice.Mul(decimal.NewFromInt(int64(input.Quantity))).Round(2),
		}
		if _, err := repos.Sales.CreateItem(ctx, item); err != nil {
			return err
		}
		result, err = s.recalculate(ctx, repos, saleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) UpdateItem(ctx context.Context, saleID, itemID int64, input inbound.UpdateSaleItemInput) (*domain.Sale, error) {
	if input.Quantity.Set && (input.Quantity.Value == nil || *input.Quantity.Value <= 0) {
		return nil, fmt.Errorf("%w: quantity must be greater than zero", domain.ErrValidation)
	}
	if input.DeliveredQuantity.Set && input.DeliveredQuantity.Value == nil {
		return nil, requiredField("delivered_quantity")
	}
	if input.UnitPrice.Set {
		if input.UnitPrice.Value == nil {
			return nil, requiredField("unit_price")
		}
		if input.UnitPrice.Value.IsNegative() {
			return nil, nonNegative("unit_price")
		}
	}

	var result *domain.Sale
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := repos.Sales.Get(ctx, saleID); err != nil {
			return referenceError(err, "Sale", saleID)
		}
		item, err := repos.Sales.GetItem(ctx, saleID, itemID)
		if err != nil {
			return referenceError(err, "Sale item", itemID)
		}

		newQuantity := item.Quantity
		if input.Quantity.Set {
			newQuantity = *input.Quantity.Value
		}
		newDelivered := item.DeliveredQuantity
		if input.DeliveredQuantity.Set {
			newDelivered = *input.DeliveredQuantity.Value
		}
		if newDelivered < 0 || newDelivered > newQuantity {
			return fmt.Errorf("%w: delivered_quantity cannot exceed quantity", domain.ErrValidation)
		}

		delta := newQuantity - item.Quantity
		if delta != 0 {
			stock, err := repos.Stock.LockForUpdate(ctx, []int64{item.ProductID})
			if err != nil {
				return err
			}
			if delta > 0 {
				available := int(stock[item.ProductID])
				if available < delta {
					return &domain.InsufficientStockError{
						SKU:       item.ProductSKU,
						Available: available,
						Requested: delta,
					}
				}
			}
			if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(-delta)); err != nil {
				return stockError(err, item.ProductSKU, stock[item.ProductID], delta)
			}
		}

		if input.UnitPrice.Set {
			item.UnitPrice = input.UnitPrice.Value.Round(2)
		}
		item.Quantity = newQuantity
		item.DeliveredQuantity = newDelivered
		item.Subtotal = item.UnitPrice.Mul(decimal.NewFromInt(int64(newQuantity))).Round(2)
		if err := repos.Sales.UpdateItem(ctx, item); err != nil {
			return err
		}
		result, err = s.recalculate(ctx, repos, saleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) DeleteItem(ctx context.Context, saleID, itemID int64) (*domain.Sale, error) {
	var result *domain.Sale
	err := s.unitOfWork.WithinTx(ctx, func(ctx context.Context, repos outbound.TxRepos) error {
		if _, err := repos.Sales.Get(ctx, saleID); err != nil {
			return referenceError(err, "Sale", saleID)
		}
		item, err := repos.Sales.GetItem(ctx, saleID, itemID)
		if err != nil {
			return referenceError(err, "Sale item", itemID)
		}
		if _, err := repos.Stock.LockForUpdate(ctx, []int64{item.ProductID}); err != nil {
			return err
		}
		if err := repos.Stock.ApplyDelta(ctx, item.ProductID, int64(item.Quantity)); err != nil {
			return err
		}
		if err := repos.Sales.DeleteItem(ctx, saleID, itemID); err != nil {
			return err
		}
		result, err = s.recalculate(ctx, repos, saleID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SaleService) recalculate(ctx context.Context, repos outbound.TxRepos, saleID int64) (*domain.Sale, error) {
	sale, err := repos.Sales.Get(ctx, saleID)
	if err != nil {
		return nil, err
	}
	items, err := repos.Sales.ListAllItems(ctx, saleID)
	if err != nil {
		return nil, err
	}
	sale.TotalAmount = saleTotal(items)
	if len(items) == 0 {
		sale.PaymentStatus = domain.PaymentStatusPending
		sale.DeliveryStatus = domain.DeliveryStatusPending
	} else {
		sale.PaymentStatus = derivePaymentStatus(sale.TotalAmount, sale.AmountPaid)
		sale.DeliveryStatus = deriveDeliveryStatus(items)
	}
	sale.DebtAmount = debtAmount(sale.TotalAmount, sale.AmountPaid)
	now := saleNow()
	sale.UpdatedAt = &now
	if err := repos.Sales.Update(ctx, saleID, sale); err != nil {
		return nil, err
	}
	return repos.Sales.Get(ctx, saleID)
}

func validateCreateSale(input inbound.CreateSaleInput) error {
	if len(input.Items) == 0 {
		return fmt.Errorf("%w: sale must have at least one item", domain.ErrValidation)
	}
	if input.PaymentMethod != nil && !validPaymentMethod(*input.PaymentMethod) {
		return fmt.Errorf("%w: invalid payment_method", domain.ErrValidation)
	}
	if input.AmountPaid.IsNegative() {
		return nonNegative("amount_paid")
	}
	if input.ClientID != nil && *input.ClientID <= 0 {
		return fmt.Errorf("%w: client_id must be greater than zero", domain.ErrValidation)
	}
	seen := make(map[int64]struct{}, len(input.Items))
	for _, item := range input.Items {
		if err := validateSaleItem(item.ProductID, item.Quantity, item.DeliveredQuantity, item.UnitPrice); err != nil {
			return err
		}
		if _, ok := seen[item.ProductID]; ok {
			return fmt.Errorf("%w: one item per product is allowed", domain.ErrValidation)
		}
		seen[item.ProductID] = struct{}{}
	}
	return nil
}

func validateSaleItem(productID int64, quantity, delivered int, unitPrice *decimal.Decimal) error {
	if productID <= 0 {
		return fmt.Errorf("%w: product_id must be greater than zero", domain.ErrValidation)
	}
	if quantity <= 0 {
		return fmt.Errorf("%w: quantity must be greater than zero", domain.ErrValidation)
	}
	if delivered < 0 || delivered > quantity {
		return fmt.Errorf("%w: delivered_quantity cannot exceed quantity", domain.ErrValidation)
	}
	if unitPrice != nil && unitPrice.IsNegative() {
		return nonNegative("unit_price")
	}
	return nil
}

func saleProductIDs(items []inbound.CreateSaleItemInput) []int64 {
	ids := make([]int64, len(items))
	for i, item := range items {
		ids[i] = item.ProductID
	}
	return sortedIDs(ids)
}

func sortedIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j-1] > result[j]; j-- {
			result[j-1], result[j] = result[j], result[j-1]
		}
	}
	return result
}

func referenceError(err error, entity string, id int64) error {
	if errors.Is(err, domain.ErrNotFound) {
		return &domain.NotFoundError{Entity: entity, ID: id}
	}
	return err
}

func stockError(err error, sku string, available int64, requested int) error {
	if errors.Is(err, domain.ErrInsufficientStock) {
		return &domain.InsufficientStockError{
			SKU:       sku,
			Available: int(available),
			Requested: requested,
		}
	}
	return err
}

func derivePaymentStatus(total, paid decimal.Decimal) string {
	if total.IsZero() && paid.IsZero() {
		return domain.PaymentStatusPending
	}
	switch {
	case paid.GreaterThanOrEqual(total):
		return domain.PaymentStatusPaid
	case paid.IsPositive():
		return domain.PaymentStatusPartial
	default:
		return domain.PaymentStatusPending
	}
}

func deriveDeliveryStatus(items []*domain.SaleItem) string {
	if len(items) == 0 {
		return domain.DeliveryStatusPending
	}
	allDelivered := true
	anyDelivered := false
	for _, item := range items {
		if item.DeliveredQuantity >= item.Quantity {
			anyDelivered = true
		} else {
			allDelivered = false
			if item.DeliveredQuantity > 0 {
				anyDelivered = true
			}
		}
	}
	if allDelivered {
		return domain.DeliveryStatusDelivered
	}
	if anyDelivered {
		return domain.DeliveryStatusPartial
	}
	return domain.DeliveryStatusPending
}

func saleTotal(items []*domain.SaleItem) decimal.Decimal {
	total := decimal.Zero
	for _, item := range items {
		total = total.Add(item.Subtotal)
	}
	return total
}

func debtAmount(total, paid decimal.Decimal) decimal.Decimal {
	debt := total.Sub(paid)
	if debt.IsNegative() {
		return decimal.Zero
	}
	return debt
}

func validPaymentStatus(value string) bool {
	switch value {
	case "", domain.PaymentStatusPending, domain.PaymentStatusPartial, domain.PaymentStatusPaid:
		return true
	default:
		return false
	}
}

func validDeliveryStatus(value string) bool {
	switch value {
	case "", domain.DeliveryStatusPending, domain.DeliveryStatusPartial, domain.DeliveryStatusDelivered:
		return true
	default:
		return false
	}
}

func validPaymentMethod(value string) bool {
	switch value {
	case domain.PaymentMethodCash, domain.PaymentMethodCredit, domain.PaymentMethodDebit:
		return true
	default:
		return false
	}
}

func validPaymentMethodFilter(value string) bool {
	return value == "" || validPaymentMethod(value)
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// PostgreSQL timestamptz keeps microseconds; truncating keeps the write
// response and a later read identical on both backends.
func saleNow() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
